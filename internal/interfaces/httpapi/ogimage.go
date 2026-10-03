package httpapi

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---------- 链接封面图（og:image）懒抓取 ----------
//
// 资讯/国内榜单多数信源不下发封面图，APP 卡片需要真实图片。
// 策略：响应时只读缓存；未命中的 URL 异步抓取 og:image/twitter:image 入缓存
// （负缓存防重复），下一次刷新即可见图。并发抓取用信号量限流。

// LinkImageCache 封面缓存端口（sqlite 实现，cli 层适配）。
type LinkImageCache interface {
	GetLinkImage(ctx context.Context, url string) (string, bool, error)
	SetLinkImage(ctx context.Context, url, image string) error
}

// ImageResolver 封面解析器：缓存优先 + 异步补抓。
type ImageResolver struct {
	cache LinkImageCache

	mu       sync.Mutex
	inflight map[string]bool
	sem      chan struct{} // 抓取并发上限
}

// NewImageResolver 构建解析器；cache 为 nil 时所有解析返回空（降级为无图）。
func NewImageResolver(cache LinkImageCache) *ImageResolver {
	return &ImageResolver{
		cache:    cache,
		inflight: map[string]bool{},
		sem:      make(chan struct{}, 8),
	}
}

// ogImageRe / ogAbsRe 宽松匹配 meta 标签的 og:image 与 content（双引号/单引号/无引号）。
var (
	ogImageRe = regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)\s*=\s*["']?(?:og:image|og:image:url|twitter:image)["']?[^>]+content\s*=\s*["']([^"']+)["']`)
	ogImageRe2 = regexp.MustCompile(`(?is)<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+(?:property|name)\s*=\s*["']?(?:og:image|og:image:url|twitter:image)["']?`)
)

// Resolve 返回缓存中的封面图 URL；未命中触发异步抓取并返回空串。
func (r *ImageResolver) Resolve(rawURL string) string {
	if r == nil || r.cache == nil || rawURL == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if img, ok, _ := r.cache.GetLinkImage(ctx, rawURL); ok {
		return img
	}
	r.mu.Lock()
	if r.inflight[rawURL] {
		r.mu.Unlock()
		return ""
	}
	r.inflight[rawURL] = true
	r.mu.Unlock()
	go r.fetchAndStore(rawURL)
	return ""
}

// fetchAndStore 后台抓取 og:image 写入缓存（含负缓存）。
func (r *ImageResolver) fetchAndStore(rawURL string) {
	defer func() {
		r.mu.Lock()
		delete(r.inflight, rawURL)
		r.mu.Unlock()
	}()
	r.sem <- struct{}{}
	defer func() { <-r.sem }()

	img := fetchOGImage(rawURL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.cache.SetLinkImage(ctx, rawURL, img); err != nil {
		log.Printf("[ogimage] 缓存写入失败 %s: %v", rawURL, err)
	}
}

// ogHTTPClient 外网抓取客户端：走环境代理（ foreign 站点依赖），整体超时收紧。
var ogHTTPClient = &http.Client{
	Timeout:   6 * time.Second,
	Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
}

// fetchOGImage 抓取页面并解析 og:image / twitter:image，返回绝对地址；失败返回空串。
func fetchOGImage(rawURL string) string {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := ogHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "html") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return ""
	}
	m := ogImageRe.FindSubmatch(body)
	if len(m) < 2 {
		m = ogImageRe2.FindSubmatch(body)
	}
	if len(m) < 2 {
		return ""
	}
	img := strings.TrimSpace(strings.ReplaceAll(string(m[1]), "&amp;", "&"))
	if img == "" || strings.HasPrefix(img, "data:") {
		return ""
	}
	abs, err := url.Parse(img)
	if err != nil {
		return ""
	}
	base, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if !abs.IsAbs() {
		abs = base.ResolveReference(abs)
	}
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}
