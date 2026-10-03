package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- 三层 IP 身份识别体系 ----------
//
// 第一层（IP 记忆）：全站中间件按 IP 建滑动窗口档案（速率/404 率/UA 集合），
//   异常记入违规事件并轻量落库 ip_profiles。
// 第二层（设备指纹）：浏览器采集 Canvas/WebGL/WebRTC/屏幕/环境特征上报，
//   服务端维护 ip_fingerprints 的 fp↔IP↔UA 多对多关系。
// 第三层（特殊标识 + 一致性核验）：对通过初检的访客签发 HMAC 身份令牌
//   （gh_id Cookie，绑定 IP+指纹+有效期）；此后每次 API 请求带 X-Device-Fp，
//   服务端核验签名完整性、IP↔设备绑定、环境自洽；被封 IP 的指纹在新 IP 出现
//   自动连坐。10 分钟违规积分 ≥100 触发升级封禁（30m→24h→7d→30d），
//   被封 IP 全站 404。
//
// 回环/内网默认白名单（IP_GUARD_LOCAL=1），公网部署设 IP_GUARD_LOCAL=0。

const (
	idCookieName     = "gh_id"
	idCookieLifetime = 7 * 24 * time.Hour
	fpHeaderName     = "X-Device-Fp"

	windowSize      = 5 * time.Minute
	profileFlushTTL = 60 * time.Second
	eventDedupeTTL  = 5 * time.Minute
	fpReportMaxRPM  = 12

	scoreBanThreshold = 100
	banWindowSeconds  = 600

	// 严重违规：直接按 7 天档
	severeRatePerMin = 600
	// 普通阈值
	warnRatePerMin   = 150
	adminRatePerMin  = 30
	scannerMinReqs   = 50
	scanner404Ratio  = 0.4
	fpChurnMaxIPs    = 8
)

// GuardStore 防护存储端口（sqlite.DB 实现，cli 层适配）。
type GuardStore interface {
	AddIPEvent(ctx context.Context, ip, kind, detail string, score int) error
	RecentIPEventsScore(ctx context.Context, ip string, seconds int) (int, error)
	ListIPEvents(ctx context.Context, limit int) ([]IPEventDTO, error)
	UpsertFingerprint(ctx context.Context, fp, ip, ua string) ([]string, error)
	ListFingerprints(ctx context.Context, limit int) ([]FingerprintDTO, error)
	FindBan(ctx context.Context, ip string) (*BanDTO, error)
	BannedAmong(ctx context.Context, ips []string) ([]string, error)
	UpsertBan(ctx context.Context, ip string, strikes, level int, reason string, duration time.Duration) error
	ListBans(ctx context.Context) ([]BanDTO, error)
	DeleteBan(ctx context.Context, ip string) error
	TouchIPProfile(ctx context.Context, ip, ua string, reqs int) error
	FindIPProfile(ctx context.Context, ip string) (*IPProfileDTO, error)
	ListFingerprintsByIP(ctx context.Context, ip string, limit int) ([]FingerprintDTO, error)
	ListIPEventsByIP(ctx context.Context, ip string, limit int) ([]IPEventDTO, error)
}

// DTO（存储层与接口层解耦）。
type IPEventDTO struct {
	ID     int64
	IP     string
	Kind   string
	Detail string
	Score  int
	At     time.Time
}
type FingerprintDTO struct {
	Fingerprint string
	IPs         []string
	UA          string
	FirstSeen   time.Time
	LastSeen    time.Time
	Hits        int
}
type BanDTO struct {
	IP        string
	Strikes   int
	Level     int
	Reason    string
	BannedAt  time.Time
	ExpiresAt time.Time
}
type IPProfileDTO struct {
	IP        string
	FirstSeen time.Time
	LastSeen  time.Time
	Reqs      int
	UASet     []string
	UALast    string
}

// banDuration 按违规次数定封禁时长（升级制）。
func banDuration(strikes int) time.Duration {
	switch {
	case strikes <= 1:
		return 30 * time.Minute
	case strikes == 2:
		return 24 * time.Hour
	case strikes == 3:
		return 7 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

// ipWindow 单 IP 的内存滑动档案（第一层）。
type ipWindow struct {
	times    []time.Time // 最近 5 分钟请求时间
	notFound int         // 窗口内 404 数
	reqs     int         // 本小时累计
	uaSet    map[string]bool
	lastFlush time.Time
	dirty     bool
}

// IPGuard 防护引擎。
type IPGuard struct {
	store GuardStore
	key   []byte // HMAC 密钥

	enabled    bool
	localOK    bool // 回环/内网放行

	mu       sync.Mutex
	windows  map[string]*ipWindow
	talkers  map[string]int // 本小时请求计数（整点重置）
	talkersReset time.Time
	dedupe   map[string]time.Time // ip+kind → 上次事件时间
	banCache map[string]banCacheEntry
	fpReport map[string][]time.Time // 指纹上报限频
}

type banCacheEntry struct {
	banned  bool
	expires time.Time
	at      time.Time
}

// NewIPGuard 构建引擎；secret 为空时进程内随机生成（重启后旧令牌失效）。
func NewIPGuard(store GuardStore) *IPGuard {
	secret := os.Getenv("IP_GUARD_SECRET")
	key := []byte(secret)
	if len(key) < 16 {
		key = make([]byte, 32)
		_, _ = rand.Read(key)
	}
	return &IPGuard{
		store:        store,
		key:          key,
		enabled:      os.Getenv("IP_GUARD_ENABLED") != "0",
		localOK:      os.Getenv("IP_GUARD_LOCAL") != "0",
		windows:      map[string]*ipWindow{},
		talkers:      map[string]int{},
		talkersReset: time.Now().Truncate(time.Hour).Add(time.Hour),
		dedupe:       map[string]time.Time{},
		banCache:     map[string]banCacheEntry{},
		fpReport:     map[string][]time.Time{},
	}
}

// isLocalIP 回环/内网判定。
func isLocalIP(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	return p.IsLoopback() || p.IsPrivate() || p.IsLinkLocalUnicast()
}

// clientIPFromRequest 取客户端 IP（优先 X-Forwarded-For / X-Real-IP）。
func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	return clientIP(r.RemoteAddr)
}

// ---------- 身份令牌（第三层·特殊标识） ----------
//
// 令牌格式：v2.<b64url(ip|fp|issued|exp)>.<b64url(HMAC-SHA256(key, payload))>
// 载荷明文可解码（便于核验绑定关系），但任何篡改都会使签名比对失败；
// fp 为签发时绑定的设备指纹，空串表示页面导航类请求（尚未采集指纹）。

// issueIDToken 签发身份令牌。
func (g *IPGuard) issueIDToken(ip, fp string) (string, time.Time) {
	issued := time.Now()
	exp := issued.Add(idCookieLifetime)
	payload := ip + "|" + fp + "|" + strconv.FormatInt(issued.Unix(), 10) + "|" + strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, g.key)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	return "v2." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig), exp
}

// verifyIDToken 核验令牌签名与有效期；返回 (有效, 令牌绑定指纹, 令牌绑定IP, 过期时间)。
func (g *IPGuard) verifyIDToken(token, currentIP string) (bool, string, string, time.Time) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		return false, "", "", time.Time{}
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[1])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(sig) != 32 {
		return false, "", "", time.Time{}
	}
	mac := hmac.New(sha256.New, g.key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return false, "", "", time.Time{}
	}
	f := strings.Split(string(payload), "|")
	if len(f) != 4 {
		return false, "", "", time.Time{}
	}
	expUnix, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return false, "", "", time.Time{}
	}
	exp := time.Unix(expUnix, 0)
	if time.Now().After(exp) {
		return false, f[1], f[0], exp
	}
	return true, f[1], f[0], exp
}

// ---------- 封禁执行 ----------

// isBanned 查封禁（30s 负缓存 / 到期自动视为解封）。
func (g *IPGuard) isBanned(ctx context.Context, ip string) bool {
	g.mu.Lock()
	if c, ok := g.banCache[ip]; ok && time.Since(c.at) < 30*time.Second {
		g.mu.Unlock()
		return c.banned && time.Now().Before(c.expires)
	}
	g.mu.Unlock()
	ban, err := g.store.FindBan(ctx, ip)
	banned := err == nil && ban != nil && time.Now().Before(ban.ExpiresAt)
	g.mu.Lock()
	expires := time.Now()
	if ban != nil {
		expires = ban.ExpiresAt
	}
	g.banCache[ip] = banCacheEntry{banned: banned, expires: expires, at: time.Now()}
	g.mu.Unlock()
	return banned
}

// ban 执行封禁（升级制：查旧记录 strikes+1）；severe 直接不低于 7 天档。
func (g *IPGuard) ban(ctx context.Context, ip, reason string, severe bool) {
	old, err := g.store.FindBan(ctx, ip)
	strikes := 1
	if old != nil && err == nil {
		strikes = old.Strikes + 1
	}
	if severe && strikes < 3 {
		strikes = 3
	}
	dur := banDuration(strikes)
	if err := g.store.UpsertBan(ctx, ip, strikes, strikes, reason, dur); err != nil {
		log.Printf("[ipguard] 封禁写入失败 %s: %v", ip, err)
		return
	}
	g.mu.Lock()
	g.banCache[ip] = banCacheEntry{banned: true, expires: time.Now().Add(dur), at: time.Now()}
	g.mu.Unlock()
	log.Printf("[ipguard] 封禁 %s（第 %d 次，%v）：%s", ip, strikes, dur, reason)
}

// event 记一条违规事件并检查积分是否到封禁线；severe 跳过积分直接封。
func (g *IPGuard) event(ctx context.Context, ip, kind, detail string, score int, severe bool) {
	if severe {
		_ = g.store.AddIPEvent(ctx, ip, kind, detail, score)
		g.ban(ctx, ip, kind+": "+detail, true)
		return
	}
	// 同类事件 5 分钟内去重（避免刷分）
	dk := ip + "|" + kind
	g.mu.Lock()
	if t, ok := g.dedupe[dk]; ok && time.Since(t) < eventDedupeTTL {
		g.mu.Unlock()
		return
	}
	g.dedupe[dk] = time.Now()
	g.mu.Unlock()
	if err := g.store.AddIPEvent(ctx, ip, kind, detail, score); err != nil {
		log.Printf("[ipguard] 事件写入失败: %v", err)
		return
	}
	total, err := g.store.RecentIPEventsScore(ctx, ip, banWindowSeconds)
	if err != nil {
		return
	}
	if total >= scoreBanThreshold {
		g.ban(ctx, ip, kind+"(积分 "+ strconv.Itoa(total) + ")", false)
	}
}

// ---------- 中间件 ----------

// Middleware 全站防护：封禁 404 → 第一层速率记录 → 第三层身份核验。
func (g *IPGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.enabled || g.store == nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ip := clientIPFromRequest(r)
		fp := r.Header.Get(fpHeaderName)

		// 回环/内网白名单：只记录不封禁（开发环境防自锁）
		local := g.localOK && isLocalIP(ip)

		// ① 封禁检查：全站 404。
		// 例外：管理端 ipguard 端点放行给后面的 adminAuth（需有效管理会话），
		// 保证被封管理员有自救解封通道，又不给攻击者任何未授权入口。
		if g.isBanned(ctx, ip) {
			if local || strings.HasPrefix(r.URL.Path, "/api/v1/admin/ipguard/") {
				next.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}

		// ② 第一层：滑动窗口记录（healthz 不计）
		rec := &statusWriter{ResponseWriter: w, status: 200}
		win := g.record(ip, r.UserAgent())
		path := r.URL.Path

		// ③ 第三层：身份令牌核验（API/带指纹请求）
		if !local {
			g.verifyIdentity(ctx, w, r, ip, fp)
		}

		// ④ 速率与爬虫判定（除白名单）
		if !local && path != "/healthz" {
			g.evaluate(ctx, ip, path, r.UserAgent(), win)
		}

		next.ServeHTTP(rec, r)

		// ⑤ 404 率统计
		if rec.status == 404 {
			g.mu.Lock()
			if w := g.windows[ip]; w != nil {
				w.notFound++
			}
			g.mu.Unlock()
		}
	})
}

// statusWriter 捕获响应状态码。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// record 第一层记录：滑动窗口 + 小时计数 + 定期落库档案。
func (g *IPGuard) record(ip, ua string) *ipWindow {
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if now.After(g.talkersReset) {
		g.talkers = map[string]int{}
		g.talkersReset = now.Truncate(time.Hour).Add(time.Hour)
	}
	g.talkers[ip]++
	w := g.windows[ip]
	if w == nil {
		w = &ipWindow{uaSet: map[string]bool{}, lastFlush: now}
		g.windows[ip] = w
	}
	cutoff := now.Add(-windowSize)
	keep := w.times[:0]
	for _, t := range w.times {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	w.times = append(keep, now)
	w.reqs++
	if ua != "" {
		w.uaSet[ua] = true
	}
	w.dirty = true
	// 档案落频：每 60s 一次；顺带清理闲置窗口
	if now.Sub(w.lastFlush) >= profileFlushTTL {
		delta := w.reqs
		w.reqs = 0
		w.lastFlush = now
		w.dirty = false
		go func(ip, ua string, n int) {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := g.store.TouchIPProfile(c, ip, ua, n); err != nil {
				log.Printf("[ipguard] 档案落库失败 %s: %v", ip, err)
			}
		}(ip, ua, delta)
	}
	for k, x := range g.windows {
		if len(x.times) == 0 && now.Sub(x.lastFlush) > 2*windowSize {
			delete(g.windows, k)
		}
	}
	return w
}

// evaluate 第一层/第三层违规判定（速率、爬虫、扫描器）。
func (g *IPGuard) evaluate(ctx context.Context, ip, path, ua string, w *ipWindow) {
	g.mu.Lock()
	rate1m := 0
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	for _, t := range w.times {
		if t.After(cutoff) {
			rate1m++
		}
	}
	nf := w.notFound
	total := len(w.times)
	g.mu.Unlock()

	// 严重：流量攻击
	if rate1m >= severeRatePerMin {
		g.event(ctx, ip, "traffic-attack", sprintf("%d req/min", rate1m), 100, true)
		return
	}
	// 普通速率超限
	if rate1m >= warnRatePerMin {
		g.event(ctx, ip, "rate", sprintf("%d req/min", rate1m), 60, false)
	}
	// 管理端探测
	if strings.HasPrefix(path, "/api/v1/admin") && rate1m >= adminRatePerMin {
		g.event(ctx, ip, "admin-probe", sprintf("admin API %d req/min", rate1m), 50, false)
	}
	// 扫描器：高请求量 + 高 404 率
	if total >= scannerMinReqs && float64(nf)/float64(total) >= scanner404Ratio {
		g.event(ctx, ip, "scanner", sprintf("404 率 %.0f%%（%d/%d）", float64(nf)/float64(total)*100, nf, total), 40, false)
	}
	// 爬虫 UA
	lowUA := strings.ToLower(ua)
	if (ua == "" || strings.Contains(lowUA, "curl") || strings.Contains(lowUA, "wget") ||
		strings.Contains(lowUA, "python") || strings.Contains(lowUA, "scrapy") ||
		strings.Contains(lowUA, "httpclient") || strings.Contains(lowUA, "bot") && !strings.Contains(lowUA, "mozilla")) &&
		!strings.HasPrefix(path, "/assets/") {
		g.event(ctx, ip, "bot-ua", ua, 25, false)
	}
}

// verifyIdentity 第三层：令牌完整性 + IP↔设备绑定核验。
func (g *IPGuard) verifyIdentity(ctx context.Context, w http.ResponseWriter, r *http.Request, ip, fp string) {
	cookie, err := r.Cookie(idCookieName)
	if err != nil || cookie.Value == "" {
		// 无令牌：带指纹的 API 请求直接签发；纯页面请求等首次 API 调用再签
		if fp != "" {
			g.issue(w, ip, fp)
		}
		return
	}
	ok, tokFp, tokIP, exp := g.verifyIDToken(cookie.Value, ip)
	if !ok {
		if !exp.IsZero() && time.Now().After(exp) {
			g.issue(w, ip, fp) // 过期属正常，重新签发
			return
		}
		// 签名错误且未过期 = 伪造/篡改 → 严重违规
		g.event(ctx, ip, "id-forgery", "身份令牌签名无效", 100, true)
		return
	}
	// 令牌绑定的 IP 与当前不符：Cookie 被搬到别的网络（也可能是宽带换 IP）
	if tokIP != ip {
		g.event(ctx, ip, "id-ip-drift", sprintf("令牌绑定 %s，当前 %s", tokIP, ip), 60, false)
		g.issue(w, ip, fp)
		return
	}
	// 设备指纹与令牌绑定的不符：Cookie 被搬到别的设备 → 高危
	if fp != "" && tokFp != "" && fp != tokFp {
		g.event(ctx, ip, "device-mismatch", "设备指纹与身份令牌绑定不符", 80, false)
	}
	// 临近过期滑动续期
	if time.Until(exp) < 24*time.Hour {
		g.issue(w, ip, fp)
	}
}

// issue 签发身份令牌 Cookie。
func (g *IPGuard) issue(w http.ResponseWriter, ip, fp string) {
	token, exp := g.issueIDToken(ip, fp)
	http.SetCookie(w, &http.Cookie{
		Name:     idCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // 允许页面导航正常回带
		Secure:   false,                // 本地 HTTP 部署；HTTPS 下由反向代理改 Secure
		Expires:  exp,
	})
}

// ---------- 对外能力（handler / 管理端用） ----------

// ReportFingerprint 第二层上报入口：登记 fp↔IP，做连坐与漂移判定。
// 返回 (响应字段, 该 IP 是否刚被封)。
func (g *IPGuard) ReportFingerprint(ctx context.Context, ip, ua, fp string) (map[string]any, bool) {
	out := map[string]any{"ok": true, "banned": false}
	if !g.enabled || g.store == nil || fp == "" {
		return out, false
	}
	local := g.localOK && isLocalIP(ip)

	// 上报限频：12 次/分钟
	g.mu.Lock()
	now := time.Now()
	lst := g.fpReport[ip]
	keep := lst[:0]
	for _, t := range lst {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= fpReportMaxRPM {
		g.mu.Unlock()
		out["ok"] = false
		out["error"] = "report too frequent"
		return out, false
	}
	g.fpReport[ip] = append(keep, now)
	g.mu.Unlock()

	knownIPs, err := g.store.UpsertFingerprint(ctx, fp, ip, ua)
	if err != nil {
		log.Printf("[ipguard] 指纹登记失败: %v", err)
		return out, false
	}
	if local {
		return out, false
	}
	// 连坐：该指纹历史上关联的 IP 有在封禁期内的 → 本 IP 同档封禁
	bannedKin, err := g.store.BannedAmong(ctx, knownIPs)
	if err == nil && len(bannedKin) > 0 && !contains(knownIPs[:len(knownIPs)-1], ip) {
		// 只在"本 IP 新出现在该指纹下"时连坐，避免已封 IP 反复上报刷日志
		g.event(ctx, ip, "fp-linked", sprintf("指纹曾关联封禁 IP %s", strings.Join(bannedKin, ",")), 100, true)
		out["banned"] = true
		return out, true
	}
	// 漂移：指纹 24h 内换 IP 过多 → 代理池
	if len(knownIPs) > fpChurnMaxIPs {
		g.event(ctx, ip, "fp-churn", sprintf("指纹关联 %d 个 IP", len(knownIPs)), 40, false)
	}
	return out, false
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Talkers Top 访客（内存近 1 小时计数）。
func (g *IPGuard) Talkers() []map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	type kv struct {
		ip string
		n  int
	}
	all := []kv{}
	for ip, n := range g.talkers {
		all = append(all, kv{ip, n})
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].n > all[i].n {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	out := []map[string]any{}
	for i, x := range all {
		if i >= 10 {
			break
		}
		out = append(out, map[string]any{"ip": x.ip, "reqs": x.n})
	}
	return out
}

// ResetBanCache 解封后刷新缓存。
func (g *IPGuard) ResetBanCache(ip string) {
	g.mu.Lock()
	delete(g.banCache, ip)
	g.mu.Unlock()
}

// BanIP 手动封禁（管理端）。
func (g *IPGuard) BanIP(ctx context.Context, ip, reason string, hours int) error {
	dur := time.Duration(hours) * time.Hour
	if dur <= 0 {
		dur = 24 * time.Hour
	}
	old, err := g.store.FindBan(ctx, ip)
	strikes := 1
	if old != nil && err == nil {
		strikes = old.Strikes + 1
	}
	if err := g.store.UpsertBan(ctx, ip, strikes, strikes, reason, dur); err != nil {
		return err
	}
	g.mu.Lock()
	g.banCache[ip] = banCacheEntry{banned: true, expires: time.Now().Add(dur), at: time.Now()}
	g.mu.Unlock()
	log.Printf("[ipguard] 手动封禁 %s（%v）：%s", ip, dur, reason)
	return nil
}
