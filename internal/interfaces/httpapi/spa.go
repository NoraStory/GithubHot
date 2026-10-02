package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/interfaces/webui"
)

// spa 提供 Vue3 单页应用：静态资源直出，未匹配路径回退 index.html（历史模式路由）。
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(webui.FS(), "dist")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" || p == "." {
		p = "index.html"
	}
	if data, err := fs.ReadFile(dist, p); err == nil {
		w.Header().Set("Content-Type", contentType(p))
		_, _ = w.Write(data)
		return
	}
	// SPA 回退
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		// dist 未构建：给出明确提示而非空白
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<h1>GithubHot</h1><p>前端未构建：请先执行 <code>cd web && npm ci && npm run build</code>，并把 <code>web/dist</code> 同步到 <code>internal/interfaces/webui/dist</code>。</p>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

func contentType(p string) string {
	switch {
	case strings.HasSuffix(p, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(p, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(p, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(p, ".png"):
		return "image/png"
	case strings.HasSuffix(p, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

// siteConfig 站点配置（前端歌单等）：MUSIC_PLAYLIST 环境变量为 JSON 数组。
func (s *Server) siteConfig(w http.ResponseWriter, _ *http.Request) {
	music := []any{}
	if raw := strings.TrimSpace(os.Getenv("MUSIC_PLAYLIST")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &music); err != nil {
			music = []any{}
		}
	}
	writeJSON(w, 200, map[string]any{
		"music":     music,
		"siteName":  "GithubHot",
		"poweredBy": "Go + Vue3",
	})
}

// digestsAPI 期刊分页列表（用户端期刊页 + 管理端期刊页）。
func (s *Server) digestsAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	page := atoiDefault(q.Get("page"), 1)
	size := atoiDefault(q.Get("pageSize"), 10)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 10
	}
	k := digest.Kind(kind)
	if kind == "" || kind == "all" {
		k = "" // 空值由仓储层语义决定：此处手工合并三类
	}

	var all []digest.Digest
	if k == "" {
		for _, kk := range []digest.Kind{digest.KindDaily, digest.KindWeekly, digest.KindMonthly} {
			list, err := s.Deps.Digests.List(s.ctx(), kk, 200)
			if err == nil {
				all = append(all, list...)
			}
		}
		// 按期号降序（同前缀内字典序即时间序）
		for i := 0; i < len(all); i++ {
			for j := i + 1; j < len(all); j++ {
				if all[j].Date > all[i].Date {
					all[i], all[j] = all[j], all[i]
				}
			}
		}
	} else {
		list, err := s.Deps.Digests.List(s.ctx(), k, 200)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		all = list
	}

	total := len(all)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	items := []digest.Digest{}
	if start < end {
		items = all[start:end]
	}
	writeJSON(w, 200, map[string]any{
		"total": total,
		"page":  page,
		"items": items,
	})
}

func atoiDefault(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 && s == "" {
		return def
	}
	return n
}

var _ = application.HotView{}
var _ = os.Getenv
