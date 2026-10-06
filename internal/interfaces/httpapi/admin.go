package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// contextWithTimeout 组合根便捷包装。
func contextWithTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 30*time.Second)
}

// sprintf fmt.Sprintf 别名（本文件内部使用）。
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// adminAuth 的旧版独立函数已移除，统一用 Server.adminAuth（见 auth.go）。

// registerAdminRoutes 管理端点：登录/退出/会话检查 + 信源 CRUD + 试抓、脚本推送、事件锁定。
// 鉴权中间件统一为 Server.adminAuth（密码会话模式 / 旧令牌模式 / 开放模式）。
func (s *Server) registerAdminRoutes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Post("/login", s.adminLogin)
		r.Get("/session", s.adminSessionCheck)
		// P4-2 通行密钥免密登录入口（守卫外；成功后复用 gh_admin_session 通道）
		r.Post("/passkey/begin-login", s.beginPasskeyLogin)
		r.Post("/passkey/finish-login", s.finishPasskeyLogin)
		r.Group(func(r chi.Router) {
			r.Use(s.adminAuth)
			r.Post("/logout", s.adminLogout)
			r.Post("/sources", s.createSource)
			r.Delete("/sources/{id}", s.deleteSource)
			r.Post("/sources/test", s.testSource)
			r.Post("/push", s.pushItem)
			r.Post("/story/{id}/lock", s.lockStory)
			// 控制台数据（Token 用量 / 诊断 / 运行历史）
			r.Get("/usage", s.usageAPI)
			r.Get("/diagnostics", s.diagnosticsAPI)
			r.Get("/runs", s.runsAPI)
			// IP 防护（封禁墙对这组端点放行，给被封管理员自救通道）
			r.Get("/ipguard/summary", s.ipGuardSummaryAPI)
			r.Post("/ipguard/ban", s.ipGuardBanAPI)
			r.Post("/ipguard/unban", s.ipGuardUnbanAPI)
			r.Get("/ipguard/ip", s.ipGuardIPDetailAPI)
			// 国内热榜 Top10 综述开关
			r.Get("/domestic-summary", s.domesticSummaryStatus)
			r.Post("/domestic-summary", s.domesticSummaryToggle)
			// 健康探针（最新结果 / 历史 / 立即探测）
			r.Get("/probes", s.probesAPI)
			r.Post("/probes/run", s.probesRunAPI)
			// 资源监测（进程 RSS / CPU / Go 运行时 / sidecar 状态）
			r.Get("/system/stats", s.systemStatsAPI)
			// P5-1 标注（金标签，source=admin）
			r.Post("/fp/label", s.fpLabelAPI)
			// §10.2 Review 队列
			r.Get("/review/queue", s.reviewQueueAPI)
			r.Post("/review/resolve", s.reviewResolveAPI)
			// §10.5 ML 诊断
			r.Get("/ml/diagnostics", s.mlDiagnosticsAPI)
			// P4-2 通行密钥管理（注册需已登录）
			r.Post("/passkey/begin-register", s.beginPasskeyRegister)
			r.Post("/passkey/finish-register", s.finishPasskeyRegister)
			r.Get("/passkey/credentials", s.listPasskeyCredentials)
			r.Delete("/passkey/credentials/{id}", s.deletePasskeyCredential)
		})
	})
}

type sourcePayload struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	URL             string            `json:"url"`
	Tier            string            `json:"tier"`
	Tags            []string          `json:"tags"`
	IntervalMinutes int               `json:"intervalMinutes"`
	Enabled         *bool             `json:"enabled"`
	Config          map[string]string `json:"config"`
	Dry             bool              `json:"dry"`
}

func (p sourcePayload) toSource(now time.Time) source.Source {
	cfg := p.Config
	if cfg == nil {
		cfg = map[string]string{}
	}
	if p.URL != "" {
		cfg["url"] = p.URL
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	if p.Tier == "" {
		p.Tier = string(source.TierMedia)
	}
	return source.Source{
		ID: p.ID, Name: p.Name, Kind: source.Kind(p.Kind), Config: cfg,
		Tier: source.Tier(p.Tier), Tags: p.Tags, IntervalMinutes: p.IntervalMinutes,
		Enabled: enabled, CreatedAt: now,
	}
}

// createSource 新增/更新信源。
func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	var p sourcePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, err)
		return
	}
	src := p.toSource(time.Now())
	if err := src.Validate(); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.Deps.Sources.Save(r.Context(), src); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": src.ID})
}

// deleteSource 删除信源。
func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ar, ok := s.Deps.Sources.(source.AdaptiveRepository)
	if !ok {
		writeErr(w, 500, errorString("仓储不支持删除"))
		return
	}
	if err := ar.Delete(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// testSource 试抓：不落库，返回抓取预览（信源管理界面用）。
func (s *Server) testSource(w http.ResponseWriter, r *http.Request) {
	var p sourcePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, err)
		return
	}
	src := p.toSource(time.Now())
	if err := src.Validate(); err != nil {
		writeErr(w, 400, err)
		return
	}
	// 干跑：只校验配置（管理端登录令牌验证用），不发起真实抓取
	if p.Dry {
		writeJSON(w, 200, map[string]any{"ok": true, "dry": true})
		return
	}
	fetcher, err := s.Deps.Fetchers.Fetcher(src.Kind)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ftctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	raws, err := fetcher.Fetch(ftctx, src, time.Now())
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type preview struct {
		URL, Title, Summary string
	}
	out := make([]preview, 0, 5)
	for _, it := range raws {
		if len(out) >= 5 {
			break
		}
		out = append(out, preview{URL: it.URL, Title: it.Title, Summary: truncStr(it.Summary, 80)})
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(raws), "preview": out})
}

func truncStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// pushItem 脚本推送（外部采集脚本写入）。
func (s *Server) pushItem(w http.ResponseWriter, r *http.Request) {
	var p struct {
		SourceID string `json:"sourceId"`
		URL      string `json:"url"`
		Title    string `json:"title"`
		Summary  string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, err)
		return
	}
	it, err := application.PushItem(r.Context(), s.Deps, p.SourceID, p.URL, p.Title, p.Summary)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": it.ID})
}

// domesticSummaryStatus 国内热榜综述开关状态。
func (s *Server) domesticSummaryStatus(w http.ResponseWriter, r *http.Request) {
	if s.Deps.Settings == nil {
		writeErr(w, 500, errorString("设置仓储不可用"))
		return
	}
	date := time.Now().Format("2006-01-02")
	has, _, _ := s.Deps.Settings.DomesticSummary(r.Context(), date)
	writeJSON(w, 200, map[string]any{
		"enabled":     application.DomesticSummaryEnabled(r.Context(), s.Deps),
		"date":        date,
		"hasSummary":  has != "",
		"summaryDate": date,
	})
}

// domesticSummaryToggle 开/关国内热榜综述（不改变已生成的历史摘要）。
func (s *Server) domesticSummaryToggle(w http.ResponseWriter, r *http.Request) {
	if s.Deps.Settings == nil {
		writeErr(w, 500, errorString("设置仓储不可用"))
		return
	}
	var p struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, err)
		return
	}
	v := "0"
	if p.Enabled {
		v = "1"
	}
	if err := s.Deps.Settings.Set(r.Context(), application.SettingDomesticSummaryEnabled, v); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": p.Enabled})
}

// lockStory 人工锁定/解锁事件（锁定后聚簇不再自动合并，AIHOT 同款保护）。
func (s *Server) lockStory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p struct {
		Manual bool `json:"manual"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.Deps.Stories.SetManual(r.Context(), id, p.Manual); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "manual": p.Manual})
}

// ---------- Agent 出口 ----------

// llmsTxt 面向 Agent 的站点说明（llms.txt 约定）。
func (s *Server) llmsTxt(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	var b strings.Builder
	b.WriteString("# GithubHot\n\n")
	b.WriteString("GitHub 开源项目热点 × AI 资讯热点 × 国内热榜：三热度追踪站。本地流水线自动采集，LLM 预筛+双评分+中文写作，事件聚簇融合；国内热榜走多源共振轻管道。\n\n")
	b.WriteString("## 数据端点\n\n")
	b.WriteString("- GET /api/v1/hot/github — GitHub 项目热度榜（JSON）\n")
	b.WriteString("- GET /api/v1/hot/news — AI 资讯热度榜（JSON）\n")
	b.WriteString("- GET /api/v1/hot/domestic — 国内热榜（JSON）\n")
	b.WriteString("- GET /api/v1/hot/fusion — 资讯×项目融合配对\n")
	b.WriteString("- GET /api/v1/hot — 三榜合一\n")
	b.WriteString("- GET /api/v1/search?q= — 站内搜索\n")
	b.WriteString("- GET /api/v1/stories?page=&pageSize= — 全量事件分页（归档）\n")
	b.WriteString("- GET /api/v1/digest/latest?format=raw — 最新日报 Markdown\n")
	b.WriteString("- GET /feed/news.xml / /feed/github.xml / /feed/digest.xml — RSS\n\n")
	b.WriteString("## 当前热点速览\n\n")
	for i, p := range v.GitHub {
		if i >= 5 {
			break
		}
		fmtF(&b, "- GitHub #%d %s（24h +%d★）：%s\n", i+1, p.FullName, p.StarsGained, p.DescriptionZh)
	}
	for i, n := range v.News {
		if i >= 5 {
			break
		}
		fmtF(&b, "- AI #%d %s\n", i+1, n.TitleZh)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// agentMD Agent Markdown 报告：双榜完整 Markdown（llms 风格内容面）。
func (s *Server) agentMD(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	var b strings.Builder
	b.WriteString("# GithubHot 双热点报告\n\n")
	b.WriteString("生成于 " + v.Generated.Format("2006-01-02 15:04") + "\n\n")
	b.WriteString("## GitHub 项目热点\n\n")
	for _, p := range v.GitHub {
		desc := p.DescriptionZh
		if desc == "" {
			desc = p.Description
		}
		fmtF(&b, "%d. **[%s](%s)** +%d★（热度 %.1f）%s\n", p.Rank, p.FullName, p.URL, p.StarsGained, p.Hotness, desc)
	}
	b.WriteString("\n## AI 资讯热点\n\n")
	for _, n := range v.News {
		fmtF(&b, "%d. **[%s](%s)**（热度 %.1f）%s\n", n.Rank, n.TitleZh, n.URL, n.Hotness, n.SummaryZh)
		if n.Overview != "" {
			fmtF(&b, "   > %s\n", n.Overview)
		}
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// fmtF 极简 Fprintf 别名（保持本文件行宽）。
func fmtF(b *strings.Builder, format string, args ...any) {
	b.WriteString(sprintf(format, args...))
}

// storyPage 事件详情页。

// storyAPI 事件详情 JSON（APP 契约）。
func (s *Server) storyAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, err := application.BuildStoryDetail(r.Context(), s.Deps, id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, v)
}
