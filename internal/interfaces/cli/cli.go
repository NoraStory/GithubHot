// Package cli 是组合根（composition root）：在这里把领域仓储、
// 端口实现与用例装配起来。装配知识只存在于接口层。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/infrastructure/fetcher"
	"github.com/NoraStory/GithubHot/internal/infrastructure/githubapi"
	"github.com/NoraStory/GithubHot/internal/infrastructure/llm"
	"github.com/NoraStory/GithubHot/internal/infrastructure/notify"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
	"github.com/NoraStory/GithubHot/internal/infrastructure/prober"
	"github.com/NoraStory/GithubHot/internal/infrastructure/render"
	"github.com/NoraStory/GithubHot/internal/interfaces/httpapi"
	"github.com/NoraStory/GithubHot/internal/interfaces/mcp"
)

// Version 版本号（发布时更新）。
const Version = "0.1.0"

// build 装配全部依赖。
func build(cfg *config.Config) (application.Deps, *sqlite.DB, error) {
	db, err := sqlite.Open(cfg.DataDir)
	if err != nil {
		return application.Deps{}, nil, err
	}
	usageRepo := sqlite.NewUsageRepo(db)
	budget := application.BudgetConfig{
		DailyTokens:  cfg.BudgetTokensPerDay,
		PriceInPerM:  cfg.PriceInPerM,
		PriceOutPerM: cfg.PriceOutPerM,
	}
	deps := application.Deps{
		Sources:        sqlite.NewSourceRepo(db),
		Items:          sqlite.NewItemRepo(db),
		Projects:       sqlite.NewProjectRepo(db),
		Stories:        sqlite.NewStoryRepo(db),
		Digests:        sqlite.NewDigestRepo(db),
		Usage:          usageRepo,
		Budget:         budget,
		GitHub:         githubapi.New(cfg.GitHubToken, cfg.GitHubProxy),
		Fetchers:       fetcher.NewRegistry(),
		DigestRenderer: render.NewMarkdown(),
		Notifier:       notify.Webhook{URL: cfg.NotifyWebhookURL, Format: cfg.NotifyWebhookFormat},
		Settings:       sqlite.NewSettingsRepo(db),
		Clock:          shared.SystemClock{},
	}
	if cfg.LLMAPIKey != "" && cfg.LLMBaseURL != "" && cfg.LLMModelA != "" {
		opts := []llm.Option{}
		if cfg.Thinking != "" {
			opts = append(opts, llm.WithThinking(cfg.Thinking))
		}
		g, lerr := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModelA, cfg.LLMModelB, llm.EmbedConfig{
			BaseURL:    cfg.EmbedBaseURL,
			APIKey:     cfg.EmbedAPIKey,
			Model:      cfg.LLMEmbed,
			Dimensions: cfg.EmbedDims,
			Style:      llm.EmbedStyle(cfg.EmbedStyle),
		}, opts...)
		if lerr != nil {
			db.Close()
			return application.Deps{}, nil, lerr
		}
		// 预算装饰器：按阶段记账 + 熔断，透明实现 LLMGateway
		guard := application.NewBudgetGuard(g, usageRepo, budget, shared.SystemClock{})
		g.SetUsageRecorder(func(u llm.Usage) {
			guard.RecordUsage(u.Kind, u.Model, u.PromptTokens, u.CompletionTokens)
		})
		deps.LLM = guard
	}
	return deps, db, nil
}

// Run 执行一次完整流水线并落盘日报与站点。
func Run(cfg *config.Config) error {
	deps, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	started := time.Now()

	ctx, cancel := signalCtx()
	defer cancel()

	res, err := application.RunPipeline(ctx, deps)
	finish := time.Now()
	status := "ok"
	if err != nil {
		status = "error"
	}
	statsJSON := fmt.Sprintf(`{"collected":%d,"written":%d,"stories":%d,"fusion":%d,"durationSeconds":%.1f}`,
		res.Collect.Inserted, res.Select.Written, res.Cluster.NewStories+res.Cluster.Merged, res.Fusion.Links, res.DurationSec)
	_ = db.RecordRun(started, finish, status, statsJSON)
	if err != nil {
		return err
	}

	// 落盘：日报 Markdown + 双榜站点（服务器部署/Actions 提交都吃这两份产物）
	digestDir := filepath.Join(cfg.DataDir, "digests")
	if err := os.MkdirAll(digestDir, 0o755); err != nil {
		return err
	}
	digestPath := filepath.Join(digestDir, res.DigestDate+".md")
	if werr := os.WriteFile(digestPath, []byte(res.DigestMD), 0o644); werr != nil {
		return fmt.Errorf("写日报文件: %w", werr)
	}
	printResult(res, digestPath)
	return nil
}

// Serve 启动 API + 双榜页 + 内置定时调度。
func Serve(cfg *config.Config) error {
	deps, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := &httpapi.Server{Deps: deps, Runs: runsRepo{db}, Admin: adminSessions{db}, Version: Version}
	srv.Guard = httpapi.NewIPGuard(guardStore{db}) // 三层 IP 身份防护

	// 健康探针：启动后首轮探测，之后每 ProbeIntervalHours（默认 6h）一轮。
	// 覆盖全部启用信源 + 关键端点（LLM 网关 / GitHub API / 音乐上游 / 背景对象存储 / 本地库）。
	storageBase := os.Getenv("R2_PUBLIC_BASE")
	if storageBase == "" {
		storageBase = "https://wanghaodatastorage.dpdns.org/githubhot"
	}
	probeSvc := &prober.Service{
		Deps:  deps,
		Store: db,
		Targets: prober.EndpointTargets{
			LLMBaseURL:  cfg.LLMBaseURL,
			LLMAPIKey:   cfg.LLMAPIKey,
			GitHubToken: cfg.GitHubToken,
			StorageBase: storageBase,
			MusicID:     cfg.MusicPlaylist,
		},
		PingDB: func(ctx context.Context) error {
			var one int
			return db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
		},
	}
	addr := "0.0.0.0:" + cfg.Port
	fmt.Printf("GithubHot %s · API+双榜页监听 http://localhost:%s\n", Version, cfg.Port)
	fmt.Printf("APP/前端契约：/api/v1/hot/github · /api/v1/hot/news · /api/v1/hot/fusion · /api/v1/digest/latest\n")

	// 内置调度：到点自动跑流水线（服务器常驻模式）
	scheduler := newScheduler(cfg.CronSpec, func() {
		fmt.Printf("[cron] 到点触发，开始跑流水线（%s）\n", time.Now().Format("15:04:05"))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if _, err := application.RunPipeline(ctx, deps); err != nil {
			fmt.Printf("[cron] 流水线失败: %v\n", err)
		} else {
			fmt.Printf("[cron] 流水线完成，日报已更新\n")
		}
	})
	scheduler.start()
	defer scheduler.stop()

	ctx, stop := signalCtx()
	defer stop()

	srv.Probes = probeBridge{probeSvc}
	probeSvc.Start(ctx, time.Duration(cfg.ProbeIntervalHours)*time.Hour)
	return httpListen(ctx, addr, srv.Router())
}

// MCP 启动 stdio MCP 服务器（供 Claude 等 Agent 客户端接入）。
func MCP(cfg *config.Config) error {
	deps, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, stop := signalCtx()
	defer stop()
	srv := &mcp.Server{Deps: deps, Version: Version}
	return srv.Run(ctx)
}

// Bench SelectBench 精选校准：JSONL 样本从 stdin 读入（无文件路径参数，杜绝路径穿越）。
// 每行一个 JSON {"text":"...","label":"pass|drop"}。
func Bench(cfg *config.Config, in io.Reader) error {
	deps, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("读取样本: %w", err)
	}
	var samples []application.SelectBenchSample
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var smp application.SelectBenchSample
		if err := json.Unmarshal([]byte(line), &smp); err != nil {
			return fmt.Errorf("样本第 %d 行解析失败: %w", i+1, err)
		}
		samples = append(samples, smp)
	}
	ctx, cancel := signalCtx()
	defer cancel()
	rep, err := application.RunSelectBench(ctx, deps, samples)
	if err != nil && rep == nil {
		return err
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(out))
	if rep != nil {
		fmt.Printf("\n精确率 %.1f%% · 召回率 %.1f%% · F1 %.1f%%（样本 %d）\n",
			rep.Precision*100, rep.Recall*100, rep.F1*100, rep.Total)
	}
	return nil
}

// Push 外部脚本推送一条资料到 script 信源。
func Push(cfg *config.Config, sourceID, rawURL, title, summary string) error {
	deps, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	// script-push 信源尚不存在时先种子化
	if _, serr := deps.Sources.FindByID(context.Background(), sourceID); serr != nil {
		if _, ierr := application.SeedSources(context.Background(), deps); ierr != nil {
			return ierr
		}
	}
	ctx, cancel := signalCtx()
	defer cancel()
	it, err := application.PushItem(ctx, deps, sourceID, rawURL, title, summary)
	if err != nil {
		return err
	}
	fmt.Printf("已推送：%s\n  id=%s\n", it.Title, it.ID)
	return nil
}

func signalCtx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// runsRepo 适配 sqlite 运行记录到应用层端口。
type runsRepo struct{ db *sqlite.DB }

func (r runsRepo) List(ctx context.Context, limit int) ([]application.RunRow, error) {
	rows, err := r.db.ListRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]application.RunRow, 0, len(rows))
	for _, rr := range rows {
		c, w, s := application.DecodeRunStats(rr.Stats)
		out = append(out, application.RunRow{
			StartedAt: rr.StartedAt, Status: rr.Status, Duration: rr.Duration,
			Collected: c, Written: w, Stories: s,
		})
	}
	return out, nil
}

// probeBridge 适配探针服务到 httpapi.ProbeReader 端口。
type probeBridge struct{ svc *prober.Service }

func (b probeBridge) LatestProbes(ctx context.Context) ([]httpapi.ProbeRow, error) {
	rows, err := b.svc.Store.LatestProbes(ctx)
	if err != nil {
		return nil, err
	}
	return toProbeRows(rows), nil
}

func (b probeBridge) ProbeHistory(ctx context.Context, target string, limit int) ([]httpapi.ProbeRow, error) {
	rows, err := b.svc.Store.ProbeHistory(ctx, target, limit)
	if err != nil {
		return nil, err
	}
	return toProbeRows(rows), nil
}

func (b probeBridge) RunProbes(ctx context.Context) {
	b.svc.Run(ctx)
}

func toProbeRows(rows []sqlite.ProbeRecord) []httpapi.ProbeRow {
	out := make([]httpapi.ProbeRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.ProbeRow{
			Target: r.Target, Kind: r.Kind, OK: r.OK,
			LatencyMS: r.LatencyMS, Detail: r.Detail, CheckedAt: r.CheckedAt,
		})
	}
	return out
}

// adminSessions 适配 sqlite 会话存储到 httpapi.AdminSessions 端口。
type adminSessions struct{ db *sqlite.DB }
func (a adminSessions) CreateAdminSession(ctx context.Context, id, ip string, ttl time.Duration) error {
	return a.db.CreateAdminSession(ctx, id, ip, ttl)
}

func (a adminSessions) FindAdminSession(ctx context.Context, id string) (created, expires time.Time, ip string, found bool, err error) {
	s, err := a.db.FindAdminSession(ctx, id)
	if err != nil || s == nil {
		return time.Time{}, time.Time{}, "", false, err
	}
	return s.CreatedAt, s.ExpiresAt, s.IP, true, nil
}

func (a adminSessions) RenewAdminSession(ctx context.Context, id string, ttl time.Duration) error {
	return a.db.RenewAdminSession(ctx, id, ttl)
}

func (a adminSessions) DeleteAdminSession(ctx context.Context, id string) error {
	return a.db.DeleteAdminSession(ctx, id)
}

func printResult(res *application.PipelineResult, digestPath string) {
	fmt.Println("\n========== 流水线结果 ==========")
	fmt.Printf("采集：到期信源 %d，新条目 %d（重复 %d，失败 %d）\n",
		res.Collect.DueSources, res.Collect.Inserted, res.Collect.Duplicates, res.Collect.ErrorCount)
	fmt.Printf("GitHub 发现：Search %d + Trending %d，新项目 %d\n",
		res.Discover.SearchRepos, res.Discover.TrendingRepos, res.Discover.NewProjects)
	fmt.Printf("精选：候选 %d → 预筛通过 %d → 双评分通过 %d → 完成写作 %d\n",
		res.Select.Candidates, res.Select.Prefiltered, res.Select.Scored, res.Select.Written)
	fmt.Printf("聚簇：新事件 %d，合并 %d；融合链接 %d\n",
		res.Cluster.NewStories, res.Cluster.Merged, res.Fusion.Links)
	fmt.Printf("榜单：GitHub %d 项 / AI 资讯 %d 项 / 融合 %d 对\n",
		len(res.View.GitHub), len(res.View.News), len(res.View.Fusion))
	fmt.Printf("日报：%s（耗时 %.1fs，模型 %s）\n", digestPath, res.DurationSec, res.ModelA)
}
