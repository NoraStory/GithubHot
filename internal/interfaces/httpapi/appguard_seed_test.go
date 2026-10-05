package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// signFor 用指定种子复算请求签名（与 android-app SignEngine 同算法）。
func signFor(seed, fp, ts, nonce, method, path string, body []byte) string {
	return expectedSignWith(seed, fp, ts, nonce, method, path, body)
}

// ---------- 种子来源：主种子 + 过渡期旧种子 ----------

// TestAppSignSeedsOrderAndDedup 种子列表：主种子在前、去重、忽略空白；
// 未配置主种子时回退出厂默认（run/mcp 与测试模式行为不变）。
func TestAppSignSeedsOrderAndDedup(t *testing.T) {
	t.Setenv("APP_SESSION_SEED", "")
	t.Setenv("APP_SIGN_SEED", "primary-1")
	t.Setenv("APP_SIGN_SEED_GRACE", "primary-1, old-1 ,, old-1 ")
	got := appSignSeeds()
	if len(got) != 2 || got[0] != "primary-1" || got[1] != "old-1" {
		t.Fatalf("seeds = %#v", got)
	}

	t.Setenv("APP_SIGN_SEED", "")
	t.Setenv("APP_SIGN_SEED_GRACE", "")
	got = appSignSeeds()
	if len(got) != 1 || got[0] != "gh-dev-seed-v1" {
		t.Fatalf("未配置主种子应回退出厂默认，实际 %#v", got)
	}
}

// TestAppGuardSignSeeds 验签按种子逐个尝试：主种子命中 0、过渡期旧种子命中 1、
// 未知种子 -1；会话令牌同样受理过渡期种子。
func TestAppGuardSignSeeds(t *testing.T) {
	t.Setenv("APP_SESSION_SEED", "")
	t.Setenv("APP_SIGN_SEED", "primary-seed-aaaa")
	t.Setenv("APP_SIGN_SEED_GRACE", "legacy-seed-bbbb")
	a := NewAppGuard(nil, nil)

	const fp = "abcdef0123456789abcdef0123456789"
	const ts = "1760000000000"
	const nonce = "01234567abcdef89"

	if got := a.signMatches(fp, ts, nonce, "GET", "/api/v1/hot/github", nil,
		signFor("primary-seed-aaaa", fp, ts, nonce, "GET", "/api/v1/hot/github", nil)); got != 0 {
		t.Fatalf("主种子应命中下标 0，实际 %d", got)
	}
	if got := a.signMatches(fp, ts, nonce, "GET", "/api/v1/hot/github", nil,
		signFor("legacy-seed-bbbb", fp, ts, nonce, "GET", "/api/v1/hot/github", nil)); got != 1 {
		t.Fatalf("过渡期旧种子应命中下标 1，实际 %d", got)
	}
	if got := a.signMatches(fp, ts, nonce, "GET", "/api/v1/hot/github", nil,
		signFor("attacker-seed", fp, ts, nonce, "GET", "/api/v1/hot/github", nil)); got != -1 {
		t.Fatalf("未知种子应无法命中，实际 %d", got)
	}
	// 大小写不敏感（客户端历史上可能发大写十六进制）
	up := strings.ToUpper(signFor("primary-seed-aaaa", fp, ts, nonce, "GET", "/api/v1/hot/github", nil))
	if got := a.signMatches(fp, ts, nonce, "GET", "/api/v1/hot/github", nil, up); got != 0 {
		t.Fatalf("签名比较应大小写不敏感（客户端现状），实际 %d", got)
	}

	if !a.tokenMatches(fp, expectedTokenWith("legacy-seed-bbbb", fp)) {
		t.Fatalf("过渡期旧种子的会话令牌应被受理")
	}
	if a.tokenMatches(fp, expectedTokenWith("attacker-seed", fp)) {
		t.Fatalf("未知种子的会话令牌不得受理")
	}
	if !a.grace {
		t.Fatalf("配置了过渡期种子应进入 grace 模式")
	}
}

// TestAppGuardNoGraceFlagWhenSingleSeed 未配置过渡期种子时不进 grace（验签失败照常计分）。
func TestAppGuardNoGraceFlagWhenSingleSeed(t *testing.T) {
	t.Setenv("APP_SESSION_SEED", "")
	t.Setenv("APP_SIGN_SEED", "only-seed")
	t.Setenv("APP_SIGN_SEED_GRACE", "")
	if a := NewAppGuard(nil, nil); a.grace {
		t.Fatalf("单一主种子不应进入 grace 模式")
	}
}

// ---------- 中间件：过渡期不计分 vs 常态计分 ----------

// appGuardRequest 构造一条带签名的 APP 请求。
func appGuardRequest(t *testing.T, seed, fp string) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := strconv.FormatInt(time.Now().UnixNano(), 16)
	path := "/api/v1/hot/github"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set(appFpHeader, fp)
	req.Header.Set(appTsHeader, ts)
	req.Header.Set(appNonceHeader, nonce)
	req.Header.Set(appSignHeader, signFor(seed, fp, ts, nonce, http.MethodGet, path, nil))
	return req
}

// TestAppGuardGraceDoesNotScoreUnknownSeed 过渡期内：存量 APP（缓存旧种子/新购机本地
// 随机种子）验签不通过时只观察不计分——否则正常用户会被连续 401 攒积分封禁。
func TestAppGuardGraceDoesNotScoreUnknownSeed(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("APP_SIGN_SEED", "primary-seed-aaaa")
	t.Setenv("APP_SIGN_SEED_GRACE", "gh-dev-seed-v1")
	g := NewIPGuard(store)
	a := NewAppGuard(store, g)

	rec := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("验签失败不应放行")
	})).ServeHTTP(rec, appGuardRequest(t, "unknown-local-random-seed", "fp-unknown-seed"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("验签失败应 401，实际 %d", rec.Code)
	}
	kinds := store.kindsOf("127.0.0.1")
	if kinds["app-sign-unverified"] != 1 {
		t.Fatalf("过渡期应记 app-sign-unverified 观察事件，实际 %v", kinds)
	}
	if kinds["app-sign-invalid"] != 0 {
		t.Fatalf("过渡期不应记 app-sign-invalid（会攒分封禁）")
	}
	// 计分必须为 0：0 分事件在 iprisk 中不参与决策
	store.mu.Lock()
	var score int
	for _, e := range store.events {
		if e.Kind == "app-sign-unverified" {
			score = e.Score
		}
	}
	store.mu.Unlock()
	if score != 0 {
		t.Fatalf("过渡期观察事件计分应为 0，实际 %d", score)
	}
	if ban := store.banOf("127.0.0.1"); ban != nil {
		t.Fatalf("过渡期不应封禁：%s", ban.Reason)
	}
}

// TestAppGuardScoresInvalidSignWithoutGrace 未配置过渡期（常态）：验签失败按强证据计 60 分。
func TestAppGuardScoresInvalidSignWithoutGrace(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("APP_SIGN_SEED", "primary-seed-aaaa")
	t.Setenv("APP_SIGN_SEED_GRACE", "")
	g := NewIPGuard(store)
	a := NewAppGuard(store, g)

	rec := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("验签失败不应放行")
	})).ServeHTTP(rec, appGuardRequest(t, "attacker-seed", "fp-attacker"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("验签失败应 401，实际 %d", rec.Code)
	}
	kinds := store.kindsOf("127.0.0.1")
	if kinds["app-sign-invalid"] != 1 {
		t.Fatalf("常态应记 app-sign-invalid，实际 %v", kinds)
	}
	if kinds["app-sign-unverified"] != 0 {
		t.Fatalf("常态不应记观察事件")
	}
}

// TestAppGuardGraceStillAcceptsLegacySeed 过渡期内：用旧种子签名的存量 APP 正常放行
// （双种子验签），且不计任何违规。
func TestAppGuardGraceStillAcceptsLegacySeed(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("APP_SIGN_SEED", "primary-seed-aaaa")
	t.Setenv("APP_SIGN_SEED_GRACE", "gh-dev-seed-v1")
	g := NewIPGuard(store)
	a := NewAppGuard(store, g)

	passed := false
	rec := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		passed = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, appGuardRequest(t, "gh-dev-seed-v1", "fp-legacy-app"))

	if !passed || rec.Code != http.StatusOK {
		t.Fatalf("过渡期旧种子应放行，实际 code=%d passed=%v", rec.Code, passed)
	}
	if len(store.events) != 0 {
		t.Fatalf("合法旧种子不应记违规事件，实际 %v", store.kindsOf("127.0.0.1"))
	}
}

// ---------- 握手通道不得再下发种子 ----------

// TestSiteConfigDoesNotLeakSeed /api/v1/site/config 是公开免签端点：签名种子绝不出现
// 在响应里（历史版本下发 gh-dev-seed-v1，等于派生密钥公开）。
func TestSiteConfigDoesNotLeakSeed(t *testing.T) {
	t.Setenv("APP_SIGN_SEED", "super-secret-seed-value")
	rec := httptest.NewRecorder()
	(&Server{}).siteConfig(rec, httptest.NewRequest(http.MethodGet, "/api/v1/site/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("site/config 应 200，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if len(body) == 0 {
		t.Fatalf("site/config 响应为空")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("site/config 响应不是 JSON: %v", err)
	}
	for _, key := range []string{"session_seed", "seed", "app_sign_seed"} {
		if v, ok := payload[key]; ok {
			t.Fatalf("site/config 不得下发 %s（实际 %v）", key, v)
		}
	}
	if strings.Contains(rec.Body.String(), "super-secret-seed-value") ||
		strings.Contains(rec.Body.String(), "gh-dev-seed-v1") {
		t.Fatalf("site/config 响应体含种子字面量: %s", rec.Body.String())
	}
	// 风控下发字段仍在（远程封禁 / 强制更新链路不受影响）
	if _, ok := payload["banned"]; !ok {
		t.Fatalf("site/config 应保留下发 banned")
	}
}
