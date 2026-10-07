package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func ipCheck(t *testing.T, s *Server, ip string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ip/check", nil)
	req.RemoteAddr = ip + ":1000"
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/ip/check 应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应应为 JSON: %v", err)
	}
	return out
}

func TestIPCheckReportsBan(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	s := &Server{Guard: NewIPGuard(store)}
	ip := "198.51.100.70"
	if err := store.UpsertBan(context.Background(), ip, 2, 1, "多证据互证", time.Hour); err != nil {
		t.Fatal(err)
	}
	out := ipCheck(t, s, ip)
	if out["banned"] != true {
		t.Fatalf("应返回 banned=true，实际 %#v", out)
	}
	if out["reason"] != "多证据互证" {
		t.Fatalf("应返回封禁原因，实际 %#v", out)
	}
	if _, ok := out["expiresAt"].(string); !ok {
		t.Fatalf("应返回到期时间，实际 %#v", out)
	}
}

func TestIPCheckReportsUnbanned(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	s := &Server{Guard: NewIPGuard(newFakeStore())}
	out := ipCheck(t, s, "198.51.100.71")
	if out["banned"] != false {
		t.Fatalf("未封禁应返回 banned=false，实际 %#v", out)
	}
}

func TestIPCheckIgnoresExpiredBan(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	s := &Server{Guard: NewIPGuard(store)}
	ip := "198.51.100.72"
	if err := store.UpsertBan(context.Background(), ip, 1, 1, "已到期", -time.Hour); err != nil {
		t.Fatal(err)
	}
	out := ipCheck(t, s, ip)
	if out["banned"] != false {
		t.Fatalf("过期封禁应返回 banned=false，实际 %#v", out)
	}
}
