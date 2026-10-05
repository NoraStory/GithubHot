// ALTCHA PoW（P4-3）：挑战签发 / 校验，与 fp/report 的执行挂钩。
//
// 纪律（规格书 P4-3 + 灰度实践）：ALTCHA_DIFFICULTY=0（默认）时仅提供挑战端点、
// fp/report 不强制——老客户端带缓存页面访问不受影响；部署方观察前端就位后置 12+
// 启用强制。ALTCHA_SECRET 未配置 → 整体降级（挑战 503，执行关闭）。
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// altchaChallengeAPI GET /api/v1/altcha/challenge?fp=...：签发一枚 PoW 挑战。
// 挑战与指纹在**签发时绑定**（服务端计算 signature，客户端无密钥不可自造），
// 解好的 nonce 无法换一个指纹重放。
func (s *Server) altchaChallengeAPI(w http.ResponseWriter, r *http.Request) {
	secret := altchaSecret()
	if secret == "" {
		writeErr(w, 503, errorString("ALTCHA 未配置（ALTCHA_SECRET 缺失）"))
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
		writeErr(w, 503, errorString("ALTCHA 未配置"))
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
	writeJSON(w, 200, map[string]any{"ok": true})
}

// checkAltchaForReport fp/report 的 PoW 执行入口：强制开启时校验失败 → 401 + 弱证据计分。
// 返回 false = 已写 401 响应，调用方应立即返回。
func (s *Server) checkAltchaForReport(w http.ResponseWriter, r *http.Request, ip, fp string, sol *altchaSolution) bool {
	secret := altchaSecret()
	if secret == "" || altchaDifficulty() <= 0 {
		return true // 未启用：行为与 PoW 之前完全一致
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
		writeErr(w, 401, errorString("需要有效的 ALTCHA 工作量证明："+err.Error()))
		return false
	}
	return true
}

// ghIDFromRequest 读 gh_id cookie 值（ALTCHA 签名绑定的第二因子；未登录为空串）。
func ghIDFromRequest(r *http.Request) string {
	if c, err := r.Cookie(idCookieName); err == nil {
		return c.Value
	}
	return ""
}
