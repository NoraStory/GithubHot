package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGuardStore 内存版 GuardStore（集成测试用，无 SQLite 依赖）。
type fakeGuardStore struct {
	mu     sync.Mutex
	events []IPEventDTO
	bans   map[string]*BanDTO
	fps    []FingerprintDTO
	phashes []PHashRowDTO
	links   []FPLinkDTO
}

func newFakeStore() *fakeGuardStore { return &fakeGuardStore{bans: map[string]*BanDTO{}} }

func (f *fakeGuardStore) AddIPEvent(_ context.Context, ip, kind, detail string, score int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, IPEventDTO{
		ID: int64(len(f.events) + 1), IP: ip, Kind: kind, Detail: detail, Score: score, At: time.Now(),
	})
	return nil
}

func (f *fakeGuardStore) RecentIPEventsScore(_ context.Context, ip string, seconds int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cut := time.Now().Add(-time.Duration(seconds) * time.Second)
	total := 0
	for _, e := range f.events {
		if e.IP == ip && e.At.After(cut) {
			total += e.Score
		}
	}
	return total, nil
}

func (f *fakeGuardStore) ListIPEvents(_ context.Context, limit int) ([]IPEventDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return tail(f.events, limit), nil
}

func (f *fakeGuardStore) ListIPEventsSince(_ context.Context, limit int, since time.Time) ([]IPEventDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []IPEventDTO{}
	for _, e := range f.events {
		if e.At.After(since) {
			out = append(out, e)
		}
	}
	return tail(out, limit), nil
}

func (f *fakeGuardStore) ListIPEventsByIP(_ context.Context, ip string, limit int) ([]IPEventDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []IPEventDTO{}
	for _, e := range f.events {
		if e.IP == ip {
			out = append(out, e)
		}
	}
	return tail(out, limit), nil
}

func (f *fakeGuardStore) UpsertFingerprint(context.Context, string, string, string, FingerprintMeta) ([]string, error) {
	return []string{}, nil
}
func (f *fakeGuardStore) ListFingerprints(context.Context, int) ([]FingerprintDTO, error) {
	return nil, nil
}
func (f *fakeGuardStore) FindBan(_ context.Context, ip string) (*BanDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bans[ip], nil
}
func (f *fakeGuardStore) BannedAmong(context.Context, []string) ([]string, error) { return nil, nil }
func (f *fakeGuardStore) UpsertBan(_ context.Context, ip string, strikes, level int, reason string, dur time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bans[ip] = &BanDTO{IP: ip, Strikes: strikes, Level: level, Reason: reason,
		BannedAt: time.Now(), ExpiresAt: time.Now().Add(dur)}
	return nil
}
func (f *fakeGuardStore) ListBans(context.Context) ([]BanDTO, error) { return nil, nil }
func (f *fakeGuardStore) DeleteBan(_ context.Context, ip string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.bans, ip)
	return nil
}
func (f *fakeGuardStore) TouchIPProfile(context.Context, string, string, int) error { return nil }
func (f *fakeGuardStore) FindIPProfile(context.Context, string) (*IPProfileDTO, error) {
	return nil, nil
}
func (f *fakeGuardStore) ListFingerprintsByIP(context.Context, string, int) ([]FingerprintDTO, error) {
	return nil, nil
}

func (f *fakeGuardStore) banOf(ip string) *BanDTO {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bans[ip]
}

// seedEvent 注入一条"ageMin 分钟前"发生的事件——生产环境同类事件有 5 分钟去重，
// 测试必须按真实时序铺开，否则同一个瞬间堆叠会失真。
func (f *fakeGuardStore) seedEvent(ip, kind string, score int, ageMin float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, IPEventDTO{
		ID: int64(len(f.events) + 1), IP: ip, Kind: kind, Detail: "seed",
		Score: score, At: time.Now().Add(-time.Duration(ageMin * float64(time.Minute))),
	})
}

func (f *fakeGuardStore) kindsOf(ip string) map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := map[string]int{}
	for _, e := range f.events {
		if e.IP == ip {
			m[e.Kind]++
		}
	}
	return m
}

func tail(in []IPEventDTO, n int) []IPEventDTO {
	if n > 0 && len(in) > n {
		in = in[len(in)-n:]
	}
	out := make([]IPEventDTO, len(in))
	copy(out, in)
	return out
}

// ---------- 回归：灾难路径（重启 / 密钥轮换不得封老访客） ----------

func TestTokenKeyRotationDoesNotBan(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理

	// 模拟"重启前"的实例签发令牌
	t.Setenv("IP_GUARD_SECRET", "secret-before-rotation-0123456789")
	old := NewIPGuard(store)
	token, _ := old.issueIDToken("1.2.3.4", "fpfpfpfpfpfpfpfp")

	// 模拟"重启后"密钥已轮换
	t.Setenv("IP_GUARD_SECRET", "secret-after-rotation-9876543210")
	g := NewIPGuard(store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hot", nil)
	req.RemoteAddr = "127.0.0.1:5555" // 以本机反向代理为直连方（TRUSTED_PROXY 生效）
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0")
	req.AddCookie(&http.Cookie{Name: idCookieName, Value: token})

	g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if ban := store.banOf("1.2.3.4"); ban != nil {
		t.Fatalf("密钥轮换后老令牌被误判为伪造并封禁：%s", ban.Reason)
	}
	kinds := store.kindsOf("1.2.3.4")
	if kinds["id-token-stale"] == 0 {
		t.Fatalf("应记录 id-token-stale 观察事件，实际 %v", kinds)
	}
	if kinds["id-forgery"] != 0 {
		t.Fatalf("不应记 id-forgery（结构合法，仅签名失效）")
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), idCookieName) {
		t.Fatalf("应重新签发身份令牌")
	}
}

func TestMalformedTokenStillBans(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	t.Setenv("IP_GUARD_SECRET", "secret-0123456789abcdef")
	g := NewIPGuard(store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hot", nil)
	req.RemoteAddr = "127.0.0.1:5555" // 以本机反向代理为直连方（TRUSTED_PROXY 生效）
	req.Header.Set("X-Forwarded-For", "5.6.7.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/126.0")
	req.AddCookie(&http.Cookie{Name: idCookieName, Value: "v2.bm90LWJhc2U2NA.c2hvcnQ"})

	g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if ban := store.banOf("5.6.7.8"); ban == nil {
		t.Fatalf("结构非法的令牌应即时封禁（无歧义伪造）")
	}
	if store.kindsOf("5.6.7.8")["id-forgery"] == 0 {
		t.Fatalf("应记 id-forgery 事件")
	}
}

// ---------- 回归：单信号不得封禁（旧实现会封） ----------

func TestSingleEnvFlagDoesNotBan(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	ctx := context.Background()

	// 旧实现：headless-ua 标 severe → 立即封 7 天
	g.Event(ctx, "9.9.9.9", "env-flag", "环境核验命中 headless-ua", 50, false)

	if ban := store.banOf("9.9.9.9"); ban != nil {
		t.Fatalf("单条环境核验命中不应封禁（旧实现 severe 秒封）：%s", ban.Reason)
	}
}

func TestRepeatedAdminProbeDoesNotBan(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	ctx := context.Background()
	const ip = "9.9.9.8"

	// 管理端探测 +50 按 5 分钟去重反复出现（旧实现第二次叠加即破 100，封管理员自己）。
	// 真实时序：12 分钟前、6 分钟前各一次，再加本次 → 衰减后约 110 < 弱类单类门槛 150
	store.seedEvent(ip, "admin-probe", 50, 12)
	store.seedEvent(ip, "admin-probe", 50, 6)
	g.event(ctx, ip, "admin-probe", "admin API 40 req/min", 50, false)

	if ban := store.banOf(ip); ban != nil {
		t.Fatalf("单一类型反复上报不应封禁：%s", ban.Reason)
	}
}

// ---------- 多证据互证 → 封（真实判定能力保留） ----------

// TestCorroboratedEnvFlagsBan 三条不同环境核验命中（各自独立 kind）应互证成封禁：
// headless-ua 50 + navigator-webdriver 60 + automation-global 60 → 封顶 40×3 = 120
func TestCorroboratedEnvFlagsBan(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	ctx := context.Background()
	const ip = "3.3.3.3"

	g.Event(ctx, ip, "env-flag:headless-ua", "环境核验命中 headless-ua", 50, false)
	g.Event(ctx, ip, "env-flag:navigator-webdriver", "环境核验命中 navigator-webdriver", 60, false)
	g.Event(ctx, ip, "env-flag:automation-global", "环境核验命中 automation-global", 60, false)

	if ban := store.banOf(ip); ban == nil {
		t.Fatalf("三条独立环境核验命中应互证封禁")
	}
}

// TestDistinctEnvFlagKindsAreIndependent 不同 flag 是独立事件（不被 5 分钟去重压掉）
func TestDistinctEnvFlagKindsAreIndependent(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	ctx := context.Background()
	const ip = "3.3.3.4"

	g.Event(ctx, ip, "env-flag:headless-ua", "a", 50, false)
	g.Event(ctx, ip, "env-flag:no-plugins", "b", 10, false)
	kinds := store.kindsOf(ip)
	if kinds["env-flag:headless-ua"] != 1 || kinds["env-flag:no-plugins"] != 1 {
		t.Fatalf("不同 flag 应各自记账，实际 %v", kinds)
	}
	// 同一 flag 短时间内重复上报仍应被去重
	g.Event(ctx, ip, "env-flag:headless-ua", "a", 50, false)
	if got := store.kindsOf(ip)["env-flag:headless-ua"]; got != 1 {
		t.Fatalf("同一 flag 应被 5 分钟去重，实际记录 %d 条", got)
	}
}

func TestCorroboratedEvidenceBans(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	ctx := context.Background()

	ip := "8.8.4.4"
	g.event(ctx, ip, "rate", "452 req/min", 60, false)
	g.event(ctx, ip, "env-flag", "环境核验命中 navigator-webdriver", 60, false)
	g.event(ctx, ip, "bot-ua", "curl/8.4.0", 25, false)

	ban := store.banOf(ip)
	if ban == nil {
		t.Fatalf("三类独立证据互证应封禁（有效分应 ≥100）")
	}
	if !strings.Contains(ban.Reason, "多证据互证") && !strings.Contains(ban.Reason, "持续性") {
		t.Fatalf("封禁理由应说明判定依据，实际 %q", ban.Reason)
	}
}

// ---------- 共享出口：UA 多样度高时速率类被稀释 ----------

func TestGuardDilutesSharedOutlet(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	ctx := context.Background()
	ip := "7.7.7.7"

	// 通过中间件让该 IP 的窗口记录到 5 种 UA（模拟共享出口）
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := g.Middleware(next)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hot", nil)
		req.RemoteAddr = "127.0.0.1:5555" // 以本机反向代理为直连方（TRUSTED_PROXY 生效）
		req.Header.Set("X-Forwarded-For", ip)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) Chrome/12"+string(rune('0'+i)))
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := g.uaDiversity(ip); got < 5 {
		t.Fatalf("UA 种类应记录 ≥5，实际 %d", got)
	}

	// 共享出口上的中等偏高速率持续（每 5 分钟一条 60 分，共 8 条 → 衰减后约 192）；
	// 稀释后 ≈77 < 弱类单类门槛 150，且只有一种违规类型 → 不应封
	for i := 7; i >= 1; i-- {
		store.seedEvent(ip, "rate", 60, float64(i*5)) // 35/30/25/20/15/10/5 分钟前
	}
	g.event(ctx, ip, "rate", "310 req/min", 60, false)
	if ban := store.banOf(ip); ban != nil {
		t.Fatalf("共享出口（UA 种类 5）不应被封：%s", ban.Reason)
	}
}

// ---------- 封禁墙与自救通道 ----------

func TestBanWallAndRescuePath(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32") // 测试用 XFF 模拟公网客户端，需声明受信代理
	g := NewIPGuard(store)
	if err := store.UpsertBan(context.Background(), "6.6.6.6", 3, 3, "测试封禁", time.Hour); err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := g.Middleware(next)

	blocked := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hot", nil)
	req.RemoteAddr = "127.0.0.1:5555" // 以本机反向代理为直连方（TRUSTED_PROXY 生效）
	req.Header.Set("X-Forwarded-For", "6.6.6.6")
	h.ServeHTTP(blocked, req)
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("被封 IP 的普通请求应为 404，实际 %d", blocked.Code)
	}

	rescue := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ipguard/summary", nil)
	req2.RemoteAddr = "127.0.0.1:5555"
	req2.Header.Set("X-Forwarded-For", "6.6.6.6")
	h.ServeHTTP(rescue, req2)
	if rescue.Code == http.StatusNotFound {
		t.Fatalf("被封 IP 应能访问 ipguard 自救端点")
	}
}


