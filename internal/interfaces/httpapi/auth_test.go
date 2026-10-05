package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeAdminSessions 内存版会话存储（含 IP 绑定字段）。
type fakeAdminSessions struct {
	sessions map[string]adminSessionRow
}

type adminSessionRow struct {
	created, expires time.Time
	ip               string
}

func newFakeAdminSessions() *fakeAdminSessions {
	return &fakeAdminSessions{sessions: map[string]adminSessionRow{}}
}

func (f *fakeAdminSessions) CreateAdminSession(_ context.Context, id, ip string, ttl time.Duration) error {
	f.sessions[id] = adminSessionRow{created: time.Now(), expires: time.Now().Add(ttl), ip: ip}
	return nil
}

func (f *fakeAdminSessions) FindAdminSession(_ context.Context, id string) (time.Time, time.Time, string, bool, error) {
	s, ok := f.sessions[id]
	if !ok {
		return time.Time{}, time.Time{}, "", false, nil
	}
	return s.created, s.expires, s.ip, true, nil
}

func (f *fakeAdminSessions) RenewAdminSession(_ context.Context, id string, ttl time.Duration) error {
	if s, ok := f.sessions[id]; ok {
		s.expires = time.Now().Add(ttl)
		f.sessions[id] = s
	}
	return nil
}

func (f *fakeAdminSessions) DeleteAdminSession(_ context.Context, id string) error {
	delete(f.sessions, id)
	return nil
}

// adminReq 构造带会话 Cookie 的管理端请求（RemoteAddr 即客户端 IP）。
func adminReq(sid, ip string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ipguard/summary", nil)
	req.RemoteAddr = ip + ":44321"
	if sid != "" {
		req.AddCookie(&http.Cookie{Name: adminCookieName, Value: sid})
	}
	return req
}

// TestAdminSessionSameSubnetPasses 同 /24 漂移放行（移动网络 / 重拨换 IP 的日常场景）。
func TestAdminSessionSameSubnetPasses(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "")
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}

	ctx := context.Background()
	if err := sessions.CreateAdminSession(ctx, "sid-1", "203.0.113.7", time.Hour); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if !s.checkAdminSession(rec, adminReq("sid-1", "203.0.113.99")) {
		t.Fatalf("同 /24 应放行，实际 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.events) != 0 {
		t.Fatalf("同网段漂移不应记违规，实际 %v", store.kindsOf("203.0.113.99"))
	}
}

// TestAdminSessionCrossSubnetRevoked 跨 /24：注销会话 + 记强证据违规 + 401。
func TestAdminSessionCrossSubnetRevoked(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "")
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}

	ctx := context.Background()
	if err := sessions.CreateAdminSession(ctx, "sid-2", "203.0.113.7", time.Hour); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if s.checkAdminSession(rec, adminReq("sid-2", "198.51.100.7")) {
		t.Fatalf("跨网段应拒绝")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("应 401，实际 %d", rec.Code)
	}
	if _, ok := sessions.sessions["sid-2"]; ok {
		t.Fatalf("会话应被注销")
	}
	store.mu.Lock()
	var score int
	var kind string
	for _, e := range store.events {
		if e.Kind == "admin-session-ip-mismatch" {
			score, kind = e.Score, e.Kind
		}
	}
	store.mu.Unlock()
	if kind == "" {
		t.Fatalf("应记 admin-session-ip-mismatch，实际 %v", store.kindsOf("198.51.100.7"))
	}
	if score != 80 {
		t.Fatalf("跨网段证据应计 80 分（低于单类即时封禁线 100），实际 %d", score)
	}
	if ban := store.banOf("198.51.100.7"); ban != nil {
		t.Fatalf("单次不符不应立即封禁（管理员换网段会自锁）：%s", ban.Reason)
	}
}

// TestAdminSessionStrictModeRejectsSameSubnet 严格模式：同 /24 也算不符。
func TestAdminSessionStrictModeRejectsSameSubnet(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "1")
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}

	ctx := context.Background()
	if err := sessions.CreateAdminSession(ctx, "sid-3", "203.0.113.7", time.Hour); err != nil {
		t.Fatal(err)
	}
	if s.checkAdminSession(httptest.NewRecorder(), adminReq("sid-3", "203.0.113.99")) {
		t.Fatalf("严格模式应拒绝同 /24 漂移")
	}
	if s.checkAdminSession(httptest.NewRecorder(), adminReq("sid-3", "203.0.113.7")) {
		t.Fatalf("严格模式下 IP 完全一致应放行")
	}
}

// TestAdminSessionEmptyIPNotChecked 会话记录 IP 为空（历史数据）时不因 IP 拒绝。
func TestAdminSessionEmptyIPNotChecked(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}

	ctx := context.Background()
	if err := sessions.CreateAdminSession(ctx, "sid-4", "", time.Hour); err != nil {
		t.Fatal(err)
	}
	if !s.checkAdminSession(httptest.NewRecorder(), adminReq("sid-4", "198.51.100.7")) {
		t.Fatalf("会话无 IP 记录时不应因 IP 拦截")
	}
}

// TestAdminSessionMismatchDedupWithinWindow 5 分钟内同类事件去重：被盗会话被连续
// 尝试使用时只记一条、不封（避免一次误判被反复放大）。
func TestAdminSessionMismatchDedupWithinWindow(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "")
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}
	ctx := context.Background()

	const attackerIP = "198.51.100.7"
	for _, sid := range []string{"sid-a", "sid-b"} {
		if err := sessions.CreateAdminSession(ctx, sid, "203.0.113.7", time.Hour); err != nil {
			t.Fatal(err)
		}
		if s.checkAdminSession(httptest.NewRecorder(), adminReq(sid, attackerIP)) {
			t.Fatalf("跨网段应拒绝")
		}
	}
	if ban := store.banOf(attackerIP); ban != nil {
		t.Fatalf("5 分钟内的重复不应放大成封禁：%s", ban.Reason)
	}
	if n := store.kindsOf(attackerIP)["admin-session-ip-mismatch"]; n != 1 {
		t.Fatalf("同类事件应去重为 1 条，实际 %d", n)
	}
}

// TestAdminSessionSustainedMismatchEscalates 持续跨网段使用（去重窗口外的同类事件）
// 越强证据单类门槛（100）→ 升级封禁，判定能力保留。
func TestAdminSessionSustainedMismatchEscalates(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "")
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}
	ctx := context.Background()

	const attackerIP = "198.51.100.7"
	// 6 分钟前已发生一次（去重窗口外，衰减后 80×0.5^0.6 ≈ 53），本次再加 80 → 越线
	store.seedEvent(attackerIP, "admin-session-ip-mismatch", 80, 6)
	if err := sessions.CreateAdminSession(ctx, "sid-c", "203.0.113.7", time.Hour); err != nil {
		t.Fatal(err)
	}
	if s.checkAdminSession(httptest.NewRecorder(), adminReq("sid-c", attackerIP)) {
		t.Fatalf("跨网段应拒绝")
	}
	if ban := store.banOf(attackerIP); ban == nil {
		t.Fatalf("持续跨网段使用管理会话应升级封禁")
	}
}

// TestAdminLoginRecordsRequestIP 登录与会话校验必须用同一 IP 口径
// （受信代理下取真实客户端 IP），否则正常管理员会被自己踢下线。
func TestAdminLoginRecordsRequestIP(t *testing.T) {
	t.Setenv("ADMIN_SESSION_IP_STRICT", "")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	store := newFakeStore()
	sessions := newFakeAdminSessions()
	s := &Server{Admin: sessions, Guard: NewIPGuard(store)}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ipguard/summary", nil)
	req.RemoteAddr = "127.0.0.1:5555" // 本机反向代理
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "sid-5"})
	ctx := context.Background()
	if err := sessions.CreateAdminSession(ctx, "sid-5", clientIPFromRequest(req), time.Hour); err != nil {
		t.Fatal(err)
	}
	if !s.checkAdminSession(httptest.NewRecorder(), req) {
		t.Fatalf("同一请求的 IP 口径应一致（XFF 解析后），不应拒绝")
	}
}
