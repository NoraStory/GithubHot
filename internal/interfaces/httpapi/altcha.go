// ALTCHA PoW（P4-3）：挑战签发 / 校验，与 fp/report 的执行挂钩。
//
// 纪律（规格书 P4-3 + 灰度实践）：ALTCHA_DIFFICULTY=0（默认）时仅提供挑战端点、
// fp/report 不强制——老客户端带缓存页面访问不受影响；部署方观察前端就位后置 12+
// 启用强制。ALTCHA_SECRET 未配置 → 整体降级（挑战 503，执行关闭）。
//
// 渗透测试修复（M-2/M-3）：
//   - 对外错误文案统一「服务暂不可用」，配置细节只进服务端日志（防未认证方探测部署状态）
//   - 已解挑战消费账本：同一 (challenge,nonce) 只能提交一次（10 分钟有效期内不可重放，
//     修 M-3 批量化上报突破口）；单实例内存实现，多副本部署时换共享存储（cache 包）
//   - APP 兼容：带 X-App-Sign 的请求跳过强制——AppGuard 中间件已在上游验签
//     （无效签名 403 到不了这里），存量 APP 不被误伤，非 APP 流量照常强制
package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/altcha"
)

// altchaSolution 客户端提交的解（fp/report 的可选 altcha 字段；APP 同算法）。
type altchaSolution struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

func altchaSecret() string    { return strings.TrimSpace(os.Getenv("ALTCHA_SECRET")) }
func altchaDifficulty() int   { v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("ALTCHA_DIFFICULTY"))); return v }
func altchaMaxAge() time.Duration {
	if v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("ALTCHA_MAXAGE_SEC"))); v > 0 {
		return time.Duration(v) * time.Second
	}
	return 10 * time.Minute
}

// altchaEnforced 是否强制 fp/report 携带有效 PoW（密钥在位且难度 > 0）。
func altchaEnforced() bool { return altchaSecret() != "" && altchaDifficulty() > 0 }

// ---------- 已解挑战消费账本（M-3 防重放） ----------

// altchaUsed 已提交解的去重账本：键 = SHA-256(challenge|nonce)（不落原始值），
// 值 = 消费时间。TTL 取挑战有效期（过期解在 Verify 就被拒，账本条目无意义）。
// 容量守卫：超 65536 条先清过期。多副本部署时换共享存储（cache 包）。
var altchaUsed = struct {
	sync.Mutex
	m map[string]time.Time
}{m: map[string]time.Time{}}

// altchaConsume 原子消费一枚解：返回 false = 已被使用（重放）。
func altchaConsume(challenge, nonce string) bool {
	sum := sha256.Sum256([]byte(challenge + "|" + nonce))
	key := base64.RawURLEncoding.EncodeToString(sum[:])
	now := time.Now()
	altchaUsed.Lock()
	defer altchaUsed.Unlock()
	if t, ok := altchaUsed.m[key]; ok && now.Sub(t) < altchaMaxAge() {
		return false
	}
	if len(altchaUsed.m) >= 65536 {
		for k, t := range altchaUsed.m {
			if now.Sub(t) >= altchaMaxAge() {
				delete(altchaUsed.m, k)
			}
		}
	}
	altchaUsed.m[key] = now
	return true
}

// altchaChallengeAPI GET /api/v1/altcha/challenge?fp=...：签发一枚 PoW 挑战。
// 挑战与指纹在**签发时绑定**（服务端计算 signature，客户端无密钥不可自造），
// 解好的 nonce 无法换一个指纹重放。
func (s *Server) altchaChallengeAPI(w http.ResponseWriter, r *http.Request) {
	secret := altchaSecret()
	if secret == "" {
		// 文案去敏（M-2）：对外不暴露配置状态，细节只进日志
		log.Printf("[altcha] 挑战签发被拒：ALTCHA_SECRET 未配置")
		writeErr(w, 503, errorString("服务暂不可用"))
		return
	}
	fp := strings.TrimSpace(r.URL.Query().Get("fp"))
	if len(fp) < 16 || len(fp) > 128 {
		writeErr(w, 400, errorString("需要合法的 fp 参数（挑战与指纹绑定）"))
		return
	}
	ch := altcha.Issue(altcha.Params{
		Secret: secret, Difficulty: altchaDifficulty(), MaxAge: altchaMaxAge(),
		Now: time.Now(), Fp: fp, GhID: ghIDFromRequest(r),
	})
	writeJSON(w, 200, map[string]any{
		"algorithm":  ch.Algorithm,
		"challenge":  ch.Challenge,
		"maxage":     ch.MaxAgeSec,
		"difficulty": ch.Difficulty,
		"signature":  ch.Signature,
	})
}

// altchaVerifyAPI POST /api/v1/altcha/verify：独立校验通道（APP 契约/联调用）。
// body {challenge, nonce, signature, fp}；signature 绑 fp+gh_id。
func (s *Server) altchaVerifyAPI(w http.ResponseWriter, r *http.Request) {
	secret := altchaSecret()
	if secret == "" {
		log.Printf("[altcha] 独立校验被拒：ALTCHA_SECRET 未配置")
		writeErr(w, 503, errorString("服务暂不可用"))
		return
	}
	var p struct {
		Challenge string `json:"challenge"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
		Fp        string `json:"fp"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&p); err != nil {
		writeErr(w, 400, errorString("请求体非法"))
		return
	}
	ip := clientIPFromRequest(r)
	if err := altcha.Verify(secret, time.Now(), altcha.Solution{
		Challenge: p.Challenge, Nonce: p.Nonce, Signature: p.Signature,
	}, p.Fp, ghIDFromRequest(r)); err != nil {
		if s.Guard != nil {
			s.Guard.Event(r.Context(), ip, "altcha-failed", "ALTCHA 校验失败："+err.Error(), 10, false)
		}
		writeErr(w, 401, errorString(err.Error()))
		return
	}
	if !altchaConsume(p.Challenge, p.Nonce) {
		if s.Guard != nil {
			s.Guard.Event(r.Context(), ip, "altcha-replayed", "同一解重复提交（重放拦截）", 20, false)
		}
		writeErr(w, 401, errorString("解已被使用，请重新领取挑战"))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// checkAltchaForReport fp/report 的 PoW 执行入口：强制开启时校验失败 → 401 + 弱证据计分。
// 返回 false = 已写 401 响应，调用方应立即返回。
// APP 兼容：带 X-App-Sign 的请求跳过——AppGuard 中间件已在上游验签（无效签名 403），
// 存量 APP 无求解器，强制会误伤全部装机；APP 有独立的签名身份层。
func (s *Server) checkAltchaForReport(w http.ResponseWriter, r *http.Request, ip, fp string, sol *altchaSolution) bool {
	secret := altchaSecret()
	if secret == "" || altchaDifficulty() <= 0 {
		return true // 未启用：行为与 PoW 之前完全一致
	}
	if r.Header.Get("X-App-Sign") != "" {
		return true // APP 通道：签名已由 AppGuard 验证
	}
	var v altcha.Solution
	if sol != nil {
		v = altcha.Solution{Challenge: sol.Challenge, Nonce: sol.Nonce, Signature: sol.Signature}
	}
	if err := altcha.Verify(secret, time.Now(), v, fp, ghIDFromRequest(r)); err != nil {
		if s.Guard != nil {
			s.Guard.Event(r.Context(), ip, "altcha-missing",
				"fp/report 缺少有效 ALTCHA PoW："+err.Error(), 10, false)
		}
		writeErr(w, 401, errorString("需要有效的 ALTCHA 工作量证明"))
		return false
	}
	if !altchaConsume(v.Challenge, v.Nonce) {
		if s.Guard != nil {
			s.Guard.Event(r.Context(), ip, "altcha-replayed", "同一解重复提交（重放拦截）", 20, false)
		}
		writeErr(w, 401, errorString("解已被使用，请重新领取挑战"))
		return false
	}
	return true
}

// checkAltchaForLogin 管理端登录的 PoW 执行入口（M-2 纵深）：强制开启时，
// 无有效解直接 401 且**不计入**爆破锁定次数（锁定只数密码错误）。
// fp 与解都来自登录请求体（前端登录页用 deviceFp() 领挑战并求解）。
// 返回 (ok, 拒绝文案)：重放与缺解文案可区分（契约测试断言）。
func (s *Server) checkAltchaForLogin(r *http.Request, fp string, sol *altchaSolution) (bool, string) {
	secret := altchaSecret()
	if secret == "" || altchaDifficulty() <= 0 {
		return true, ""
	}
	var v altcha.Solution
	if sol != nil {
		v = altcha.Solution{Challenge: sol.Challenge, Nonce: sol.Nonce, Signature: sol.Signature}
	}
	if err := altcha.Verify(secret, time.Now(), v, fp, ghIDFromRequest(r)); err != nil {
		return false, "需要有效的 ALTCHA 工作量证明"
	}
	if !altchaConsume(v.Challenge, v.Nonce) {
		return false, "解已被使用，请重新领取挑战"
	}
	return true, ""
}

// ghIDFromRequest 读 gh_id cookie 值（ALTCHA 签名绑定的第二因子；未登录为空串）。
func ghIDFromRequest(r *http.Request) string {
	if c, err := r.Cookie(idCookieName); err == nil {
		return c.Value
	}
	return ""
}
