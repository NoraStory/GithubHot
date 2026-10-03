package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestAppGuardSignVectors 与 android-app Kotlin 客户端（SignEngine/SessionManager
// Java 兜底路径）对拍的固定测试向量，由 Python 复刻客户端算法生成。
// 算法变更任一侧都必须同步更新向量。
func TestAppGuardSignVectors(t *testing.T) {
	t.Setenv("APP_SESSION_SEED", "gh-dev-seed-v1")
	a := NewAppGuard(nil, nil)

	fp := "abcdef0123456789abcdef0123456789"
	const ts = "1760000000000"
	const nonce = "01234567abcdef89"

	if got, want := a.expectedToken(fp), "e1bc6c96572d41b0bbd15a869c0fe20a8b51ee8a"; got != want {
		t.Fatalf("token = %s, want %s", got, want)
	}
	if got := a.expectedSign(fp, ts, nonce, "GET", "/api/v1/hot/github", nil); got != "9e0feb6dcbcc3b3fd2c8fc2ae4193eadab71d5fa50da297b6808cd5cf1156980" {
		t.Fatalf("sign1 = %s", got)
	}
	if got := a.expectedSign(fp, ts, nonce, "GET", "/api/v1/digests?pageSize=20", nil); got != "964fc438d95a9a2b48162146aa152cc66298ed06579e290aa3f0c056e0c636bc" {
		t.Fatalf("sign2 = %s", got)
	}
}

// TestParseBrowserFp X-Browser-Fp 头解析：base64url JSON、白名单、截断、非法输入。
func TestParseBrowserFp(t *testing.T) {
	// 合法负载：2 个白名单键 + 1 个非白名单键 + 1 个超长值
	raw := base64.RawURLEncoding.EncodeToString([]byte(
		`{"canvas":"abc123","renderer":"Mali-G720","evil":"x","screen":"` + strings.Repeat("9", 200) + `"}`))
	m := parseBrowserFp(raw)
	if m["canvas"] != "abc123" || m["renderer"] != "Mali-G720" {
		t.Fatalf("合法键未通过: %v", m)
	}
	if _, ok := m["evil"]; ok {
		t.Fatalf("非白名单键应被丢弃: %v", m)
	}
	if _, ok := m["screen"]; ok {
		t.Fatalf("超长值应被丢弃: %v", m)
	}
	if len(parseBrowserFp("!!!bad")) != 0 {
		t.Fatalf("非法 base64 应返回空")
	}
	if len(parseBrowserFp("")) != 0 {
		t.Fatalf("空输入应返回空")
	}
}
