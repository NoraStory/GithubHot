package safehttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestSafeDialRejectsPrivateResolution 拨号时刻校验：解析结果混入私网地址
// 即拒绝连接（即使预校验阶段解析为公网，连接时刻仍会拦截切换后的内网记录）。
func TestSafeDialRejectsPrivateResolution(t *testing.T) {
	orig := lookupHost
	t.Cleanup(func() { lookupHost = orig })
	lookupHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}}, nil
	}
	_, err := safeDialContext(context.Background(), "tcp", "rebind.example.com:443")
	if !errors.Is(err, ErrDNSRejected) {
		t.Fatalf("拨号应拒绝解析到私网的 host，实际: %v", err)
	}
	// 公网解析结果 → 不以 SSRF 语义拒绝（连接失败属网络层错误）
	lookupHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	_, err = safeDialContext(context.Background(), "tcp", "public.example.com:443")
	if errors.Is(err, ErrDNSRejected) {
		t.Fatalf("公网解析结果不应被 SSRF 拒绝: %v", err)
	}
}

// TestDoRebindingProtection 端到端 TOCTOU：预校验时解析为公网（放行），
// 连接时刻解析切换为私网（拦截），内网目标零请求。
func TestDoRebindingProtection(t *testing.T) {
	var innerHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&innerHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	orig := lookupHost
	t.Cleanup(func() { lookupHost = orig })
	var calls int32
	lookupHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "attacker.example.com" {
			return orig(ctx, host)
		}
		switch atomic.AddInt32(&calls, 1) {
		case 1: // 预校验（validate → resolvePublic）：公网，放行
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		default: // 拨号时刻：rebinding 到内网地址
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
	}

	_, _, err := Do(context.Background(), http.MethodGet, "http://attacker.example.com/", nil, nil)
	if err == nil {
		t.Fatal("rebinding 请求必须失败")
	}
	if !errors.Is(err, ErrDNSRejected) && !strings.Contains(err.Error(), "私有") {
		t.Fatalf("应以 DNS 拒绝失败，实际: %v", err)
	}
	if atomic.LoadInt32(&innerHits) != 0 {
		t.Fatalf("内网目标收到 %d 次请求（rebinding 防线未生效）", innerHits)
	}
}
