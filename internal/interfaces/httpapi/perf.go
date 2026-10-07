package httpapi

import (
	"bytes"
	"compress/gzip"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/cache"
	"github.com/NoraStory/GithubHot/internal/infrastructure/memcache"
)

// ---------- 响应缓存（"Redis 类"缓存层的服务端入口）----------

// respCache 两级响应缓存：L1 进程内 + L2 Redis（InitCaches 按 REDIS_ADDR 装配）。
// 公共只读 API 命中直接回放；管理端 / 鉴权路由不在缓存面，天然不受影响。
// 包级默认纯 L1（测试与非 serve 模式），Serve 启动时经 InitCaches 重建。
var respCache = newDefaultRespCache()

func newDefaultRespCache() *cache.TwoTier {
	return cache.New(memcache.New(2048), "", "", 0, "gh", func(msg string) { log.Printf("[cache] %s", msg) })
}

// InitCaches 按 Redis 配置重建两级缓存。必须在 config.Load 之后调用（.env 已
// 落到进程环境），Serve 启动早期调用一次；测试可直接替换包级变量。
func InitCaches(redisAddr, redisPassword string, redisDB int) {
	enableLog := func(msg string) { log.Printf("[cache] %s", msg) }
	respCache = cache.New(memcache.New(2048), redisAddr, redisPassword, redisDB, "gh", enableLog)
	ghAvatarCache = cache.New(memcache.New(1024), redisAddr, redisPassword, redisDB, "ghavatar", nil)
}

type cachedWriter struct {
	http.ResponseWriter
	buf    bytes.Buffer
	status int
}

func (cw *cachedWriter) WriteHeader(code int) {
	cw.status = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *cachedWriter) Write(b []byte) (int, error) {
	if cw.status == 0 {
		cw.status = http.StatusOK
	}
	cw.buf.Write(b)
	return cw.ResponseWriter.Write(b)
}

// cacheTTLFor 公共只读路径的缓存时长（0 = 不缓存）。热榜/期刊等数据只在管道
// 运行后变化，分钟级 TTL 足够新鲜，换来重复访问零 SQLite 压力与零重建开销
// （BuildHotView 每请求全量查库，是首页并发下的主要 CPU/锁竞争点）。
func cacheTTLFor(path string) time.Duration {
	switch {
	case strings.HasPrefix(path, "/api/v1/hot"):
		return 60 * time.Second
	case strings.HasPrefix(path, "/api/v1/digest"), strings.HasPrefix(path, "/api/v1/stories"), strings.HasPrefix(path, "/api/v1/story/"):
		return 120 * time.Second
	case strings.HasPrefix(path, "/api/v1/search"):
		return 60 * time.Second
	case strings.HasPrefix(path, "/api/v1/site/config"), strings.HasPrefix(path, "/api/v1/music/"), strings.HasPrefix(path, "/api/v1/sources"):
		return 300 * time.Second
	case strings.HasPrefix(path, "/feed/"):
		return 120 * time.Second
	}
	return 0
}

// responseCacheMiddleware 只缓 GET 命中 cacheTTLFor 的路径；非 200 不缓存。
// 回放还原捕获时的 Content-Type（/api JSON 与 /feed XML 各归其位）。
// 守护/风控中间件在更外层已跑完，此处回放不影响防护计分。
func responseCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ttl := cacheTTLFor(r.URL.Path)
		if ttl == 0 || r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		key := "resp:" + r.URL.RequestURI()
		if e, ok := respCache.Get(key); ok {
			w.Header().Set("Content-Type", e.ContentType)
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(e.Body)
			return
		}
		cw := &cachedWriter{ResponseWriter: w}
		next.ServeHTTP(cw, r)
		if cw.status == http.StatusOK && cw.buf.Len() > 0 && cw.buf.Len() < 2<<20 {
			respCache.Set(key, cache.Entry{
				ContentType: cw.Header().Get("Content-Type"),
				Body:        append([]byte(nil), cw.buf.Bytes()...),
			}, ttl)
		}
	})
}

// ---------- gzip 压缩（静态文本与 API JSON 通吃）----------

// gzCompressible 按 Content-Type 判定可压缩；视频/图片/字体本身已压缩，跳过省 CPU。
func gzCompressible(ct string) bool {
	if ct == "" {
		return false
	}
	for _, p := range []string{"text/", "application/json", "application/javascript", "application/xml", "image/svg+xml"} {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	return false
}

// gzWriterPool gzip writer 池：复用压缩器内部缓冲。
var gzWriterPool = sync.Pool{New: func() any { return gzip.NewWriter(new(bytes.Buffer)) }}

// gzipMiddleware 对 GET 的可压缩响应做透明 gzip。仅流式包装：决定是否压缩的
// 依据是 handler 首次写头时的 Content-Type，压缩字节随写随发。
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 只压 GET；HEAD 无体、304/206 分段与已压缩格式交给透传路径
		if r.Method != http.MethodGet ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		g := &gzipResponseWriter{ResponseWriter: w}
		defer func() {
			if g.gz != nil {
				_ = g.gz.Close() // 冲刷 gzip 尾部
				gzWriterPool.Put(g.gz)
				g.gz = nil
			}
		}()
		next.ServeHTTP(g, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	plain       bool // 判定为不可压缩，透传
	wroteHeader bool
}

func (g *gzipResponseWriter) commit() {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	h := g.Header()
	ct := h.Get("Content-Type")
	if g.plainAcceptable(ct) {
		g.plain = true
	} else {
		h.Del("Content-Length") // 压缩后长度变化，避免下游误用
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		g.gz = gzWriterPool.Get().(*gzip.Writer)
		g.gz.Reset(g.ResponseWriter)
	}
}

// plainAcceptable 命中任一条件则放弃压缩。
func (g *gzipResponseWriter) plainAcceptable(ct string) bool {
	return g.Header().Get("Content-Encoding") != "" || // 下游已编码（如视频流）
		!gzCompressible(ct)
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	// 非 200（错误体小、304 无体）不压
	if code != http.StatusOK {
		g.plain = true
	}
	g.commit()
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		g.commit()
	}
	if g.plain {
		return g.ResponseWriter.Write(b)
	}
	return g.gz.Write(b)
}
