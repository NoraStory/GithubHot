package httpapi

import (
	"net/http/httptest"
	"testing"
)

// 默认（未配置 TRUSTED_PROXY）：完全忽略代理头，防自填 XFF 栽赃/绕过。
func TestClientIPDefaultsToRemoteAddr(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "203.0.113.9:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "1.2.3.5")
	if got := clientIPFromRequest(r); got != "203.0.113.9" {
		t.Fatalf("未配置 TRUSTED_PROXY 时应使用 RemoteAddr，实际 %s", got)
	}
}

// 直连方不在信任名单：即便对方自报 XFF，也不采信。
func TestUntrustedPeerCannotSpoofXFF(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "198.51.100.7:5555" // 公网直连，不在信任名单
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := clientIPFromRequest(r); got != "198.51.100.7" {
		t.Fatalf("非受信直连应使用 RemoteAddr，实际 %s", got)
	}
}

// 受信代理追加真实 IP 后，取最右侧非受信地址——攻击者自填的前缀被忽略。
func TestTrustedProxyIgnoresSpoofedPrefix(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	// 攻击者自填 1.2.3.4，nginx 用 $proxy_add_x_forwarded_for 追加真实客户端
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.20")
	if got := clientIPFromRequest(r); got != "203.0.113.20" {
		t.Fatalf("应从右往左取第一个非受信地址，实际 %s", got)
	}
}

// 多级受信代理链：跳过受信内网地址，取最近的真实客户端。
func TestTrustedProxyMultiHop(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32,10.0.0.0/8")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.5")
	if got := clientIPFromRequest(r); got != "9.9.9.9" {
		t.Fatalf("多级代理应取 9.9.9.9，实际 %s", got)
	}
}

// X-Real-IP 兜底（无 XFF 时）。
func TestTrustedProxyRealIPFallback(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Real-IP", "203.0.113.30")
	if got := clientIPFromRequest(r); got != "203.0.113.30" {
		t.Fatalf("应采信 X-Real-IP，实际 %s", got)
	}
}

// 裸 IP 配置（无 /掩码）按单机处理。
func TestTrustedProxyBareIPConfig(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "127.0.0.1")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.40")
	if got := clientIPFromRequest(r); got != "203.0.113.40" {
		t.Fatalf("裸 IP 配置应生效，实际 %s", got)
	}
}

// 全部为受信地址（畸形 XFF）时回落到 RemoteAddr。
func TestTrustedProxyAllTrustedFallsBack(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "127.0.0.1, ")
	if got := clientIPFromRequest(r); got != "127.0.0.1" {
		t.Fatalf("受信链应回落 RemoteAddr，实际 %s", got)
	}
}

// IPv6 受信代理。
func TestTrustedProxyIPv6(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "::1/128")
	r := httptest.NewRequest("GET", "/api/v1/hot", nil)
	r.RemoteAddr = "[::1]:5555"
	r.Header.Set("X-Forwarded-For", "2001:db8::5")
	if got := clientIPFromRequest(r); got != "2001:db8::5" {
		t.Fatalf("IPv6 代理应生效，实际 %s", got)
	}
}
