package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/interfaces/webui"
)

// spa 提供 Vue3 单页应用：静态资源直出，未匹配路径回退 index.html（历史模式路由）。
// /video/ 前缀优先走磁盘目录（VIDEOS_DIR，默认 ./videos）：背景视频体积大，
// 不进 embed 也不进 git，本地毫秒级加载；目录缺文件时回退 embed。
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
	if strings.HasPrefix(p, "video/") {
		if serveDiskVideo(w, r, strings.TrimPrefix(p, "video/")) {
			return
		}
		// 磁盘没有该文件 → 继续走 embed（兼容旧部署）
	}
	if data, err := fs.ReadFile(dist, p); err == nil {
		w.Header().Set("Content-Type", contentType(p))
		// 带哈希的资源可长缓存；其余静态资源与页面禁用缓存（embed 无 Last-Modified，
		// no-cache 无法真正回源校验，必须 no-store），避免部署后浏览器拿到旧版入口页
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
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
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

// serveDiskVideo 从 VIDEOS_DIR 提供 /video/ 下的媒体文件；未配置时默认
// 取可执行文件旁的 videos/ 目录（不依赖进程工作目录，Windows 服务化也稳）。
// 命中返回 true；目录不可用或文件不存在返回 false，由调用方回退 embed。
func serveDiskVideo(w http.ResponseWriter, r *http.Request, name string) bool {
	dir := strings.TrimSpace(os.Getenv("VIDEOS_DIR"))
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return false
		}
		dir = filepath.Join(filepath.Dir(exe), "videos")
	} else if !filepath.IsAbs(dir) {
		if exe, err := os.Executable(); err == nil {
			dir = filepath.Join(filepath.Dir(exe), dir)
		}
	}
	// path.Clean 已处理 ..，这里再拒绝绝对路径与越界分隔符，双保险
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return false
	}
	full := filepath.Join(dir, filepath.FromSlash(name))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return false
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, full)
	return true
}

// favicon 站点图标：dist 只内嵌 SVG 图标，老式客户端硬请求 /favicon.ico 时以同内容回退。
func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(webui.FS(), "dist")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(dist, "favicon.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
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
	case strings.HasSuffix(p, ".webp"):
		return "image/webp"
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
	homeVideos := strings.TrimSpace(os.Getenv("HOME_VIDEOS"))
	if homeVideos == "" {
		// 默认片单：前 6 个远程黑白老电影 + R2 托管域名上的本地新增横屏（x1~x6 与远程重复已剔除）
		homeVideos = strings.Join([]string{
			"https://pic.lololowe.com/video/x/1.mp4", "https://pic.lololowe.com/video/x/2.mp4",
			"https://pic.lololowe.com/video/x/3.mp4", "https://pic.lololowe.com/video/x/4.mp4",
			"https://pic.lololowe.com/video/x/5.mp4", "https://pic.lololowe.com/video/x/6.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/x/x7.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/x/x8.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/x/x10.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/x/x11.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/x/x12.mp4",
		}, "|")
	}
	// 竖屏片单：R2 上的本地 y 系列（y1~y12，无 y9）
	homePortraitVideos := strings.TrimSpace(os.Getenv("HOME_PORTRAIT_VIDEOS"))
	if homePortraitVideos == "" {
		homePortraitVideos = strings.Join([]string{
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y1.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y2.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y3.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y4.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y5.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y6.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y7.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y8.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y10.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y11.mp4",
			"https://wanghaodatastorage.dpdns.org/githubhot/y/y12.mp4",
		}, "|")
	}
	writeJSON(w, 200, map[string]any{
		"music":              music,
		"siteName":           "GithubHot",
		"poweredBy":          "Go + Vue3",
		"homeVideos":         homeVideos,
		"homePortraitVideos": homePortraitVideos,
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
