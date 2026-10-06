package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFpReportRejectsOversizedBody 免登录入口的请求体上限：fp/report 此前无
// MaxBytesReader，超大流式 JSON 会被全量读入解析（实测 20MB）再被字段校验拒绝。
func TestFpReportRejectsOversizedBody(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	s := &Server{Guard: NewIPGuard(store)}
	big := `{"fp":"` + strings.Repeat("A", 300*1024) + `"}` // 300KB > 256KB 上限
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(big))
	req.RemoteAddr = "198.51.100.77:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("超大 body 不应被正常处理")
	}
}

// TestAdminLoginRejectsOversizedBody admin/login 同样限 4KB（免登录入口）。
func TestAdminLoginRejectsOversizedBody(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD_HASH", "argon2id$fake-hash-for-test")
	s := &Server{}
	big := `{"password":"` + strings.Repeat("x", 8*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(big))
	req.RemoteAddr = "198.51.100.78:4444"
	rec := httptest.NewRecorder()
	s.adminLogin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("超大 body 应 400，实际 %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFinishPasskeyLoginRejectsOversizedBody finish-login 限 1MB（守卫外入口）。
func TestFinishPasskeyLoginRejectsOversizedBody(t *testing.T) {
	s := &Server{}
	big := `{"token":"` + strings.Repeat("x", 2*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/passkey/finish-login", strings.NewReader(big))
	rec := httptest.NewRecorder()
	s.finishPasskeyLogin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("超大 body 应 400，实际 %d", rec.Code)
	}
}

// TestBehaviorOnlyReportNoEnvFlag P4-5 行为周期补充上报（仅 {fp,behavior,clock_skew_ppm}，
// 无 flags/coherent/指纹分量）不得触发"旧客户端"环境核验补记——此前每次页面隐藏
// 与每 5 分钟的行为上报都被误记 ua-platform-mismatch +30。
func TestBehaviorOnlyReportNoEnvFlag(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	s := &Server{Guard: NewIPGuard(store)}
	body := `{"fp":"behavior-ping-fp-00001","behavior":{"mouse":{"events":25}},"clock_skew_ppm":1.5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body))
	req.RemoteAddr = "198.51.100.90:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("行为上报应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	if kinds := store.kindsOf("198.51.100.90"); kinds["env-flag:ua-platform-mismatch"] > 0 {
		t.Fatalf("行为补充上报不应产生环境核验事件: %v", kinds)
	}
}
