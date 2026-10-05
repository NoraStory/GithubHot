package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeJA4Mapper 内存版 JA4 映射（P3-3 tlsCheck 单测用）。
type fakeJA4Mapper struct {
	loaded bool
	m      map[string]string
}

func (f *fakeJA4Mapper) Loaded() bool { return f.loaded }

func (f *fakeJA4Mapper) Lookup(ja4 string) (string, bool) {
	if !f.loaded {
		return "", false
	}
	app, ok := f.m[ja4]
	return app, ok
}

const (
	curlJA4   = "t13d1713h2_5b57614c22b0_3d5424432f57" // curl 典型形状
	chromeJA4 = "t13d1516h2_8daaf6152771_b186095e22b6" // Chrome 典型形状
	unknownJA4 = "t13d2200h0_000000000000_000000000000"
)

// reportWithJA4 上报一条带连接 JA4 的指纹，返回该 IP 的事件 kind→score。
func reportWithJA4(t *testing.T, store *fakeGuardStore, mapper JA4Mapper, fp, ip, ua, ja4 string) (map[string]int, *FingerprintDTO) {
	t.Helper()
	g := NewIPGuard(store)
	g.SetJA4DB(mapper)
	s := &Server{Guard: g}
	body := `{"fp":"` + fp + `","flags":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body))
	req.RemoteAddr = ip + ":4444"
	req.Header.Set("User-Agent", ua)
	if ja4 != "" {
		req = req.WithContext(WithJA4(req.Context(), ja4))
	}
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("上报应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	kinds := map[string]int{}
	for _, e := range store.events {
		if e.IP == ip {
			kinds[e.Kind] = e.Score
		}
	}
	row, _ := store.FindFingerprint(context.Background(), fp)
	return kinds, row
}

// TestUATLSMismatch curl 的 TLS 栈配 Chrome UA → 高置信伪造证据。
// 灰度期 0 分只记录；关闭灰度 25 分。
func TestUATLSMismatch(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	mapper := &fakeJA4Mapper{loaded: true, m: map[string]string{
		curlJA4: "curl 8.4.0", chromeJA4: "Chrome 120.0",
	}}
	kinds, _ := reportWithJA4(t, store, mapper, "fp-ual-tls-000001", "198.51.100.60",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0", curlJA4)
	if got, ok := kinds["env-flag:fpb_ua_tls_mismatch"]; !ok || got != 0 {
		t.Fatalf("灰度期应 0 分记录，实际 %v", kinds)
	}

	// 关灰度 → 25 分
	store2 := newFakeStore()
	t.Setenv("FP_SCORE_SHADOW", "0")
	kinds2, _ := reportWithJA4(t, store2, mapper, "fp-ual-tls-000002", "198.51.100.61",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0", curlJA4)
	if got, ok := kinds2["env-flag:fpb_ua_tls_mismatch"]; !ok || got != 25 {
		t.Fatalf("出灰度应 25 分，实际 %v", kinds2)
	}
}

// TestUATLSBrowserOK 真浏览器栈 + 浏览器 UA → 零误报（不产生任何 tls 相关事件）。
func TestUATLSBrowserOK(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	mapper := &fakeJA4Mapper{loaded: true, m: map[string]string{chromeJA4: "Chrome 120.0"}}
	kinds, _ := reportWithJA4(t, store, mapper, "fp-ual-ok-0000003", "198.51.100.62",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0", chromeJA4)
	for k := range kinds {
		if strings.Contains(k, "tls") {
			t.Fatalf("浏览器栈不应产生 TLS 事件：%v", kinds)
		}
	}
}

// TestTLSUnknown 指纹不在已知库 → tls_unknown 仅记录 0 分（新版本/未知工具）。
func TestTLSUnknown(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	mapper := &fakeJA4Mapper{loaded: true, m: map[string]string{chromeJA4: "Chrome 120.0"}}
	kinds, _ := reportWithJA4(t, store, mapper, "fp-ual-unk-000004", "198.51.100.63",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0", unknownJA4)
	if got, ok := kinds["env-flag:tls_unknown"]; !ok || got != 0 {
		t.Fatalf("未知指纹应记 tls_unknown 0 分，实际 %v", kinds)
	}
	if _, ok := kinds["env-flag:fpb_ua_tls_mismatch"]; ok {
		t.Fatalf("未知指纹不应计 mismatch")
	}
}

// TestTLSCheckSkips 纯 HTTP（无 JA4）或映射库未加载 → 整体跳过，ja4 照常入库为空。
func TestTLSCheckSkips(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	notLoaded := &fakeJA4Mapper{loaded: false}
	kinds, row := reportWithJA4(t, store, notLoaded, "fp-ual-skip-000005", "198.51.100.64",
		"Mozilla/5.0 Chrome/120.0", curlJA4)
	for k := range kinds {
		if strings.Contains(k, "tls") {
			t.Fatalf("映射未加载不应产生 TLS 事件：%v", kinds)
		}
	}
	if row == nil || row.JA4 != curlJA4 {
		t.Fatalf("JA4 应随指纹入库（映射加载与否无关）")
	}
	// 无 JA4（纯 HTTP）
	kinds2, row2 := reportWithJA4(t, store, mapper(), "fp-ual-skip-000006", "198.51.100.65",
		"Mozilla/5.0 Chrome/120.0", "")
	if len(kinds2) != 0 {
		t.Fatalf("纯 HTTP 上报不应有任何事件：%v", kinds2)
	}
	if row2 == nil || row2.JA4 != "" {
		t.Fatalf("无 JA4 时入库应为空")
	}
}

func mapper() JA4Mapper {
	return &fakeJA4Mapper{loaded: true, m: map[string]string{curlJA4: "curl 8.4.0"}}
}
