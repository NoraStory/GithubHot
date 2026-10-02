// Package cli 是组合根（composition root）：在这里把领域仓储、
// 端口实现与用例装配起来。装配知识只存在于接口层。
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/infrastructure/fetcher"
	"github.com/NoraStory/GithubHot/internal/infrastructure/githubapi"
	"github.com/NoraStory/GithubHot/internal/infrastructure/llm"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
	"github.com/NoraStory/GithubHot/internal/infrastructure/render"
	"github.com/NoraStory/GithubHot/internal/interfaces/httpapi"
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
		GitHub:         githubapi.New(cfg.GitHubToken),
		Fetchers:       fetcher.NewRegistry(),
		DigestRenderer: render.NewMarkdown(),
		SiteRenderer:   render.NewSite(),
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
	siteDir := filepath.Join(cfg.DataDir, "site")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		return err
	}
	if res.SiteHTML != "" {
		if werr := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte(res.SiteHTML), 0o644); werr != nil {
			return fmt.Errorf("写站点文件: %w", werr)
		}
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

	srv := &httpapi.Server{Deps: deps, Runs: runsRepo{db}, Version: Version}
	addr := "0.0.0.0:" + cfg.Port
	fmt.Printf("GithubHot %s · API+双榜页监听 http://localhost:%s\n", Version, cfg.Port)
	fmt.Printf("APP/前端契约：/api/v1/hot/github · /api/v1/hot/news · /api/v1/hot/fusion · /api/v1/digest/latest\n")

	// 内置调度：到点自动跑流水线（服务器常驻模式）
	scheduler := newScheduler(cfg.CronSpec, func() {
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
	return httpListen(ctx, addr, srv.Router())
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
