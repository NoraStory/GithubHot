package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// /video/ 慢源代理缓存：背景视频轮播的卡顿根源是片单来源（主题作者个人源
// pic.lololowe.com 境内访问慢、R2 自定义域名跨境抖动）。本代理把首次访问的
// 回源流 tee 到本地磁盘，之后所有访客都吃毫秒级本地直出（Range 可用）。
//
// 语义：磁盘命中 → 本地直出；未命中 → 单飞回源（流式转发给当前客户端 + 落盘），
// 同名并发请求等待缓存就绪；回源失败/超时 → 302 跳 R2 兜底（客户端直连）。
// 下载用独立 context（不受 15s 请求超时约束）：客户端断开只停止转发，落盘继续，
// 下一个访客即命中。

const (
	videoCacheDirName = "cache"
	videoHTTPTimeout  = 5 * time.Minute
	videoWaitTimeout  = 12 * time.Second
	videoMaxBytes     = 2 << 30 // 2GB：超限按最旧文件驱逐（约 200 个 10MB 视频）
)

// videoKeyRe 只允许 x|y/<短字母数字>.mp4 回源（含本地命名 x7/y1 与远程命名 1..6），
// 防任意路径拼接成回源 URL。
var videoKeyRe = regexp.MustCompile(`^(x|y)/[a-z0-9]{1,4}\.mp4$`)

var videoHTTPClient = &http.Client{Timeout: videoHTTPTimeout}

// videoLocks 同名下载单飞：先到者回源，后来者等待其落盘。
var videoLocks = struct {
	sync.Mutex
	m map[string]*sync.Mutex
}{m: map[string]*sync.Mutex{}}

func videoLock(name string) *sync.Mutex {
	videoLocks.Lock()
	defer videoLocks.Unlock()
	if l, ok := videoLocks.m[name]; ok {
		return l
	}
	l := &sync.Mutex{}
	videoLocks.m[name] = l
	return l
}

// videoOriginURL 片单键的回源地址：x1~x6 只在主题作者源上，其余走自家 R2。
func videoOriginURL(name string) string {
	if strings.HasPrefix(name, "x/") {
		n := strings.TrimSuffix(strings.TrimPrefix(name, "x/"), ".mp4")
		if v, err := strconv.Atoi(n); err == nil && v >= 1 && v <= 6 {
			return "https://pic.lololowe.com/video/" + name
		}
	}
	if base := r2Base(); base != "" {
		return base + "/" + name
	}
	return ""
}

// videoCacheDir 视频缓存目录（videos/ 下 cache/ 子目录，确保存在）。
func videoCacheDir(dir string) string {
	d := filepath.Join(dir, videoCacheDirName)
	_ = os.MkdirAll(d, 0o755)
	return d
}

// serveVideoCached 主入口：返回 true 表示已响应（本地直出/回源流式/兜底 302）。
// 查找顺序：videos/<name>（人工管理的权威位置）→ cache/<扁平化名>（代理落盘）。
func serveVideoCached(w http.ResponseWriter, r *http.Request, dir, name string) bool {
	full := filepath.Join(dir, filepath.FromSlash(name))
	// 1) 本地命中：ServeContent 自带 Range（拖动/分段缓冲都在毫秒级）
	if fi, ok := statFile(full); ok {
		return serveCachedVideoFile(w, r, full, fi)
	}
	cacheName := strings.ReplaceAll(name, "/", "_")
	cacheFull := filepath.Join(videoCacheDir(dir), cacheName)
	if fi, ok := statFile(cacheFull); ok {
		return serveCachedVideoFile(w, r, cacheFull, fi)
	}
	if !videoKeyRe.MatchString(name) || r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	// 2) 回源单飞：抢锁者回源并落盘；抢不到的等待缓存就绪
	lock := videoLock(name)
	if lock.TryLock() {
		defer lock.Unlock()
		// 双检：等待期间可能已被前一个下载完
		if fi, ok := statFile(cacheFull); ok {
			return serveCachedVideoFile(w, r, cacheFull, fi)
		}
		return videoProxyStream(w, r, dir, name, cacheFull)
	}
	// 等待已在进行的下载（按 500ms 步进轮询文件，简单且不引入更多同步机制）
	deadline := time.Now().Add(videoWaitTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if fi, ok := statFile(cacheFull); ok {
			return serveCachedVideoFile(w, r, cacheFull, fi)
		}
	}
	// 仍未就绪：302 让客户端直连源站兜底（x/y 键均有对应远端）
	if src := videoOriginURL(name); src != "" {
		http.Redirect(w, r, src, http.StatusFound)
		return true
	}
	return false
}

func statFile(path string) (os.FileInfo, bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return nil, false
	}
	return fi, true
}

func serveCachedVideoFile(w http.ResponseWriter, r *http.Request, full string, fi os.FileInfo) bool {
	f, err := os.Open(full)
	if err != nil {
		return false
	}
	defer f.Close()
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, "video.mp4", fi.ModTime(), f)
	return true
}

// videoProxyStream 回源流式转发 + tee 落盘。客户端断开（请求 15s 超时/用户关页）
// 只停止转发，落盘继续完成——缓存预热一次性付清，后续访客全部命中本地。
func videoProxyStream(w http.ResponseWriter, r *http.Request, dir, name, final string) bool {
	src := videoOriginURL(name)
	if src == "" {
		return false
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, src, nil)
	if err != nil {
		return false
	}
	resp, err := videoHTTPClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return false
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// 回源流无已知总长：不设 Content-Length（chunked），浏览器全量缓冲播放；
	// 首个访客之后即由本地 ServeContent 提供带 Range 的正规服务。
	w.WriteHeader(http.StatusOK)

	cachePath := videoCacheDir(dir)
	tmp := filepath.Join(cachePath, fmt.Sprintf("%s.%d.tmp", filepath.Base(final), time.Now().UnixNano()))
	f, err := os.Create(tmp)
	if err != nil {
		// 落盘失败：纯转发透传（与热链等速，不更糟）
		_, _ = io.Copy(w, resp.Body)
		return true
	}
	defer func() {
		f.Close()
		_ = os.Remove(tmp) // 成功路径已在 rename 后不存在，此处兜底清残留
	}()

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 64<<10)
	clientAlive := true
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return true // 磁盘故障：停止缓存，客户端流也无法保证，直接结束
			}
			if clientAlive {
				if _, werr := w.Write(buf[:n]); werr != nil {
					clientAlive = false // 客户端断开：继续落盘，不再转发
				} else if flusher != nil {
					flusher.Flush()
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return true // 源中断：半截 temp 会被 defer 清掉
		}
	}
	if err := f.Close(); err != nil {
		return true
	}
	if err := os.Rename(tmp, final); err == nil {
		videoCacheEvict(cachePath)
	}
	return true
}

// videoCacheEvict 容量守卫：超 videoMaxBytes 时按修改时间从最旧开始删。
// 文件量级 ~20 个，全扫成本可忽略。
func videoCacheEvict(cachePath string) {
	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return
	}
	type vf struct {
		path  string
		size  int64
		mtime time.Time
	}
	var total int64
	files := make([]vf, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		files = append(files, vf{filepath.Join(cachePath, e.Name()), info.Size(), info.ModTime()})
	}
	if total <= videoMaxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
	for _, f := range files {
		if total <= videoMaxBytes {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
