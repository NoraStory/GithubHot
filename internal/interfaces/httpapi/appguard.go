package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/config"
)

// ---------- APP 侧风控（Android 客户端） ----------
//
// 与 android-app SignEngine/SessionManager 逐字节对齐的校验端：
//   sessionKey = HMAC-SHA256(key=(seed+":"+fp), msg="gh-session-v1")
//   签名        = HMAC-SHA256(key=sessionKey, msg=ts+"\n"+nonce+"\n"+METHOD+"\n"+path+query+"\n"+sha256hex(body))
//   会话令牌    = hex(sha256(sessionKey + fp))[:40]
// 校验通过 → 设备指纹/型号/威胁报告归档进 IP 防护档案（管理端 IP 下钻可见）；
// 校验失败 → 记违规事件（积分与 IP 防护体系共用，累计到线自动封禁）；
// APP_BANNED=1 / APP_FORCE_UPGRADE_URL 设置时直接拒签（远程封禁/强制更新生效）。

const (
	appSignHeader = "X-App-Sign"
	appTsHeader   = "X-App-Ts"
	appNonceHeader = "X-App-Nonce"
	appTokenHeader = "X-Session-Token"
	appFpHeader   = "X-Device-Fingerprint"
	appModelHeader = "X-Device-Model"
	appThreatHeader = "X-Threat-Report"
	appBrowserFpHeader = "X-Browser-Fp" // APP WebView 浏览器指纹（base64url JSON）

	appSignWindow    = 300 * time.Second // 时间戳容差 ±5min（设备时钟漂移）
	appNonceTTL      = 10 * time.Minute
	appFpRecordTTL   = time.Minute // 同指纹归档限频
	appMaxBody       = 1 << 20
)

// AppGuard APP 请求防护引擎。
type AppGuard struct {
	store  GuardStore
	guard  *IPGuard // 违规事件/封禁复用三层 IP 防护体系
	seeds  []string // [0] 主种子；其后为 APP_SIGN_SEED_GRACE 过渡期旧种子
	seed   string   // = seeds[0]（签名向量与令牌派生用主种子）
	grace  bool     // 过渡期：无常用种子验签通过时只观察不计分（存量 APP 尚在改构建期注入）
	banned bool
	force  string

	mu        sync.Mutex
	nonces    map[string]time.Time
	fpLast    map[string]time.Time
}

// NewAppGuard 构建。种子只从环境读取（APP_SIGN_SEED / 旧名 APP_SESSION_SEED），
// 不再由 /api/v1/site/config 公开下发；未配置时沿用出厂默认（serve 模式会被
// config.CheckAppSignSeed 拒绝启动）。
func NewAppGuard(store GuardStore, guard *IPGuard) *AppGuard {
	seeds := appSignSeeds()
	return &AppGuard{
		store:  store,
		guard:  guard,
		seeds:  seeds,
		seed:   seeds[0],
		grace:  len(seeds) > 1,
		banned: osBanned(),
		force:  strings.TrimSpace(os.Getenv("APP_FORCE_UPGRADE_URL")),
		nonces: map[string]time.Time{},
		fpLast: map[string]time.Time{},
	}
}

// appSignSeeds 验签种子列表：主种子在前，过渡期旧种子在后（去重、保序、非空）。
func appSignSeeds() []string {
	primary := config.AppSignSeedFromEnv()
	if primary == "" {
		primary = config.DevAppSignSeed // run/mcp/测试模式：保持与历史行为一致
	}
	out := []string{primary}
	seen := map[string]bool{primary: true}
	for _, s := range config.AppSignSeedGraceFromEnv() {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func osBanned() bool {
	return os.Getenv("APP_BANNED") == "1" || strings.EqualFold(os.Getenv("APP_BANNED"), "true")
}

// deriveSessionKey 与客户端 SessionManager 完全一致的密钥派生（指定种子）。
func deriveSessionKeyWith(seed, fp string) []byte {
	secret := []byte(seed + ":" + fp)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("gh-session-v1"))
	return mac.Sum(nil)
}

// deriveSessionKey 主种子的密钥派生。
func (a *AppGuard) deriveSessionKey(fp string) []byte {
	return deriveSessionKeyWith(a.seed, fp)
}

// expectedToken 与客户端 SessionManager 一致的会话令牌（指定种子）。
func expectedTokenWith(seed, fp string) string {
	key := deriveSessionKeyWith(seed, fp)
	h := sha256.New()
	h.Write(key)
	h.Write([]byte(fp))
	return hex.EncodeToString(h.Sum(nil))[:40]
}

// expectedToken 主种子的会话令牌。
func (a *AppGuard) expectedToken(fp string) string {
	return expectedTokenWith(a.seed, fp)
}

// expectedSign 重算请求签名（path 含 query，body 为原始字节；指定种子）。
func expectedSignWith(seed, fp, ts, nonce, method, path string, body []byte) string {
	bh := sha256.Sum256(body)
	payload := ts + "\n" + nonce + "\n" + strings.ToUpper(method) + "\n" + path + "\n" + hex.EncodeToString(bh[:])
	mac := hmac.New(sha256.New, deriveSessionKeyWith(seed, fp))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// expectedSign 主种子的请求签名。
func (a *AppGuard) expectedSign(fp, ts, nonce, method, path string, body []byte) string {
	return expectedSignWith(a.seed, fp, ts, nonce, method, path, body)
}

// signMatches 逐个种子校验签名，返回命中的种子下标（-1 = 无一命中）。
func (a *AppGuard) signMatches(fp, ts, nonce, method, path string, body []byte, sig string) int {
	want := strings.ToLower(sig)
	for i, seed := range a.seeds {
		if hmac.Equal([]byte(expectedSignWith(seed, fp, ts, nonce, method, path, body)), []byte(want)) {
			return i
		}
	}
	return -1
}

// tokenMatches 会话令牌是否与任一种子下的期望值一致（过渡期旧种子同样受理）。
func (a *AppGuard) tokenMatches(fp, tok string) bool {
	for _, seed := range a.seeds {
		if hmac.Equal([]byte(expectedTokenWith(seed, fp)), []byte(tok)) {
			return true
		}
	}
	return false
}

// fail 校验失败：记违规事件（与 IP 防护积分/封禁打通）并返回 401。
func (a *AppGuard) fail(ctx context.Context, ip, kind, detail string, score int) {
	if a.guard != nil {
		a.guard.Event(ctx, ip, kind, detail, score, false)
	}
}

// Middleware 仅处理带 X-App-Sign 的 APP 请求；浏览器流量直接放行给后续。
func (a *AppGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get(appSignHeader)
		if sig == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ip := clientIPFromRequest(r)
		fp := r.Header.Get(appFpHeader)

		// 握手引导通道：种子/风控策略下发本身无需会话，跳过签名校验与封禁拦截
		// （客户端据此拿到 session_seed 派生密钥，后续数据接口才走强校验）。
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/site/config" {
			if fp != "" {
				a.recordFingerprint(ctx, ip, fp, r.Header.Get(appModelHeader), r.Header.Get(appThreatHeader), r.Header.Get(appBrowserFpHeader))
			}
			next.ServeHTTP(w, r)
			return
		}

		// 远程封禁 / 强制更新：握手通道已下发，这里在服务端同样执行。
		if a.banned {
			writeJSON(w, 403, map[string]any{"error": "banned"})
			return
		}
		if a.force != "" {
			writeJSON(w, 403, map[string]any{"error": "force_upgrade", "url": a.force})
			return
		}
		if fp == "" {
			a.fail(ctx, ip, "app-no-fp", "APP 请求缺少设备指纹头", 40)
			writeJSON(w, 401, map[string]any{"error": "missing fingerprint"})
			return
		}

		// 读取 body 参与签名并重置，保证下游 handler 还能读。
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(r.Body, appMaxBody))
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		ts := r.Header.Get(appTsHeader)
		nonce := r.Header.Get(appNonceHeader)
		tm, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || time.Since(time.UnixMilli(tm)) > appSignWindow ||
			time.Until(time.UnixMilli(tm)) > appSignWindow {
			a.fail(ctx, ip, "app-expired", "请求时间戳超窗", 30)
			writeJSON(w, 401, map[string]any{"error": "expired"})
			return
		}
		if a.nonceSeen(nonce) {
			a.fail(ctx, ip, "app-nonce-replay", "nonce 重放", 60)
			writeJSON(w, 401, map[string]any{"error": "replay"})
			return
		}
		path := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		if a.signMatches(fp, ts, nonce, r.Method, path, body, sig) < 0 {
			if a.grace {
				// 过渡期：存量 APP 仍持有握手期种子的缓存，无常用种子验签通过时
				// 只观察不计分（否则新购机/清数据的正常用户会被连续 401 攒到封禁）
				a.fail(ctx, ip, "app-sign-unverified", "接口签名无法用任何已知种子校验（过渡期观察）", 0)
			} else {
				a.fail(ctx, ip, "app-sign-invalid", "接口签名无效", 60)
			}
			writeJSON(w, 401, map[string]any{"error": "bad sign"})
			return
		}
		if tok := r.Header.Get(appTokenHeader); tok != "" && !a.tokenMatches(fp, tok) {
			a.fail(ctx, ip, "app-token-mismatch", "会话令牌与指纹绑定不符", 40)
			writeJSON(w, 401, map[string]any{"error": "bad token"})
			return
		}

		// 校验通过：指纹/型号/威胁报告归档（限频 1 次/分/指纹），
		// 管理端 /admin/ipguard 单 IP 下钻可见 APP 设备档案。
		a.recordFingerprint(ctx, ip, fp, r.Header.Get(appModelHeader), r.Header.Get(appThreatHeader), r.Header.Get(appBrowserFpHeader))
		next.ServeHTTP(w, r)
	})
}

// nonceSeen 记录并判定 nonce 是否已用（10 分钟窗，惰性清理）。
func (a *AppGuard) nonceSeen(nonce string) bool {
	if nonce == "" {
		return true // 空 nonce 视为重放
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if len(a.nonces) > 10000 {
		for k, t := range a.nonces {
			if now.Sub(t) > appNonceTTL {
				delete(a.nonces, k)
			}
		}
	}
	if t, ok := a.nonces[nonce]; ok && now.Sub(t) < appNonceTTL {
		return true
	}
	a.nonces[nonce] = now
	return false
}

// recordFingerprint APP 设备指纹归档进 IP 防护体系（带型号、威胁命中与 WebView 浏览器分量）。
func (a *AppGuard) recordFingerprint(ctx context.Context, ip, fp, model, threat, browserFp string) {
	if a.store == nil {
		return
	}
	a.mu.Lock()
	// 限频键含 payload 摘要：完全相同的上报 1 次/分，携带新分量（渐进补齐）的不限
	last := a.fpLast[fp+"|"+ip+"|"+payloadTag(model, threat, browserFp)]
	if time.Since(last) < appFpRecordTTL {
		a.mu.Unlock()
		return
	}
	a.fpLast[fp+"|"+ip+"|"+payloadTag(model, threat, browserFp)] = time.Now()
	a.mu.Unlock()

	threat = strings.TrimSpace(threat)
	flags := []string{}
	if threat != "" {
		for _, t := range strings.Split(threat, ",") {
			if t = strings.TrimSpace(t); t != "" {
				flags = append(flags, "app-"+t)
			}
		}
	}
	ua := "GithubHot-APP"
	if model != "" {
		ua = "GithubHot-APP (" + model + ")"
	}
	components := map[string]string{"client": "android"}
	if model != "" {
		components["model"] = model
	}
	if threat != "" {
		components["threat"] = threat
	}
	// WebView 浏览器指纹分量（Canvas/WebGL/音频/字体/显卡/屏幕），键白名单 + 长度截断
	for k, v := range parseBrowserFp(browserFp) {
		components[k] = v
	}
	if _, err := a.store.UpsertFingerprint(ctx, fp, ip, ua, FingerprintMeta{Components: components, Flags: flags}); err != nil {
		log.Printf("[appguard] 指纹归档失败 %s: %v", fp, err)
	}
}

// payloadTag 限频键的 payload 摘要（FNV-1a，非加密用途）。
func payloadTag(s ...string) string {
	h := uint32(2166136261)
	for _, part := range s {
		for i := 0; i < len(part); i++ {
			h ^= uint32(part[i])
			h *= 16777619
		}
		h ^= 0xff
	}
	return fmt.Sprintf("%08x", h)
}

// browserFpKeys APP 可上报的浏览器分量键白名单（与管理端指纹表列对应）。
var browserFpKeys = map[string]bool{
	"canvas": true, "webgl": true, "audio": true, "fonts": true,
	"renderer": true, "screen": true,
}

// parseBrowserFp 解析 X-Browser-Fp（base64url JSON）为分量 map；非法输入返回空。
func parseBrowserFp(raw string) map[string]string {
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		if b, err = base64.URLEncoding.DecodeString(raw); err != nil {
			return out
		}
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return out
	}
	for k, v := range m {
		if !browserFpKeys[k] {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" || len(v) > 160 {
			continue
		}
		out[k] = v
	}
	return out
}
