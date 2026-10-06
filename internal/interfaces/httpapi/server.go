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
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/NoraStory/GithubHot/internal/infrastructure/sidecarclient"
	"sync"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/domain/item"
)

// Server 依赖注入。
type Server struct {
	Deps    application.Deps
	Runs    application.RunRepo
	Admin   AdminSessions // 管理端会话存储（nil 时管理端仅支持旧令牌/开放模式）
	Guard   *IPGuard      // 三层 IP 身份防护（nil = 不启用）
	AppGuard *AppGuard    // APP 签名校验/指纹归档/远程封禁执行（nil = 不启用）
	Images  *ImageResolver  // 卡片封面 og:image 懒抓取缓存（nil = 不下发图片）
	Favicons *FaviconService // 信源 favicon 瓦片代理（nil = 无图卡片不回退图标）
	Probes  ProbeReader   // 健康探针（nil = 未启用，管理端探针页不可用）
	AttestNonces AttestNonces // P4-1 平台证明 nonce 存储（nil = 未启用，attest 端点 503）
	WebAuthn    *webauthn.WebAuthn // P4-2 通行密钥（nil = 未启用，passkey 端点 503）
	PasskeyStore PasskeyStore      // P4-2 通行密钥存储（与 WebAuthn 成对）

	passkeyOnce sync.Once
	passkeyMaps *passkeySessionStore

	sidecarOnce   sync.Once
	sidecarClient *sidecarclient.Client
	TLSMode bool          // P3-1：serve 以 TLS 运行（证书或 ACME）→ HSTS 中间件启用
	Version string
}

// hstsMiddleware TLS 模式专用：告知浏览器后续访问强制 HTTPS（规格书 P3-1）。
func hstsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// Router 构建路由。
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors)
	if s.TLSMode {
		r.Use(hstsMiddleware) // P3-1：仅 TLS 模式启用 HSTS
	}
	if s.Guard != nil {
		r.Use(s.Guard.Middleware) // 三层 IP 防护：封禁 404 → 速率记录 → 身份核验
	}
	if s.AppGuard != nil {
		r.Use(s.AppGuard.Middleware) // APP 请求：签名校验 + 指纹归档 + 远程封禁/强更
	}
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": s.Version})
	})
	r.Get("/favicon.ico", s.favicon)
	r.Get("/llms.txt", s.llmsTxt)
	r.Get("/privacy", s.privacyPageAPI) // 隐私政策页面（SPA路由）
		r.Get("/api/v1/site/config", s.siteConfig)
		r.Get("/api/v1/music/playlist", s.musicPlaylist)
		r.Get("/api/v1/gh/avatar/{owner}", s.ghAvatar)
		r.Get("/api/v1/digests", s.digestsAPI)
	// Vue3 SPA：静态资源 + 历史模式回退
	r.Get("/", s.spa)
	r.NotFound(s.spa)

	// RSS 输出：同一份内容给订阅器和 Agent 用
	r.Get("/feed/news.xml", s.feedNews)
	r.Get("/feed/github.xml", s.feedGitHub)
	r.Get("/feed/digest.xml", s.feedDigest)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/hot/github", s.hotGitHub)
		r.Get("/hot/news", s.hotNews)
		r.Get("/hot/domestic", s.hotDomestic)
		r.Get("/hot/fusion", s.hotFusion)
		r.Get("/hot", s.hotAll)
		r.Get("/digest/latest", s.digestLatest)
		r.Get("/digest/{date}", s.digestByDate)
		r.Get("/sources", s.sources)
		r.Get("/search", s.searchAPI)
		r.Get("/agent/hot.md", s.agentMD)
		r.Get("/story/{id}", s.storyAPI)
		r.Get("/stories", s.storiesArchiveAPI)
		r.Get("/favicon", s.faviconAPI)
		s.registerAdminRoutes(r)
		s.registerIPGuardRoutes(r)
	})
	return r
}

// ---------- handlers ----------

func (s *Server) buildView(ctx context.Context) (application.HotView, error) {
	return application.BuildHotView(ctx, s.Deps, digest.KindDaily)
}

func (s *Server) hotGitHub(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"generatedAt": v.Generated, "items": v.GitHub})
}

func (s *Server) hotNews(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	for i := range v.News {
		v.News[i].Image = s.Images.Resolve(v.News[i].URL)
	}
	writeJSON(w, 200, map[string]any{"generatedAt": v.Generated, "items": v.News})
}

// hotDomestic 国内热榜：实时计算的轻管道视图（多源共振，无 LLM）+ 当日综述。
func (s *Server) hotDomestic(w http.ResponseWriter, r *http.Request) {
	v, err := application.BuildDomesticView(r.Context(), s.Deps, 0)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	for i := range v.Items {
		v.Items[i].Image = s.Images.Resolve(v.Items[i].URL)
	}
	writeJSON(w, 200, map[string]any{
		"generatedAt": v.Generated,
		"items":       v.Items,
		"summary":     application.TodayDomesticSummary(r.Context(), s.Deps),
	})
}

func (s *Server) hotFusion(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"generatedAt": v.Generated, "pairs": v.Fusion})
}

func (s *Server) hotAll(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) digestLatest(w http.ResponseWriter, r *http.Request) {
	d, err := s.Deps.Digests.Latest(r.Context(), digest.KindDaily)
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
	d, err := s.Deps.Digests.FindByDate(r.Context(), date)
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
	list := s.sourceInfos(r.Context())
	writeJSON(w, 200, map[string]any{"items": list})
}

// sourceInfos 信源展示行（/api/v1/sources 与控制台共用）。
func (s *Server) sourceInfos(ctx context.Context) []SourceInfoDTO {
	all, err := s.Deps.Sources.All(ctx)
	if err != nil {
		return nil
	}
	out := make([]SourceInfoDTO, 0, len(all))
	for _, src := range all {
		adapter := "builtin"
		if !src.Kind.Implemented() {
			adapter = "extension-point"
		}
		lastFetched := ""
		if src.LastFetchedAt != nil {
			lastFetched = src.LastFetchedAt.Format("01-02 15:04")
		}
		out = append(out, SourceInfoDTO{
			ID: src.ID, Name: src.Name, Kind: string(src.Kind), Tier: string(src.Tier),
			Tags: src.Tags, Enabled: src.Enabled, Adapter: adapter, Config: src.Config,
			CurrentInterval: src.CurrentIntervalMinutes, EmptyStreak: src.EmptyStreak,
			LastFetchedAt: lastFetched,
		})
	}
	return out
}

// SourceInfoDTO 信源展示行。
type SourceInfoDTO struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Tier            string            `json:"tier"`
	Tags            []string          `json:"tags"`
	Enabled         bool              `json:"enabled"`
	Adapter         string            `json:"adapter"`
	Config          map[string]string `json:"config,omitempty"`
	CurrentInterval int               `json:"currentIntervalMinutes"`
	EmptyStreak     int               `json:"emptyStreak"`
	LastFetchedAt   string            `json:"lastFetchedAt,omitempty"`
}

// ---------- 搜索 ----------

func (s *Server) searchAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, err := application.Search(r.Context(), s.Deps, q, 30)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	// 空结果返回 [] 而不是 null
	if results == nil {
		results = []application.SearchResult{}
	}
	writeJSON(w, 200, map[string]any{"query": q, "results": results})
}

// usageAPI Token 用量 JSON（APP/运维）。
func (s *Server) usageAPI(w http.ResponseWriter, r *http.Request) {
	v, err := application.UsageOverview(r.Context(), s.Deps.Usage, s.Deps.Budget, s.Deps.Clock)
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
	items, err := s.Deps.Items.ByStage(r.Context(), []item.Stage{stage}, 50)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	names := map[string]string{}
	if all, err := s.Deps.Sources.All(r.Context()); err == nil {
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
func (s *Server) runsAPI(w http.ResponseWriter, r *http.Request) {
	if s.Runs == nil {
		writeJSON(w, 200, map[string]any{"items": []any{}})
		return
	}
	rows, err := application.ListRuns(r.Context(), s.Runs, 20)
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

// ---------- helpers ----------

type errorString string

func (e errorString) Error() string { return string(e) }

var errNotFound = errorString("not found")

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
