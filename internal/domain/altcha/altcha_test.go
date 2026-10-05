package altcha

import (
	"strings"
	"testing"
	"time"
)

const testSecret = "test-altcha-secret"

// TestIssueChallengeShape 挑战结构：可解码、四段、HMAC 与参数一致、签名绑定 fp。
func TestIssueChallengeShape(t *testing.T) {
	now := time.Unix(1791200000, 0)
	ch := Issue(Params{Secret: testSecret, Difficulty: 12, MaxAge: 10 * time.Minute, Now: now,
		Fp: "fp1", GhID: "gh1"})
	if ch.Algorithm != "SHA-256" || ch.Difficulty != 12 || ch.MaxAgeSec != 600 || ch.Signature == "" {
		t.Fatalf("挑战参数不符: %+v", ch)
	}
	nonce, err := Solve(ch.Challenge, 12)
	if err != nil {
		t.Fatalf("求解失败: %v", err)
	}
	if err := Verify(testSecret, now.Add(time.Minute), Solution{
		Challenge: ch.Challenge, Nonce: nonce, Signature: ch.Signature,
	}, "fp1", "gh1"); err != nil {
		t.Fatalf("合法解应通过: %v", err)
	}
	// 同一解换指纹 → 签名绑定应拒绝
	if err := Verify(testSecret, now.Add(time.Minute), Solution{
		Challenge: ch.Challenge, Nonce: nonce, Signature: ch.Signature,
	}, "fp-other", "gh1"); err == nil || !strings.Contains(err.Error(), "不匹配") {
		t.Fatalf("跨指纹重放应拒绝: %v", err)
	}
}

// TestSolveDifficulty 求解产出的 nonce 确实满足前导零比特要求。
func TestSolveDifficulty(t *testing.T) {
	ch := "demo-challenge-string"
	for _, d := range []int{4, 8, 12} {
		nonce, err := Solve(ch, d)
		if err != nil {
			t.Fatalf("difficulty %d 求解失败: %v", d, err)
		}
		if bits := leadingZeroBits(sha256Sum(ch + nonce)); bits < d {
			t.Fatalf("difficulty %d 的解仅 %d 比特前导零", d, bits)
		}
	}
}

// TestVerifyFailures 逐项失败路径：过期 / 篡改 HMAC / PoW 不足 / 指纹不匹配 / 缺字段。
func TestVerifyFailures(t *testing.T) {
	now := time.Unix(1791200000, 0)
	ch := Issue(Params{Secret: testSecret, Difficulty: 8, MaxAge: time.Minute, Now: now,
		Fp: "fp1", GhID: "gh1"})
	nonce, _ := Solve(ch.Challenge, 8)
	ok := Solution{Challenge: ch.Challenge, Nonce: nonce, Signature: ch.Signature}

	// 正解通过
	if err := Verify(testSecret, now.Add(30*time.Second), ok, "fp1", "gh1"); err != nil {
		t.Fatalf("正解应通过: %v", err)
	}
	// 过期
	if err := Verify(testSecret, now.Add(2*time.Minute), ok, "fp1", "gh1"); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("过期应拒绝: %v", err)
	}
	// 换密钥（HMAC 不符）
	if err := Verify("other-secret", now, ok, "fp1", "gh1"); err == nil || !strings.Contains(err.Error(), "HMAC") {
		t.Fatalf("密钥不符应拒绝: %v", err)
	}
	// PoW 不足：直接篡改 nonce 使零比特不足
	bad := Solution{Challenge: ch.Challenge, Nonce: "999999999", Signature: ch.Signature}
	if err := Verify(testSecret, now, bad, "fp1", "gh1"); err == nil {
		t.Fatalf("PoW 不足应拒绝")
	}
	// 跨指纹重放
	if err := Verify(testSecret, now, ok, "fp2", "gh1"); err == nil || !strings.Contains(err.Error(), "不匹配") {
		t.Fatalf("跨指纹重放应拒绝: %v", err)
	}
	// 缺字段
	if err := Verify(testSecret, now, Solution{Challenge: ch.Challenge}, "fp1", "gh1"); err == nil {
		t.Fatalf("缺字段应拒绝")
	}
	// 伪造签名（客户端无密钥不可自造）
	forge := Solution{Challenge: ch.Challenge, Nonce: nonce, Signature: hmacSHA256("attacker", "x")}
	if err := Verify(testSecret, now, forge, "fp1", "gh1"); err == nil {
		t.Fatalf("伪造签名应拒绝")
	}
}

// TestLeadingZeroBits 计数器。
func TestLeadingZeroBits(t *testing.T) {
	var d [32]byte
	d[0] = 0xff
	if got := leadingZeroBits(d); got != 0 {
		t.Fatalf("0xff 前导零应 0，实际 %d", got)
	}
	d[0] = 0x00
	d[1] = 0x0f
	if got := leadingZeroBits(d); got != 12 {
		t.Fatalf("0x000f 前导零应 12，实际 %d", got)
	}
	var full [32]byte
	if got := leadingZeroBits(full); got != 256 {
		t.Fatalf("全零应 256，实际 %d", got)
	}
}
