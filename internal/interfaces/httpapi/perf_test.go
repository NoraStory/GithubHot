package httpapi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/NoraStory/GithubHot/internal/infrastructure/cache"
	"github.com/NoraStory/GithubHot/internal/infrastructure/memcache"
)

func resetRespCache() { respCache = cache.New(memcache.New(0), "", "", 0, "ghtest", nil) }

func TestEntryEnvelopeRoundTrip(t *testing.T) {
	e := cache.Entry{ContentType: "application/xml; charset=utf-8", Body: []byte("<rss/>")}
	raw := e.Encode()
	got, ok := cache.DecodeEntry(raw)
	if !ok || got.ContentType != e.ContentType || string(got.Body) != "<rss/>" {
		t.Fatalf("envelope 往返不符: %+v ok=%v", got, ok)
	}
	if _, ok := cache.DecodeEntry([]byte("no-newline")); ok {
		t.Fatal("无分隔符的值应解析失败")
	}
}

func TestGzipMiddlewareCompresses(t *testing.T) {
	h := gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, strings.Repeat("a", 1000))
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("应设置 Content-Encoding: gzip，实际头: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("响应体应为合法 gzip 流: %v", err)
	}
	got, _ := io.ReadAll(zr)
	if len(got) != 1000 || bytes.Count(got, []byte("a")) != 1000 {
		t.Fatalf("解压后内容不符，长度 %d", len(got))
	}
}

func TestGzipMiddlewarePassthrough(t *testing.T) {
	// 不可压缩类型 / 无 Accept-Encoding / 非 GET → 原样透传
	for _, tc := range []struct {
		name    string
		method  string
		ae      string
		ct      string
		wantEnc string
	}{
		{"无 Accept-Encoding", http.MethodGet, "", "text/plain", ""},
		{"视频类型", http.MethodGet, "gzip", "video/mp4", ""},
		{"HEAD 请求", http.MethodHead, "gzip", "text/plain", ""},
		{"带 Range", http.MethodGet, "gzip", "text/plain", ""},
	} {
		h := gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", tc.ct)
			_, _ = io.WriteString(w, strings.Repeat("a", 100))
		}))
		req := httptest.NewRequest(tc.method, "/x", nil)
		if tc.ae != "" {
			req.Header.Set("Accept-Encoding", tc.ae)
		}
		if tc.name == "带 Range" {
			req.Header.Set("Range", "bytes=0-50")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != tc.wantEnc {
			t.Fatalf("%s: Content-Encoding 应为 %q，实际 %q", tc.name, tc.wantEnc, got)
		}
	}
}

func TestResponseCacheHitAndMiss(t *testing.T) {
	resetRespCache()
	calls := 0
	h := responseCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeJSON(w, 200, map[string]string{"n": strconv.Itoa(calls)})
	}))
	get := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hot", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	r1 := get()
	if calls != 1 {
		t.Fatalf("首次应穿透到 handler")
	}
	r2 := get()
	if calls != 1 {
		t.Fatalf("同 URI 第二次应命中缓存，实际 handler 调用 %d 次", calls)
	}
	if r2.Header().Get("X-Cache") != "HIT" {
		t.Fatal("命中响应应带 X-Cache: HIT")
	}
	if r1.Body.String() != r2.Body.String() {
		t.Fatal("命中回放内容应与首响一致")
	}
	// 不同 query = 不同 key，不命中
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hot?kind=news", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if calls != 2 {
		t.Fatalf("不同 query 应穿透，实际 %d 次", calls)
	}
}

func TestResponseCacheNonCacheablePaths(t *testing.T) {
	resetRespCache()
	calls := 0
	h := responseCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		writeJSON(w, 200, map[string]string{"ok": "1"})
	}))
	for _, p := range []string{"/api/v1/admin/anything", "/api/v1/gh/avatar/foo", "/api/v1/hot"} {
		// gh/avatar 高基数不缓存；连续两次同路径对比调用数
		before := calls
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
		}
		if p == "/api/v1/hot" {
			if calls != before+1 {
				t.Fatalf("%s 应被缓存（第二次命中）", p)
			}
		} else if calls != before+2 {
			t.Fatalf("%s 不应被缓存（两次都应穿透）", p)
		}
	}
}

func TestVideoKeyRegex(t *testing.T) {
	for _, ok := range []string{"x/1.mp4", "y/12.mp4", "x/x7.mp4", "y/y1.mp4"} {
		if !videoKeyRe.MatchString(ok) {
			t.Fatalf("%s 应在回源白名单", ok)
		}
	}
	if videoKeyRe.MatchString("../etc/passwd") || videoKeyRe.MatchString("a/b.mp4") || videoKeyRe.MatchString("x/BAD.mp4") {
		t.Fatal("越界/非法路径不应通过白名单")
	}
}
