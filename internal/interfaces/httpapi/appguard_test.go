package httpapi

import "testing"

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
