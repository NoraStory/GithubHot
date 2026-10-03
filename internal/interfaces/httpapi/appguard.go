package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
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

	appSignWindow    = 300 * time.Second // 时间戳容差 ±5min（设备时钟漂移）
	appNonceTTL      = 10 * time.Minute
	appFpRecordTTL   = time.Minute // 同指纹归档限频
	appMaxBody       = 1 << 20
)

// AppGuard APP 请求防护引擎。
type AppGuard struct {
	store  GuardStore
	guard  *IPGuard // 违规事件/封禁复用三层 IP 防护体系
	seed   string
	banned bool
	force  string

	mu        sync.Mutex
	nonces    map[string]time.Time
	fpLast    map[string]time.Time
}

// NewAppGuard 构建；seed 缺省与 siteConfig 下发保持一致（gh-dev-seed-v1）。
func NewAppGuard(store GuardStore, guard *IPGuard) *AppGuard {
	return &AppGuard{
		store:  store,
		guard:  guard,
		seed:   appSessionSeed(),
		banned: osBanned(),
		force:  strings.TrimSpace(os.Getenv("APP_FORCE_UPGRADE_URL")),
		nonces: map[string]time.Time{},
		fpLast: map[string]time.Time{},
	}
}

func osBanned() bool {
	return os.Getenv("APP_BANNED") == "1" || strings.EqualFold(os.Getenv("APP_BANNED"), "true")
}

// deriveSessionKey 与客户端 SessionManager 完全一致的密钥派生。
func (a *AppGuard) deriveSessionKey(fp string) []byte {
	secret := []byte(a.seed + ":" + fp)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("gh-session-v1"))
	return mac.Sum(nil)
}

// expectedToken 与客户端 SessionManager 一致的会话令牌。
func (a *AppGuard) expectedToken(fp string) string {
	key := a.deriveSessionKey(fp)
	h := sha256.New()
	h.Write(key)
	h.Write([]byte(fp))
	return hex.EncodeToString(h.Sum(nil))[:40]
}

// expectedSign 重算请求签名（path 含 query，body 为原始字节）。
func (a *AppGuard) expectedSign(fp, ts, nonce, method, path string, body []byte) string {
	bh := sha256.Sum256(body)
	payload := ts + "\n" + nonce + "\n" + strings.ToUpper(method) + "\n" + path + "\n" + hex.EncodeToString(bh[:])
	mac := hmac.New(sha256.New, a.deriveSessionKey(fp))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
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
				a.recordFingerprint(ctx, ip, fp, r.Header.Get(appModelHeader), r.Header.Get(appThreatHeader))
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
		want := a.expectedSign(fp, ts, nonce, r.Method, path, body)
		if !hmac.Equal([]byte(strings.ToLower(want)), []byte(strings.ToLower(sig))) {
			a.fail(ctx, ip, "app-sign-invalid", "接口签名无效", 60)
			writeJSON(w, 401, map[string]any{"error": "bad sign"})
			return
		}
		if tok := r.Header.Get(appTokenHeader); tok != "" && tok != a.expectedToken(fp) {
			a.fail(ctx, ip, "app-token-mismatch", "会话令牌与指纹绑定不符", 40)
			writeJSON(w, 401, map[string]any{"error": "bad token"})
			return
		}

		// 校验通过：指纹/型号/威胁报告归档（限频 1 次/分/指纹），
		// 管理端 /admin/ipguard 单 IP 下钻可见 APP 设备档案。
		a.recordFingerprint(ctx, ip, fp, r.Header.Get(appModelHeader), r.Header.Get(appThreatHeader))
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

// recordFingerprint APP 设备指纹归档进 IP 防护体系（带型号与威胁命中）。
func (a *AppGuard) recordFingerprint(ctx context.Context, ip, fp, model, threat string) {
	if a.store == nil {
		return
	}
	a.mu.Lock()
	last := a.fpLast[fp+"|"+ip]
	if time.Since(last) < appFpRecordTTL {
		a.mu.Unlock()
		return
	}
	a.fpLast[fp+"|"+ip] = time.Now()
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
	if _, err := a.store.UpsertFingerprint(ctx, fp, ip, ua, FingerprintMeta{Components: components, Flags: flags}); err != nil {
		log.Printf("[appguard] 指纹归档失败 %s: %v", fp, err)
	}
}
