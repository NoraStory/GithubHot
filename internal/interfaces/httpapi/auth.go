package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/netident"
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

// adminOpenMode 管理端是否处于无鉴权开放模式（密码哈希与旧令牌均为空）。
func adminOpenMode() bool {
	return os.Getenv("ADMIN_PASSWORD_HASH") == "" && os.Getenv("ADMIN_TOKEN") == ""
}

// CheckAdminAuthConfig serve 启动校验（在 adminAuth 之外兜底）：
// TLS（公网部署标志）下开放模式直接拒绝启动——与 APP_SIGN_SEED 同一严格度，
// 否则忘配密码的公网实例会把封禁/解封/信源管理/访客指纹档案全部暴露给任何人；
// 纯 HTTP 视为本地开发，保留开放模式但打 CRITICAL 级警告。
func CheckAdminAuthConfig(tlsEnabled bool) error {
	if !adminOpenMode() {
		return nil
	}
	if tlsEnabled {
		return fmt.Errorf("管理端鉴权未配置（ADMIN_PASSWORD_HASH / ADMIN_TOKEN 均为空）：TLS 公网部署下管理端完全开放，拒绝启动；"+
			"请执行 `githubhot admin hash 你的密码` 生成哈希并写入 .env 的 ADMIN_PASSWORD_HASH")
	}
	log.Printf("[CRITICAL] 管理端完全开放（ADMIN_PASSWORD_HASH / ADMIN_TOKEN 均未配置）：所有 /api/v1/admin/* 端点无需鉴权；"+
		"仅限本机开发使用，任何非回环部署必须配置其一")
	return nil
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

// checkAdminSession 校验会话 Cookie（含来源网段绑定）；有效时滑动续期，无效写 401。
// 返回 false 表示已处理响应（调用方应中断）。
func (s *Server) checkAdminSession(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || cookie.Value == "" {
		writeErr(w, 401, errorString("未登录管理端"))
		return false
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	created, expires, sessIP, found, err := s.Admin.FindAdminSession(ctx, cookie.Value)
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
	// 会话绑定来源网段：Cookie 被搬运到别处（或被拷走后异地使用）时立即注销会话。
	// 默认放宽到 IPv4 /24、IPv6 /64（移动网络出口漂移），ADMIN_SESSION_IP_STRICT=1 收紧为完全一致。
	curIP := clientIPFromRequest(r)
	if sessIP != "" && !netident.SameScope(sessIP, curIP, adminSessionIPStrict()) {
		ctx2, cancel2 := contextWithTimeout(r.Context())
		_ = s.Admin.DeleteAdminSession(ctx2, cookie.Value)
		if s.Guard != nil {
			// 强证据（身份类）但**不**给到单类即时封禁线（100）：会话已就地注销，
			// 安全效果已达成；而管理员换宽带 / 手机切基站会落到别的网段，
			// 一上来就封会把他自己挡在登录页外（被封 IP 连 /admin/login 都是 404）。
			// 80 分：重复跨网段（多次会话）或与其它证据互证才封。
			s.Guard.Event(ctx2, curIP, "admin-session-ip-mismatch",
				"管理会话跨网段使用（签发 "+sessIP+"，当前 "+curIP+"）", 80, false)
		}
		cancel2()
		// 只记前 8 位：会话 ID 本身就是可用凭证，不该整条落到日志里
		log.Printf("[admin] 会话来源不符，已注销：签发 %s / 当前 %s（会话 %s…）",
			sessIP, curIP, truncateSessionID(cookie.Value))
		writeErr(w, 401, errorString("会话与访问来源不符，已注销，请重新登录"))
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
	// 有效管理会话的 IP 加入防护白名单（免封禁，TTL 与会话一致）
	if s.Guard != nil {
		s.Guard.Whitelist(curIP, ttl)
	}
	return true
}

// adminSessionIPStrict 会话 IP 严格模式（默认 0 = 放宽到 /24 与 /64）。
func adminSessionIPStrict() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_SESSION_IP_STRICT")))
	return v == "1" || v == "true"
}

// truncateSessionID 日志用的会话 ID 前缀（凭证不落全文）。
func truncateSessionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
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
	// P4-2：WEBAUTHN_ONLY=1 时密码登录禁用（通行密钥是唯一入口）
	if passwordLoginDisabled() {
		writeErr(w, 403, errorString("密码登录已禁用（WEBAUTHN_ONLY），请使用通行密钥"))
		return
	}
	passwordHash := os.Getenv("ADMIN_PASSWORD_HASH")
	if passwordHash == "" {
		writeErr(w, 400, errorString("服务端未启用密码登录（未配置 ADMIN_PASSWORD_HASH）"))
		return
	}
	// 与 checkAdminSession 用同一套口径（受信代理下取真实客户端 IP），
	// 否则会话记录的 IP 与后续校验的 IP 不可比，正常管理员会被自己踢下线。
	ip := clientIPFromRequest(r)
	if !adminLoginGuard.allow(ip) {
		writeErr(w, 429, errorString("失败次数过多，已锁定 10 分钟"))
		return
	}
	var p loginPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&p); err != nil || p.Password == "" {
		writeErr(w, 400, errorString("请求体需要 {\"password\": \"...\"}"))
		return
	}
	ok, err := adminauth.VerifyPassword(passwordHash, p.Password)
	if err != nil {
		log.Printf("[admin] 密码校验出错: %v", err)
	}
	if !ok {
		adminLoginGuard.fail(ip)
		// 联动 IP 防护：连续爆破触发锁定 → 按强证据计分（非 severe：初犯档 30 分钟，
		// 而非旧实现的 7 天——管理员自己连错 5 次密码不应被月级封禁）
		if s.Guard != nil && !adminLoginGuard.allow(ip) {
			ctx2, cancel2 := contextWithTimeout(r.Context())
			s.Guard.Event(ctx2, ip, "admin-brute", "管理端密码爆破锁定", 100, false)
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
