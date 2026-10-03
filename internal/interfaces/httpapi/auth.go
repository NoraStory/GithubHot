package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/adminauth"
)

// ---------- 管理端鉴权（Argon2id 密码 + 服务端会话 Cookie） ----------
//
// 三种模式按 .env 配置自动切换，均只作用于 /api/v1/admin/*：
//  1. ADMIN_PASSWORD_HASH 已配置（推荐）：POST /admin/login 验证 Argon2id 哈希，
//     通过后下发 HttpOnly + SameSite=Strict 会话 Cookie（gh_admin_session），
//     会话存 SQLite admin_sessions 表，12h 有效、活跃滑动续期，"记住我" 7 天。
//  2. ADMIN_TOKEN 已配置（旧版兼容）：请求头 X-Admin-Token 必须等于该值。
//  3. 两者都没配：管理端完全开放（仅本地开发用）。

const (
	adminCookieName   = "gh_admin_session"
	adminSessionTTL   = 12 * time.Hour
	adminRememberTTL  = 7 * 24 * time.Hour
	loginMaxFails     = 5
	loginLockDuration = 10 * time.Minute
)

// AdminSessions 会话存储端口（由 sqlite.DB 实现）。
type AdminSessions interface {
	CreateAdminSession(ctx context.Context, id, ip string, ttl time.Duration) error
	FindAdminSession(ctx context.Context, id string) (created, expires time.Time, ip string, found bool, err error)
	RenewAdminSession(ctx context.Context, id string, ttl time.Duration) error
	DeleteAdminSession(ctx context.Context, id string) error
}

// adminAuth 管理端守卫：按上述三种模式校验。
func (s *Server) adminAuth(next http.Handler) http.Handler {
	passwordHash := os.Getenv("ADMIN_PASSWORD_HASH")
	legacyToken := os.Getenv("ADMIN_TOKEN")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case passwordHash != "":
			if !s.checkAdminSession(w, r) {
				return
			}
		case legacyToken != "":
			if r.Header.Get("X-Admin-Token") != legacyToken {
				writeErr(w, 401, errorString("需要 X-Admin-Token（ADMIN_TOKEN 已启用）"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// checkAdminSession 校验会话 Cookie；有效时滑动续期，无效写 401。
// 返回 false 表示已处理响应（调用方应中断）。
func (s *Server) checkAdminSession(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || cookie.Value == "" {
		writeErr(w, 401, errorString("未登录管理端"))
		return false
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	created, expires, _, found, err := s.Admin.FindAdminSession(ctx, cookie.Value)
	if err != nil || !found {
		if err != nil {
			log.Printf("[admin] 会话查询失败: %v", err)
		}
		writeErr(w, 401, errorString("会话无效或已过期，请重新登录"))
		return false
	}
	now := time.Now()
	if now.After(expires) {
		ctx2, cancel2 := contextWithTimeout(r.Context())
		_ = s.Admin.DeleteAdminSession(ctx2, cookie.Value)
		cancel2()
		writeErr(w, 401, errorString("会话已过期，请重新登录"))
		return false
	}
	// 活跃会话滑动续期：剩余不足一半时延长一个完整周期
	ttl := adminSessionTTL
	if expires.Sub(created) > adminSessionTTL {
		ttl = expires.Sub(created) // 保持"记住我"长周期不变
	}
	if expires.Sub(now) < ttl/2 {
		ctx2, cancel3 := contextWithTimeout(r.Context())
		if err := s.Admin.RenewAdminSession(ctx2, cookie.Value, ttl); err != nil {
			log.Printf("[admin] 会话续期失败: %v", err)
		}
		cancel3()
	}
	return true
}

// clientIP 从 RemoteAddr 取纯 IP（去掉随机源端口，否则每次连接都像新客户端）。
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// loginRateLimiter（内存计数器，单机部署足够）

type loginGuard struct {
	mu      sync.Mutex
	attempts map[string]*ipAttempts
}

type ipAttempts struct {
	fails       int
	lockedUntil time.Time
}

var adminLoginGuard = &loginGuard{attempts: map[string]*ipAttempts{}}

// allow 该 IP 当前是否允许尝试登录。
func (g *loginGuard) allow(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	a := g.attempts[ip]
	if a == nil {
		return true
	}
	// 锁定期内一律拒绝；注意不能在这里清零失败计数，
	// 否则每次 allow 都会把累计次数冲掉，永远锁不住。
	return !time.Now().Before(a.lockedUntil)
}

// fail 记录一次失败；连续 5 次锁定 10 分钟。
func (g *loginGuard) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a := g.attempts[ip]
	if a == nil {
		a = &ipAttempts{}
		g.attempts[ip] = a
	}
	a.fails++
	if a.fails >= loginMaxFails {
		a.lockedUntil = time.Now().Add(loginLockDuration)
		a.fails = 0
		log.Printf("[admin] %s 登录失败 %d 次，锁定 %v", ip, loginMaxFails, loginLockDuration)
	}
}

// succeed 登录成功后清零计数。
func (g *loginGuard) succeed(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, ip)
}

// ---------- 登录 / 退出 / 会话检查 ----------

type loginPayload struct {
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// adminLogin POST /api/v1/admin/login：验证密码 → 建会话 → 下发 Cookie。
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	passwordHash := os.Getenv("ADMIN_PASSWORD_HASH")
	if passwordHash == "" {
		writeErr(w, 400, errorString("服务端未启用密码登录（未配置 ADMIN_PASSWORD_HASH）"))
		return
	}
	ip := clientIP(r.RemoteAddr)
	if !adminLoginGuard.allow(ip) {
		writeErr(w, 429, errorString("失败次数过多，已锁定 10 分钟"))
		return
	}
	var p loginPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Password == "" {
		writeErr(w, 400, errorString("请求体需要 {\"password\": \"...\"}"))
		return
	}
	ok, err := adminauth.VerifyPassword(passwordHash, p.Password)
	if err != nil {
		log.Printf("[admin] 密码校验出错: %v", err)
	}
	if !ok {
		adminLoginGuard.fail(ip)
		// 联动 IP 防护：连续爆破触发锁定时按严重违规封禁
		if s.Guard != nil && !adminLoginGuard.allow(ip) {
			ctx2, cancel2 := contextWithTimeout(r.Context())
			s.Guard.Event(ctx2, clientIP(r.RemoteAddr), "admin-brute", "管理端密码爆破锁定", 100, true)
			cancel2()
		}
		log.Printf("[admin] %s 登录失败", ip)
		writeErr(w, 401, errorString("账号或密码错误"))
		return
	}
	adminLoginGuard.succeed(ip)

	// 256 位随机会话 ID
	sid := make([]byte, 32)
	if _, err := rand.Read(sid); err != nil {
		writeErr(w, 500, errorString("生成会话失败"))
		return
	}
	sessionID := base64.RawURLEncoding.EncodeToString(sid)
	ttl := adminSessionTTL
	if p.Remember {
		ttl = adminRememberTTL
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	if err := s.Admin.CreateAdminSession(ctx, sessionID, ip, ttl); err != nil {
		writeErr(w, 500, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   int(ttl.Seconds()),
	})
	log.Printf("[admin] %s 登录成功（%v）", ip, ttl)
	writeJSON(w, 200, map[string]any{"ok": true, "expiresInHours": ttl.Hours()})
}

// adminLogout POST /api/v1/admin/logout：删服务端会话 + 清 Cookie。
func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminCookieName); err == nil && cookie.Value != "" {
		ctx, cancel := contextWithTimeout(r.Context())
		defer cancel()
		if err := s.Admin.DeleteAdminSession(ctx, cookie.Value); err != nil {
			log.Printf("[admin] 删除会话失败: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// adminSessionCheck GET /api/v1/admin/session：前端路由守卫探测登录态。
// 不滑动续期（避免探测放大写入），只读校验。
func (s *Server) adminSessionCheck(w http.ResponseWriter, r *http.Request) {
	passwordHash := os.Getenv("ADMIN_PASSWORD_HASH")
	legacyToken := os.Getenv("ADMIN_TOKEN")
	if passwordHash == "" && legacyToken == "" {
		writeJSON(w, 200, map[string]any{"valid": true, "mode": "open"})
		return
	}
	if passwordHash == "" {
		// 旧令牌模式：前端已无法证明持有令牌，视为未登录
		writeErr(w, 401, errorString("未登录管理端"))
		return
	}
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || cookie.Value == "" {
		writeErr(w, 401, errorString("未登录管理端"))
		return
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	_, expires, _, found, err := s.Admin.FindAdminSession(ctx, cookie.Value)
	if err != nil || !found || time.Now().After(expires) {
		writeErr(w, 401, errorString("会话无效或已过期"))
		return
	}
	writeJSON(w, 200, map[string]any{"valid": true, "expiresAt": expires.Format(time.RFC3339)})
}
