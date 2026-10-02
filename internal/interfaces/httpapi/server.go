// Package httpapi 是面向人与未来手机 APP 的 REST 接口层。
// 全部端点带 CORS 头与 /api/v1 版本前缀——APP 契约从这里开始。
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/infrastructure/render"
)

// Server 依赖注入。
type Server struct {
	Deps    application.Deps
	Runs    application.RunRepo
	Version string
}

// Router 构建路由。
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/", s.index)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": s.Version})
	})
	r.Get("/console", s.consolePage)
	r.Get("/search", s.searchPage)

	// RSS 输出：同一份内容给订阅器和 Agent 用
	r.Get("/feed/news.xml", s.feedNews)
	r.Get("/feed/github.xml", s.feedGitHub)
	r.Get("/feed/digest.xml", s.feedDigest)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/hot/github", s.hotGitHub)
		r.Get("/hot/news", s.hotNews)
		r.Get("/hot/fusion", s.hotFusion)
		r.Get("/hot", s.hotAll)
		r.Get("/digest/latest", s.digestLatest)
		r.Get("/digest/{date}", s.digestByDate)
		r.Get("/sources", s.sources)
		r.Get("/search", s.searchAPI)
		// 控制台数据（APP/运维消费）
		r.Get("/admin/usage", s.usageAPI)
		r.Get("/admin/diagnostics", s.diagnosticsAPI)
		r.Get("/admin/runs", s.runsAPI)
	})
	return r
}

// ---------- handlers ----------

func (s *Server) buildView() (application.HotView, error) {
	return application.BuildHotView(s.ctx(), s.Deps, digest.KindDaily)
}

func (s *Server) hotGitHub(w http.ResponseWriter, _ *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"generatedAt": v.Generated, "items": v.GitHub})
}

func (s *Server) hotNews(w http.ResponseWriter, _ *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"generatedAt": v.Generated, "items": v.News})
}

func (s *Server) hotFusion(w http.ResponseWriter, _ *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"generatedAt": v.Generated, "pairs": v.Fusion})
}

func (s *Server) hotAll(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) digestLatest(w http.ResponseWriter, r *http.Request) {
	d, err := s.Deps.Digests.Latest(s.ctx(), digest.KindDaily)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if d == nil {
		writeErr(w, 404, errNotFound)
		return
	}
	// ?format=raw 直接返回 markdown 文本
	if r.URL.Query().Get("format") == "raw" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(d.Markdown))
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) digestByDate(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	d, err := s.Deps.Digests.FindByDate(s.ctx(), date)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if d == nil {
		writeErr(w, 404, errNotFound)
		return
	}
	if r.URL.Query().Get("format") == "raw" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(d.Markdown))
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	list := s.sourceInfos()
	writeJSON(w, 200, map[string]any{"items": list})
}

// sourceInfos 信源展示行（/api/v1/sources 与控制台共用）。
func (s *Server) sourceInfos() []SourceInfoDTO {
	all, err := s.Deps.Sources.All(s.ctx())
	if err != nil {
		return nil
	}
	out := make([]SourceInfoDTO, 0, len(all))
	for _, src := range all {
		adapter := "builtin"
		if !src.Kind.Implemented() {
			adapter = "extension-point"
		}
		out = append(out, SourceInfoDTO{
			ID: src.ID, Name: src.Name, Kind: string(src.Kind), Tier: string(src.Tier),
			Tags: src.Tags, Enabled: src.Enabled, Adapter: adapter,
		})
	}
	return out
}

// SourceInfoDTO 信源展示行。
type SourceInfoDTO struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Tier    string   `json:"tier"`
	Tags    []string `json:"tags"`
	Enabled bool     `json:"enabled"`
	Adapter string   `json:"adapter"`
}

// ---------- 搜索 ----------

func (s *Server) searchAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, err := application.Search(s.ctx(), s.Deps, q, 30)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"query": q, "results": results})
}

func (s *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	start := time.Now()
	results, err := application.Search(s.ctx(), s.Deps, q, 30)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	v := application.SearchView{Query: q, Results: results, Took: time.Since(start).Round(time.Millisecond).String()}
	htmlOut, err := s.Deps.SiteRenderer.RenderSearch(s.ctx(), v)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(htmlOut))
}

// ---------- 控制台 ----------

var diagStages = []string{"written", "scored", "rejected", "filtered", "prefiltered", "dropped", "new"}

func (s *Server) consolePage(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildConsole(r)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	htmlOut, err := s.Deps.SiteRenderer.RenderConsole(s.ctx(), v)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(htmlOut))
}

func (s *Server) buildConsole(r *http.Request) (application.ConsoleView, error) {
	var v application.ConsoleView
	ctx := s.ctx()
	v.Usage, _ = application.UsageOverview(ctx, s.Deps.Usage, s.Deps.Budget, s.Deps.Clock)
	if s.Runs != nil {
		rows, err := application.ListRuns(ctx, s.Runs, 10)
		if err == nil {
			v.Runs = append(v.Runs, rows...)
		}
	}
	v.Stages = diagStages
	v.Stage = r.URL.Query().Get("stage")
	if v.Stage == "" {
		v.Stage = "written"
	}
	items, err := s.Deps.Items.ByStage(ctx, []item.Stage{item.Stage(v.Stage)}, 50)
	if err == nil {
		names := map[string]string{}
		if all, err := s.Deps.Sources.All(ctx); err == nil {
			for _, src := range all {
				names[src.ID] = src.Name
			}
		}
		for _, it := range items {
			v.Diagnostics = append(v.Diagnostics, application.DiagRow{
				ID: it.ID, Stage: string(it.Selection.Stage),
				SourceName: names[it.SourceID], Title: it.Title, TitleZh: it.Selection.TitleZh,
				URL: it.URL, Reason: it.Selection.Reason,
				ScoreA: it.Selection.ScoreA, ScoreB: it.Selection.ScoreB,
				Published: it.PublishedAt.Format("01-02 15:04"),
			})
		}
	}
	for _, src := range s.sourceInfos() {
		v.Sources = append(v.Sources, application.SourceInfo{
			ID: src.ID, Name: src.Name, Kind: src.Kind, Tier: src.Tier,
			Tags: src.Tags, Enabled: src.Enabled, Adapter: src.Adapter,
		})
	}
	v.SourcesCount = len(v.Sources)
	for _, kind := range []string{"daily", "weekly", "monthly"} {
		if dg, err := s.Deps.Digests.Latest(ctx, digest.Kind(kind)); err == nil && dg != nil {
			v.Digests = append(v.Digests, application.DigestMeta{Date: dg.Date, Kind: string(dg.Kind)})
		}
	}
	return v, nil
}

// usageAPI Token 用量 JSON（APP/运维）。
func (s *Server) usageAPI(w http.ResponseWriter, _ *http.Request) {
	v, err := application.UsageOverview(s.ctx(), s.Deps.Usage, s.Deps.Budget, s.Deps.Clock)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, v)
}

// diagnosticsAPI 内容诊断 JSON。
func (s *Server) diagnosticsAPI(w http.ResponseWriter, r *http.Request) {
	stage := item.Stage(r.URL.Query().Get("stage"))
	if stage == "" {
		stage = item.StageWritten
	}
	items, err := s.Deps.Items.ByStage(s.ctx(), []item.Stage{stage}, 50)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	names := map[string]string{}
	if all, err := s.Deps.Sources.All(s.ctx()); err == nil {
		for _, src := range all {
			names[src.ID] = src.Name
		}
	}
	type row struct {
		ID      string  `json:"id"`
		Stage   string  `json:"stage"`
		Source  string  `json:"source"`
		Title   string  `json:"title"`
		TitleZh string  `json:"titleZh"`
		URL     string  `json:"url"`
		Reason  string  `json:"reason"`
		ScoreA  float64 `json:"scoreA"`
		ScoreB  float64 `json:"scoreB"`
	}
	out := make([]row, 0, len(items))
	for _, it := range items {
		out = append(out, row{
			ID: it.ID, Stage: string(it.Selection.Stage), Source: names[it.SourceID],
			Title: it.Title, TitleZh: it.Selection.TitleZh, URL: it.URL,
			Reason: it.Selection.Reason, ScoreA: it.Selection.ScoreA, ScoreB: it.Selection.ScoreB,
		})
	}
	writeJSON(w, 200, map[string]any{"stage": string(stage), "items": out})
}

// runsAPI 运行历史 JSON。
func (s *Server) runsAPI(w http.ResponseWriter, _ *http.Request) {
	if s.Runs == nil {
		writeJSON(w, 200, map[string]any{"items": []any{}})
		return
	}
	rows, err := application.ListRuns(s.ctx(), s.Runs, 20)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	type runOut struct {
		StartedAt string  `json:"startedAt"`
		Status    string  `json:"status"`
		Duration  float64 `json:"durationSeconds"`
		Collected int     `json:"collected"`
		Written   int     `json:"written"`
		Stories   int     `json:"stories"`
	}
	out := make([]runOut, 0, len(rows))
	for _, rr := range rows {
		out = append(out, runOut{rr.StartedAt, rr.Status, rr.Duration, rr.Collected, rr.Written, rr.Stories})
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

// index 渲染双榜页（serve 模式实时渲染）。
func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	site := render.NewSite()
	html, err := site.RenderIndex(s.ctx(), v)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// ---------- helpers ----------

var errNotFound = errorString("not found")

type errorString string

func (e errorString) Error() string { return string(e) }

func (s *Server) ctx() context.Context { return context.Background() }

// cors 跨域中间件：白名单来自 CORS_ORIGINS 环境变量（逗号分隔）。
// 默认为空 = 不放行任何跨域（同源与原生 APP 不受影响）。
// 公开站点部署时把前端域名加入白名单即可。
func cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(os.Getenv("CORS_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// parseIntSafe 整数解析（未来分页用）。
func parseIntSafe(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
