// Package attest — P4-1 Play Integrity 服务端判定（规格书 §7 P4-1）。
//
// 约束（规格原文）：本项目 APP 走自建 APK 分发，appIntegrity.appRecognitionVerdict
// 通常为 UNRECOGNIZED_VERSION——**这不是拒绝条件**。真正校验：
//   1. requestDetails.requestHash == 服务端下发的 nonce（防重放）；
//   2. timestampMillis 新鲜（< 10 分钟）；
//   3. appIntegrity.certificateSha256Digest ∈ 期望集（APP_EXPECTED_CERT_SHA256）；
//   4. deviceIntegrity.deviceRecognitionVerdict 含 MEETS_DEVICE_INTEGRITY
//      （仅 MEETS_BASIC_INTEGRITY → 降级计半分）。
// 判定解析为纯函数（mock JSON 单测三用例：PLAY_RECOGNIZED / UNRECOGNIZED_VERSION /
// requestHash 不符）；nonce 存储为内存消费型（防重放 + TTL + 绑 gh_id）。
package attest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---------- nonce 存储（内存消费型，规格书 P4-1：HMAC 存内存 map，TTL 10min，绑 gh_id） ----------

type nonceEntry struct {
	expires time.Time
	ghID    string
}

// NonceStore 挑战 nonce 存储。键存 SHA-256（不落原始 nonce），消费即失效（防重放）。
type NonceStore struct {
	mu sync.Mutex
	m  map[string]nonceEntry
}

func NewNonceStore() *NonceStore { return &NonceStore{m: map[string]nonceEntry{}} }

// Issue 签发 32 字节随机 nonce（hex），TTL 内有效，绑定当前 gh_id。
func (s *NonceStore) Issue(now time.Time, ttl time.Duration, ghID string) (string, time.Time) {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	nonce := hex.EncodeToString(buf)
	expiry := now.Add(ttl)
	s.mu.Lock()
	s.sweepLocked(now)
	s.m[nonceHash(nonce)] = nonceEntry{expires: expiry, ghID: ghID}
	s.mu.Unlock()
	return nonce, expiry
}

// Take 消费 nonce：存在、未过期 → 返回绑定的 gh_id 并立即失效；否则拒绝。
func (s *NonceStore) Take(nonce string, now time.Time) (ghID string, ok bool) {
	key := nonceHash(nonce)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	e, exists := s.m[key]
	if !exists {
		return "", false
	}
	delete(s.m, key)
	if now.After(e.expires) {
		return "", false
	}
	return e.ghID, true
}

// Peek 非消费查询（verify 需先校验 gh_id 绑定再消费的场景）。
func (s *NonceStore) Peek(nonce string, now time.Time) (ghID string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, exists := s.m[nonceHash(nonce)]
	if !exists || now.After(e.expires) {
		return "", false
	}
	return e.ghID, true
}

func (s *NonceStore) sweepLocked(now time.Time) {
	for k, e := range s.m {
		if now.After(e.expires) {
			delete(s.m, k)
		}
	}
}

func nonceHash(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

// ---------- Google 判定响应解析与评估（纯函数，mock JSON 单测） ----------

// googleVerdict decodeIntegrityToken 响应（tokenPayloadExternal 包裹）。
type googleVerdict struct {
	TokenPayloadExternal struct {
		RequestDetails struct {
			RequestHash     string `json:"requestHash"`
			TimestampMillis int64  `json:"timestampMillis"`
		} `json:"requestDetails"`
		AppIntegrity struct {
			AppRecognitionVerdict   string   `json:"appRecognitionVerdict"`
			CertificateSha256Digest []string `json:"certificateSha256Digest"`
			PackageName             string   `json:"packageName"`
		} `json:"appIntegrity"`
		DeviceIntegrity struct {
			DeviceRecognitionVerdict []string `json:"deviceRecognitionVerdict"`
		} `json:"deviceIntegrity"`
	} `json:"tokenPayloadExternal"`
}

// Result 评估结果。
type Result struct {
	Valid          bool   // 是否通过（Level=play_integrity 时）
	Level          string // play_integrity | signature_fallback
	Reason         string // 失败原因 / 通过摘要
	AppRecognized  bool   // PLAY_RECOGNIZED（自分发通常 UNRECOGNIZED_VERSION，不影响）
	BasicIntegrity bool   // 仅 MEETS_BASIC_INTEGRITY（降级计半分）
	FullIntegrity  bool   // 含 MEETS_DEVICE_INTEGRITY
	CertOK         bool
}

// EvaluatePlayIntegrity 解析并评估 Google 响应（spec 三用例 + 证书/设备档位）。
func EvaluatePlayIntegrity(responseJSON []byte, expectNonce string, now time.Time,
	expectedCerts []string, expectPackage string) (*Result, error) {
	var v googleVerdict
	if err := json.Unmarshal(responseJSON, &v); err != nil {
		return nil, fmt.Errorf("Google 响应解析失败: %w", err)
	}
	rd := v.TokenPayloadExternal.RequestDetails
	ai := v.TokenPayloadExternal.AppIntegrity
	di := v.TokenPayloadExternal.DeviceIntegrity

	// ① 防重放：requestHash 必须等于服务端 nonce
	if !strings.EqualFold(rd.RequestHash, expectNonce) {
		return nil, fmt.Errorf("requestHash 不符（期望 nonce 绑定）")
	}
	// ② 新鲜度 < 10 分钟
	ts := time.UnixMilli(rd.TimestampMillis)
	if now.Sub(ts) > 10*time.Minute || ts.After(now.Add(time.Minute)) {
		return nil, fmt.Errorf("integrity token 过期或时间异常")
	}
	res := &Result{Level: "play_integrity", AppRecognized: ai.AppRecognitionVerdict == "PLAY_RECOGNIZED"}

	// ③ 证书指纹 ∈ 期望集（APP_EXPECTED_CERT_SHA256）
	certSet := map[string]bool{}
	for _, c := range expectedCerts {
		certSet[strings.ToLower(strings.TrimSpace(c))] = true
	}
	for _, c := range ai.CertificateSha256Digest {
		if certSet[strings.ToLower(strings.TrimSpace(c))] {
			res.CertOK = true
			break
		}
	}
	if len(expectedCerts) > 0 && !res.CertOK {
		res.Reason = "签名证书不在期望集"
		return res, nil
	}
	// 包名核对（配置了才核对）
	if expectPackage != "" && ai.PackageName != expectPackage {
		res.Reason = fmt.Sprintf("包名不符 %s", ai.PackageName)
		return res, nil
	}
	// ④ 设备档位
	for _, d := range di.DeviceRecognitionVerdict {
		switch d {
		case "MEETS_DEVICE_INTEGRITY":
			res.FullIntegrity = true
		case "MEETS_BASIC_INTEGRITY":
			res.BasicIntegrity = true
		}
	}
	if !res.FullIntegrity && !res.BasicIntegrity {
		res.Reason = "设备完整性判定缺失"
		return res, nil
	}
	res.Valid = true
	res.Reason = "play_integrity 通过"
	return res, nil
}

// EvaluateSignatureFallback 降级路径（无 GMS / 未配置 Play Console）：APP 直接上报
// 签名证书 SHA-256 + ThreatDetect 结果 → 比对期望集。银标准（可被 hook，成本高于无防护）。
func EvaluateSignatureFallback(certSHA256 string, threat map[string]any, expectedCerts []string) *Result {
	res := &Result{Level: "signature_fallback"}
	cert := strings.ToLower(strings.TrimSpace(certSHA256))
	for _, c := range expectedCerts {
		if strings.ToLower(strings.TrimSpace(c)) == cert && cert != "" {
			res.CertOK = true
			break
		}
	}
	// threat 里任何明确的真阳性标记都视为未通过（root/emulator/hook）
	for k, v := range threat {
		if b, ok := v.(bool); ok && b {
			res.Reason = fmt.Sprintf("威胁标记 %s", k)
			return res
		}
	}
	if !res.CertOK {
		res.Reason = "签名证书不在期望集"
		return res
	}
	res.Valid = true
	res.Reason = "signature_fallback 通过"
	return res
}
