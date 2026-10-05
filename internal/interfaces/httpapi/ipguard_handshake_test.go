package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// guardHit 通过防护中间件打一次请求，返回响应码。
func guardHit(g *IPGuard, path, ip string) int {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:5555" // 直连方=本机代理，客户端 IP 由 XFF 给出
	req.Header.Set("X-Forwarded-For", ip)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) Chrome/126.0 Mobile")
	rec := httptest.NewRecorder()
	g.Middleware(next).ServeHTTP(rec, req)
	return rec.Code
}

// TestHandshakeRateLimited 握手通道超限返回 429 并计分（弱证据，单条不封）。
func TestHandshakeRateLimited(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "5")
	g := NewIPGuard(store)

	const ip = "203.0.113.7"
	for i := 1; i <= 5; i++ {
		if code := guardHit(g, handshakePath, ip); code != http.StatusOK {
			t.Fatalf("第 %d 次握手应在阈值内放行，实际 %d", i, code)
		}
	}
	if code := guardHit(g, handshakePath, ip); code != http.StatusTooManyRequests {
		t.Fatalf("第 6 次应 429，实际 %d", code)
	}
	kinds := store.kindsOf(ip)
	if kinds["handshake-rate"] == 0 {
		t.Fatalf("超限应记 handshake-rate 事件，实际 %v", kinds)
	}
	if ban := store.banOf(ip); ban != nil {
		t.Fatalf("单条限流事件不应封禁：%s", ban.Reason)
	}
}

// TestHandshakeLimitIsIndependentFromSiteRate 握手计数与全站速率窗口互相独立：
// 打满普通接口不消耗握手额度，反之亦然。
func TestHandshakeLimitIsIndependentFromSiteRate(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "3")
	g := NewIPGuard(store)

	const ip = "203.0.113.8"
	for i := 0; i < 10; i++ {
		if code := guardHit(g, "/api/v1/hot", ip); code != http.StatusOK {
			t.Fatalf("普通接口不应被握手限流拦截，第 %d 次实际 %d", i+1, code)
		}
	}
	for i := 1; i <= 3; i++ {
		if code := guardHit(g, handshakePath, ip); code != http.StatusOK {
			t.Fatalf("普通流量不应消耗握手额度，第 %d 次实际 %d", i, code)
		}
	}
	if code := guardHit(g, handshakePath, ip); code != http.StatusTooManyRequests {
		t.Fatalf("第 4 次握手应 429，实际 %d", code)
	}
	// 反向：握手打满后再打普通接口仍应放行
	if code := guardHit(g, "/api/v1/hot", ip); code != http.StatusOK {
		t.Fatalf("握手超限不应连带拦截其它接口，实际 %d", code)
	}
}

// TestHandshakeLimitEnvDefault 阈值可配置，非法/缺省回落 30。
func TestHandshakeLimitEnvDefault(t *testing.T) {
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "")
	if got := handshakeRatePerMin(); got != handshakeRateDefault {
		t.Fatalf("缺省应回落 %d，实际 %d", handshakeRateDefault, got)
	}
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "abc")
	if got := handshakeRatePerMin(); got != handshakeRateDefault {
		t.Fatalf("非法值应回落 %d，实际 %d", handshakeRateDefault, got)
	}
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "60")
	if got := handshakeRatePerMin(); got != 60 {
		t.Fatalf("应读取配置值 60，实际 %d", got)
	}
}

// TestHandshakeLimitSkipsLocal 回环/内网白名单不受握手限流影响（本地开发防自锁）。
func TestHandshakeLimitSkipsLocal(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("IP_GUARD_LOCAL", "1")
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "2")
	g := NewIPGuard(store)
	// 直接用 RemoteAddr 指向回环地址（不经 XFF）
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := g.Middleware(next)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, handshakePath, nil)
		req.RemoteAddr = "127.0.0.1:44321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("本机请求不应被握手限流，第 %d 次实际 %d", i+1, rec.Code)
		}
	}
}

// TestHandshakeLimitDoesNotBlockRealFlow 常规客户端节奏（冷启动 1-2 次）不受影响。
func TestHandshakeLimitDoesNotBlockRealFlow(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	t.Setenv("HANDSHAKE_RATE_PER_MIN", "30")
	g := NewIPGuard(store)

	const ip = "198.51.100.9"
	for i := 0; i < 3; i++ {
		if code := guardHit(g, handshakePath, ip); code != http.StatusOK {
			t.Fatalf("正常客户端握手次数应放行，实际 %d", code)
		}
	}
	if len(store.events) != 0 {
		t.Fatalf("正常节奏不应产生违规事件，实际 %v", store.kindsOf(ip))
	}
}
