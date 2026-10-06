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
