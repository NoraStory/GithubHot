package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
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
	// APP 安装包直出 + IP 防护：每 IP 限频（10 分钟 5 次），超限 429 并按违规事件
	// 计入 IP 守护积分体系（累计到线自动封禁）。封禁 IP 到不了这里——IPGuard
	// 中间件对封禁 IP 已全站 404（含 /app/），无需重复判封。
	if strings.HasPrefix(p, "app/") {
		if s.Guard != nil {
			ip := clientIPFromRequest(r)
			if r.Method != http.MethodHead && !appDlAllow(ip, time.Now()) {
				s.Guard.event(r.Context(), ip, "app-download-flood", "APP 安装包下载过于频繁", 2, false)
				writeJSON(w, 429, map[string]any{"error": "rate limited"})
				return
			}
		}
		serveDiskApk(w, r, strings.TrimPrefix(p, "app/"))
		return
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
// 本地命中直接直出；未命中走慢源代理缓存（首次回源 tee 落盘，之后本地直出，
// 见 video_cache.go）；代理不可用且为 x/y 键时 302 跳 R2 兜底；否则返回 false
// 由调用方回退 embed。
func serveDiskVideo(w http.ResponseWriter, r *http.Request, name string) bool {
	// path.Clean 已处理 ..，这里再拒绝绝对路径与越界分隔符，双保险
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return false
	}
	dir := videosDir()
	if dir == "" {
		return false
	}
	return serveVideoCached(w, r, dir, name)
}

// appDlLimiter APP 安装包下载限流：每 IP 滑动窗口（10 分钟 5 次）。
// 与 IP 守护联动：超限方除 429 外按违规事件计分，累计到线自动封禁。
var appDlLimiter = struct {
	sync.Mutex
	m map[string][]time.Time
}{m: map[string][]time.Time{}}

func appDlAllow(ip string, now time.Time) bool {
	const window = 10 * time.Minute
	const max = 5
	appDlLimiter.Lock()
	defer appDlLimiter.Unlock()
	ts := appDlLimiter.m[ip]
	cut := now.Add(-window)
	keep := ts[:0]
	for _, t := range ts {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(appDlLimiter.m, ip)
	}
	if len(keep) >= max {
		appDlLimiter.m[ip] = keep
		return false
	}
	appDlLimiter.m[ip] = append(keep, now)
	return true
}

// serveDiskApk 从 apksDir() 提供 /app/ 下的 APK 安装包（APP 内自动更新下载源）。
// 只允许 .apk 文件；不存在返回 404。
func serveDiskApk(w http.ResponseWriter, r *http.Request, name string) {
	if filepath.IsAbs(name) || strings.Contains(name, "..") || !strings.HasSuffix(name, ".apk") {
		http.NotFound(w, r)
		return
	}
	dir := apksDir()
	if dir == "" {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(dir, filepath.FromSlash(name))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(full)+"\"")
	http.ServeFile(w, r, full)
}

// apksDir APP 安装包目录：APK_DIR 环境变量优先（相对路径锚定可执行文件目录），
// 默认取可执行文件旁的 apks/。
func apksDir() string {
	dir := strings.TrimSpace(os.Getenv("APK_DIR"))
	if dir != "" && filepath.IsAbs(dir) {
		return dir
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if dir == "" {
		dir = "apks"
	}
	return filepath.Join(filepath.Dir(exe), dir)
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
		// 默认片单：全部走本地 /video/ 代理路径（video_cache.go 慢源缓存）——
		// x1~x6 回源主题作者源，x7+/y 系列回源自家 R2；首个访客触发预热落盘，
		// 之后所有访客本地毫秒级直出。不再把第三方慢源 URL 直接暴露给浏览器。
		homeVideos = strings.Join([]string{
			"/video/x/1.mp4", "/video/x/2.mp4",
			"/video/x/3.mp4", "/video/x/4.mp4",
			"/video/x/5.mp4", "/video/x/6.mp4",
			"/video/x/x7.mp4",
			"/video/x/x8.mp4",
			"/video/x/x10.mp4",
			"/video/x/x11.mp4",
			"/video/x/x12.mp4",
		}, "|")
	}
	// 竖屏片单：y 系列（y1~y12，无 y9），同样走本地代理路径
	homePortraitVideos := strings.TrimSpace(os.Getenv("HOME_PORTRAIT_VIDEOS"))
	if homePortraitVideos == "" {
		homePortraitVideos = strings.Join([]string{
			"/video/y/y1.mp4",
			"/video/y/y2.mp4",
			"/video/y/y3.mp4",
			"/video/y/y4.mp4",
			"/video/y/y5.mp4",
			"/video/y/y6.mp4",
			"/video/y/y7.mp4",
			"/video/y/y8.mp4",
			"/video/y/y10.mp4",
			"/video/y/y11.mp4",
			"/video/y/y12.mp4",
		}, "|")
	}
	writeJSON(w, 200, map[string]any{
		"music":              music,
		"siteName":           "GithubHot",
		"poweredBy":          "Go + Vue3",
		"homeVideos":         homeVideos,
		"homePortraitVideos": homePortraitVideos,
		// ---- APP 风控下发（启动握手通道）----
		// 注意：签名种子**绝不**在此下发（历史版本曾下发 gh-dev-seed-v1，等于公开
		// 派生密钥）。种子只经环境变量提供，APP 侧构建期注入。
		"banned":            os.Getenv("APP_BANNED") == "1" || strings.EqualFold(os.Getenv("APP_BANNED"), "true"),
		"force_upgrade_url": os.Getenv("APP_FORCE_UPGRADE_URL"),
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
			list, err := s.Deps.Digests.List(r.Context(), kk, 200)
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
		list, err := s.Deps.Digests.List(r.Context(), k, 200)
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
	items, total, err := s.Deps.Stories.ListPage(r.Context(), kind, (page-1)*size, size)
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
