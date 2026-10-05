package attest

import (
	"strings"
	"testing"
	"time"
)

var now = time.Unix(1791300000, 0)
var certs = []string{"AB:CD:EF:01", "DE:AD:BE:EF"}

// mockVerdict 构造 Google decodeIntegrityToken 响应（tokenPayloadExternal 包裹）。
func mockVerdict(requestHash string, tsMillis int64, verdict string, certList []string, device []string) []byte {
	certs := "[]"
	if len(certList) > 0 {
		quoted := make([]string, len(certList))
		for i, c := range certList {
			quoted[i] = `"` + c + `"`
		}
		certs = "[" + strings.Join(quoted, ",") + "]"
	}
	dev := "[]"
	if len(device) > 0 {
		quoted := make([]string, len(device))
		for i, d := range device {
			quoted[i] = `"` + d + `"`
		}
		dev = "[" + strings.Join(quoted, ",") + "]"
	}
	return []byte(`{"tokenPayloadExternal":{
		"requestDetails":{"requestHash":"` + requestHash + `","timestampMillis":` +
		intStr(tsMillis) + `},
		"appIntegrity":{"appRecognitionVerdict":"` + verdict + `","certificateSha256Digest":` + certs +
		`,"packageName":"com.norastory.githubhot"},
		"deviceIntegrity":{"deviceRecognitionVerdict":` + dev + `}}}`)
}

func intStr(n int64) string { return strings.TrimSpace(strings.Repeat("", 0) + itoa(n)) }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

const nonce = "aa11bb22cc33dd44ee55ff66"

// 用例 1（规格）：PLAY_RECOGNIZED + 证书在期望集 + MEETS_DEVICE_INTEGRITY → 通过（满档）。
func TestEvaluatePlayRecognized(t *testing.T) {
	ts := now.Add(-time.Minute).UnixMilli()
	res, err := EvaluatePlayIntegrity(mockVerdict(nonce, ts, "PLAY_RECOGNIZED", certs, []string{"MEETS_DEVICE_INTEGRITY"}),
		nonce, now, certs, "com.norastory.githubhot")
	if err != nil {
		t.Fatalf("应通过: %v", err)
	}
	if !res.Valid || res.Level != "play_integrity" || !res.FullIntegrity || !res.CertOK || !res.AppRecognized {
		t.Fatalf("结果不符: %+v", res)
	}
}

// 用例 2（规格）：UNRECOGNIZED_VERSION（自分发预期）→ 不是拒绝条件，仍通过。
func TestEvaluateUnrecognizedVersion(t *testing.T) {
	ts := now.Add(-time.Minute).UnixMilli()
	res, err := EvaluatePlayIntegrity(mockVerdict(nonce, ts, "UNRECOGNIZED_VERSION", certs, []string{"MEETS_DEVICE_INTEGRITY"}),
		nonce, now, certs, "com.norastory.githubhot")
	if err != nil {
		t.Fatalf("UNRECOGNIZED_VERSION 不应拒绝: %v", err)
	}
	if !res.Valid || res.AppRecognized {
		t.Fatalf("自分发应通过且 AppRecognized=false: %+v", res)
	}
	// 仅 BASIC_INTEGRITY → 降级半分（仍 Valid，标记 BasicIntegrity）
	res2, err := EvaluatePlayIntegrity(mockVerdict(nonce, ts, "UNRECOGNIZED_VERSION", certs, []string{"MEETS_BASIC_INTEGRITY"}),
		nonce, now, certs, "")
	if err != nil || !res2.Valid || !res2.BasicIntegrity || res2.FullIntegrity {
		t.Fatalf("BASIC_INTEGRITY 应降级通过: %+v err=%v", res2, err)
	}
	// 证书不在期望集 → 拒
	res3, err := EvaluatePlayIntegrity(mockVerdict(nonce, ts, "UNRECOGNIZED_VERSION", []string{"XX:XX"}, []string{"MEETS_DEVICE_INTEGRITY"}),
		nonce, now, certs, "")
	if err != nil || res3.Valid || !strings.Contains(res3.Reason, "证书") {
		t.Fatalf("证书不符应拒绝: %+v err=%v", res3, err)
	}
	// 设备档位缺失 → 拒
	res4, _ := EvaluatePlayIntegrity(mockVerdict(nonce, ts, "UNRECOGNIZED_VERSION", certs, nil), nonce, now, certs, "")
	if res4.Valid || !strings.Contains(res4.Reason, "设备完整性") {
		t.Fatalf("设备档位缺失应拒绝: %+v", res4)
	}
}

// 用例 3（规格）：requestHash 不符（防重放）→ 错误。
func TestEvaluateRequestHashMismatch(t *testing.T) {
	ts := now.Add(-time.Minute).UnixMilli()
	_, err := EvaluatePlayIntegrity(mockVerdict("deadbeef", ts, "PLAY_RECOGNIZED", certs, []string{"MEETS_DEVICE_INTEGRITY"}),
		nonce, now, certs, "")
	if err == nil || !strings.Contains(err.Error(), "requestHash 不符") {
		t.Fatalf("requestHash 不符应报错: %v", err)
	}
	// 时间过期（>10min）→ 错误
	_, err = EvaluatePlayIntegrity(mockVerdict(nonce, now.Add(-11*time.Minute).UnixMilli(), "PLAY_RECOGNIZED", certs, []string{"MEETS_DEVICE_INTEGRITY"}),
		nonce, now, certs, "")
	if err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("时间过期应报错: %v", err)
	}
}

// TestNonceStore 签发/消费/重放拒绝/TTL 过期/gh_id 绑定。
func TestNonceStore(t *testing.T) {
	s := NewNonceStore()
	nonce, expiry := s.Issue(now, 10*time.Minute, "gh-abc")
	if expiry.Before(now.Add(9 * time.Minute)) {
		t.Fatalf("TTL 不符")
	}
	// 未消费可 Peek
	if gh, ok := s.Peek(nonce, now); !ok || gh != "gh-abc" {
		t.Fatalf("Peek 应命中: %q %v", gh, ok)
	}
	// 消费成功；二次消费（重放）拒绝
	if gh, ok := s.Take(nonce, now.Add(time.Second)); !ok || gh != "gh-abc" {
		t.Fatalf("消费应命中")
	}
	if _, ok := s.Take(nonce, now.Add(time.Second)); ok {
		t.Fatalf("重放应拒绝")
	}
	// TTL 过期
	n2, _ := s.Issue(now, time.Second, "gh-x")
	if _, ok := s.Take(n2, now.Add(2*time.Second)); ok {
		t.Fatalf("过期 nonce 应拒绝")
	}
	// 随机性：两次签发不同
	n3, _ := s.Issue(now, time.Minute, "")
	if n3 == nonce {
		t.Fatalf("nonce 应随机")
	}
}

// TestSignatureFallback 降级路径：证书在集 + 无威胁 → 通过；威胁标记 → 拒。
func TestSignatureFallback(t *testing.T) {
	res := EvaluateSignatureFallback("ab:cd:ef:01", map[string]any{"rooted": false}, certs)
	if !res.Valid || res.Level != "signature_fallback" || !res.CertOK {
		t.Fatalf("降级路径应通过: %+v", res)
	}
	res2 := EvaluateSignatureFallback("ab:cd:ef:01", map[string]any{"emulator": true}, certs)
	if res2.Valid || !strings.Contains(res2.Reason, "emulator") {
		t.Fatalf("威胁标记应拒绝: %+v", res2)
	}
	res3 := EvaluateSignatureFallback("bad-cert", nil, certs)
	if res3.Valid {
		t.Fatalf("证书不符应拒绝: %+v", res3)
	}
}
