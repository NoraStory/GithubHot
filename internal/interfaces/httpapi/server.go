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
	"github.com/NoraStory/GithubHot/internal/infrastructure/render"
)

// Server 依赖注入。
type Server struct {
	Deps    application.Deps
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

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/hot/github", s.hotGitHub)
		r.Get("/hot/news", s.hotNews)
		r.Get("/hot/fusion", s.hotFusion)
		r.Get("/hot", s.hotAll)
		r.Get("/digest/latest", s.digestLatest)
		r.Get("/digest/{date}", s.digestByDate)
		r.Get("/sources", s.sources)
	})
	return r
}

// ---------- handlers ----------

func (s *Server) buildView() (application.HotView, error) {
	return application.BuildHotView(s.ctx(), s.Deps)
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
	d, err := s.Deps.Digests.Latest(s.ctx())
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
	all, err := s.Deps.Sources.All(s.ctx())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	type srcDTO struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Kind    string   `json:"kind"`
		Tier    string   `json:"tier"`
		Tags    []string `json:"tags"`
		Enabled bool     `json:"enabled"`
		Adapter string   `json:"adapter"`
	}
	out := make([]srcDTO, 0, len(all))
	for _, s := range all {
		adapter := "builtin"
		if !s.Kind.Implemented() {
			adapter = "extension-point"
		}
		out = append(out, srcDTO{s.ID, s.Name, string(s.Kind), string(s.Tier), s.Tags, s.Enabled, adapter})
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
