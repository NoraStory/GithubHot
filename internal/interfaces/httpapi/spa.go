package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/domain/story"
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

// videosDir 本地视频目录：VIDEOS_DIR 环境变量优先（相对路径锚定可执行文件目录），
// 默认取可执行文件旁的 videos/。取不到可执行文件路径时返回空串。
func videosDir() string {
	dir := strings.TrimSpace(os.Getenv("VIDEOS_DIR"))
	if dir != "" && filepath.IsAbs(dir) {
		return dir
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if dir == "" {
		dir = "videos"
	}
	return filepath.Join(filepath.Dir(exe), dir)
}

// r2Base R2 后备方案的公开地址前缀（对象键为 x/x7.mp4、y/y4.mp4 风格）。
// R2_PUBLIC_BASE 环境变量可整体切换；留空则禁用后备。
func r2Base() string {
	if b := strings.TrimSpace(os.Getenv("R2_PUBLIC_BASE")); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	return "https://wanghaodatastorage.dpdns.org/githubhot"
}

// videoURL 生成片单/兜底用的视频地址：本地磁盘有文件用 /video/ 本地直出，
// 缺失时自动落到 R2 后备域名，保证换机器没拷 videos/ 也能放。
func videoURL(dir, localPath, r2Key string) string {
	if dir != "" {
		full := filepath.Join(dir, filepath.FromSlash(r2Key))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return localPath
		}
	}
	return r2Base() + "/" + r2Key
}

// serveDiskVideo 从 videosDir() 提供 /video/ 下的媒体文件。
// 命中返回 true；文件不存在且配置了 R2 后备时 302 跳 R2；否则返回 false 由调用方回退 embed。
func serveDiskVideo(w http.ResponseWriter, r *http.Request, name string) bool {
	// path.Clean 已处理 ..，这里再拒绝绝对路径与越界分隔符，双保险
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return false
	}
	dir := videosDir()
	if dir == "" {
		return false
	}
	full := filepath.Join(dir, filepath.FromSlash(name))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		// R2 后备：/video/x/x7.mp4 → {r2Base}/x/x7.mp4
		if base := r2Base(); base != "" && (strings.HasPrefix(name, "x/") || strings.HasPrefix(name, "y/")) {
			http.Redirect(w, r, base+"/"+strings.TrimPrefix(path.Clean("/"+name), "/"), http.StatusFound)
			return true
		}
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
		// 默认片单：前 6 个远程黑白老电影 + 本地/R2 横屏（磁盘有文件走本地直出，缺文件自动落 R2 后备）
		dir := videosDir()
		homeVideos = strings.Join([]string{
			"https://pic.lololowe.com/video/x/1.mp4", "https://pic.lololowe.com/video/x/2.mp4",
			"https://pic.lololowe.com/video/x/3.mp4", "https://pic.lololowe.com/video/x/4.mp4",
			"https://pic.lololowe.com/video/x/5.mp4", "https://pic.lololowe.com/video/x/6.mp4",
			videoURL(dir, "/video/x/x7.mp4", "x/x7.mp4"),
			videoURL(dir, "/video/x/x8.mp4", "x/x8.mp4"),
			videoURL(dir, "/video/x/x10.mp4", "x/x10.mp4"),
			videoURL(dir, "/video/x/x11.mp4", "x/x11.mp4"),
			videoURL(dir, "/video/x/x12.mp4", "x/x12.mp4"),
		}, "|")
	}
	// 竖屏片单：本地/R2 的 y 系列（y1~y12，无 y9）
	homePortraitVideos := strings.TrimSpace(os.Getenv("HOME_PORTRAIT_VIDEOS"))
	if homePortraitVideos == "" {
		dir := videosDir()
		homePortraitVideos = strings.Join([]string{
			videoURL(dir, "/video/y/y1.mp4", "y/y1.mp4"),
			videoURL(dir, "/video/y/y2.mp4", "y/y2.mp4"),
			videoURL(dir, "/video/y/y3.mp4", "y/y3.mp4"),
			videoURL(dir, "/video/y/y4.mp4", "y/y4.mp4"),
			videoURL(dir, "/video/y/y5.mp4", "y/y5.mp4"),
			videoURL(dir, "/video/y/y6.mp4", "y/y6.mp4"),
			videoURL(dir, "/video/y/y7.mp4", "y/y7.mp4"),
			videoURL(dir, "/video/y/y8.mp4", "y/y8.mp4"),
			videoURL(dir, "/video/y/y10.mp4", "y/y10.mp4"),
			videoURL(dir, "/video/y/y11.mp4", "y/y11.mp4"),
			videoURL(dir, "/video/y/y12.mp4", "y/y12.mp4"),
		}, "|")
	}
	writeJSON(w, 200, map[string]any{
		"music":              music,
		"siteName":           "GithubHot",
		"poweredBy":          "Go + Vue3",
		"homeVideos":         homeVideos,
		"homePortraitVideos": homePortraitVideos,
		// ---- APP 风控下发（启动握手通道）----
		"session_seed":        appSessionSeed(),
		"banned":              os.Getenv("APP_BANNED") == "1" || strings.EqualFold(os.Getenv("APP_BANNED"), "true"),
		"force_upgrade_url":   os.Getenv("APP_FORCE_UPGRADE_URL"),
	})
}

// appSessionSeed APP 会话种子：HMAC 派生根密钥，服务端签名校验与客户端派生
// 必须一致（生产经 KMS/CI 注入，绝不进仓库；本地缺省用开发默认值）。
func appSessionSeed() string {
	if v := strings.TrimSpace(os.Getenv("APP_SESSION_SEED")); v != "" {
		return v
	}
	return "gh-dev-seed-v1"
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

// storiesArchiveAPI 全量事件分页（归档页用）：GET /api/v1/stories?page=&pageSize=
// 只返回归档需要的轻量字段，默认仅资讯事件（kind=news）。
func (s *Server) storiesArchiveAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	size := atoiDefault(q.Get("pageSize"), 100)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 100
	}
	kind := story.KindNews
	if k := q.Get("kind"); k == "all" || k == string(story.KindProject) || k == string(story.KindDomestic) {
		kind = story.Kind(k)
	}
	items, total, err := s.Deps.Stories.ListPage(s.ctx(), kind, (page-1)*size, size)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	type row struct {
		StoryID     string    `json:"storyId"`
		TitleZh     string    `json:"titleZh"`
		FirstSeenAt time.Time `json:"firstSeenAt"`
	}
	rows := []row{}
	for _, it := range items {
		rows = append(rows, row{StoryID: it.ID, TitleZh: it.TitleZh, FirstSeenAt: it.FirstSeenAt})
	}
	writeJSON(w, 200, map[string]any{"total": total, "page": page, "items": rows})
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
