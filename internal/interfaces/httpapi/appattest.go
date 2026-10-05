// P4-1 平台证明端点（规格书 §7 P4-1）：
//   GET  /api/v1/app/attest/challenge → {nonce, expires_at}（32B hex，TTL 10min，绑 gh_id）
//   POST /api/v1/app/attest/verify    → {token}（Play Integrity）或 {nonce, cert_sha256, threat}
//                                       → {level, verdict, attest_required}
// 结果 JSON 存 ip_fingerprints.attestation（绑定 X-Device-Fp 指纹）。
// ATTEST_REQUIRED=1 时无效设备进入只读降级模式（响应携带标志，APP 侧行为契约）。
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/attest"
	"github.com/NoraStory/GithubHot/internal/infrastructure/playintegrity"
)

// AttestNonces 平台证明 nonce 存储（cli 装配注入）。
type AttestNonces interface {
	Issue(now time.Time, ttl time.Duration, ghID string) (string, time.Time)
	Peek(nonce string, now time.Time) (string, bool)
	Take(nonce string, now time.Time) (string, bool)
}

func attestExpectedCerts() []string {
	out := []string{}
	for _, c := range strings.Split(os.Getenv("APP_EXPECTED_CERT_SHA256"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

func playIntegrityConfigured() bool {
	return os.Getenv("PLAY_INTEGRITY_PACKAGE") != "" && os.Getenv("PLAY_INTEGRITY_SA_JSON") != ""
}

func attestRequired() bool { return os.Getenv("ATTEST_REQUIRED") == "1" }

// appAttestChallengeAPI GET /api/v1/app/attest/challenge。
func (s *Server) appAttestChallengeAPI(w http.ResponseWriter, r *http.Request) {
	if s.AttestNonces == nil {
		writeErr(w, 503, errorString("平台证明未启用"))
		return
	}
	nonce, expiry := s.AttestNonces.Issue(time.Now(), 10*time.Minute, ghIDFromRequest(r))
	writeJSON(w, 200, map[string]any{
		"nonce":      nonce,
		"expires_at": expiry.UTC().Format(time.RFC3339),
	})
}

// appAttestVerifyAPI POST /api/v1/app/attest/verify。
func (s *Server) appAttestVerifyAPI(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Nonce     string         `json:"nonce"`
		Token     string         `json:"token"`       // Play Integrity verdict token（主路径）
		CertSHA   string         `json:"cert_sha256"` // 降级路径：签名证书指纹
		Threat    map[string]any `json:"threat"`      // 降级路径：ThreatDetect 结果
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		writeErr(w, 400, errorString("请求体非法"))
		return
	}
	if s.AttestNonces == nil {
		writeErr(w, 503, errorString("平台证明未启用"))
		return
	}
	ip := clientIPFromRequest(r)
	ghNow := ghIDFromRequest(r)
	// nonce 绑定：存在、未过期、gh_id 与签发时一致 → 消费（防重放）
	storedGh, ok := s.AttestNonces.Peek(p.Nonce, time.Now())
	if !ok {
		writeErr(w, 400, errorString("nonce 无效或已过期"))
		return
	}
	if storedGh != "" && ghNow != storedGh {
		writeErr(w, 403, errorString("nonce 与当前会话不符"))
		return
	}
	s.AttestNonces.Take(p.Nonce, time.Now())

	expectedCerts := attestExpectedCerts()
	pkg := os.Getenv("PLAY_INTEGRITY_PACKAGE")
	var result *attest.Result
	var rawVerdict any
	switch {
	case p.Token != "" && playIntegrityConfigured():
		sa, err := playintegrity.LoadSA(os.Getenv("PLAY_INTEGRITY_SA_JSON"))
		if err != nil {
			writeErr(w, 500, errorString(err.Error()))
			return
		}
		client := playintegrity.NewClient(sa, pkg)
		respBody, err := client.DecodeIntegrityToken(r.Context(), p.Token, time.Now())
		if err != nil {
			// Google 调用失败不等于设备不合格：按降级路径处理（若带 cert_sha256）
			if p.CertSHA != "" {
				result = attest.EvaluateSignatureFallback(p.CertSHA, p.Threat, expectedCerts)
				break
			}
			writeErr(w, 502, errorString(err.Error()))
			return
		}
		result, err = attest.EvaluatePlayIntegrity(respBody, p.Nonce, time.Now(), expectedCerts, pkg)
		if err != nil {
			if s.Guard != nil {
				s.Guard.Event(r.Context(), ip, "attest-failed", err.Error(), 10, false)
			}
			writeErr(w, 400, errorString(err.Error()))
			return
		}
		_ = json.Unmarshal(respBody, &rawVerdict)
	case p.CertSHA != "":
		result = attest.EvaluateSignatureFallback(p.CertSHA, p.Threat, expectedCerts)
	default:
		writeErr(w, 400, errorString("需要 token（Play Integrity）或 cert_sha256（降级路径）"))
		return
	}

	// 结果写回指纹档案（attestation 列）；失败记弱证据（可与其余证据互证）
	fp := strings.TrimSpace(r.Header.Get(fpHeaderName))
	if s.Guard != nil && fp != "" {
		verdictJSON, _ := json.Marshal(map[string]any{
			"level": result.Level, "valid": result.Valid, "reason": result.Reason,
			"full_integrity": result.FullIntegrity, "basic_integrity": result.BasicIntegrity,
			"cert_ok": result.CertOK, "at": time.Now().UTC().Format(time.RFC3339),
		})
		_ = s.Guard.Store().UpdateAttestation(r.Context(), fp, string(verdictJSON))
	}
	if !result.Valid && s.Guard != nil {
		s.Guard.Event(r.Context(), ip, "attest-failed", result.Reason, 10, false)
	}
	writeJSON(w, 200, map[string]any{
		"level":           result.Level,
		"valid":           result.Valid,
		"reason":          result.Reason,
		"attest_required": attestRequired(),
		"verdict":         rawVerdict,
	})
}
