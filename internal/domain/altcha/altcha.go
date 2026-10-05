// Package altcha — P4-3 ALTCHA 工作量证明（规格书 §7 P4-3，自研实现，无外部依赖）。
//
// 算法（规格原文）：
//   challenge = base64url("maxAge:difficulty:salt:hmac")，
//   其中 hmac = HMAC-SHA256(ALTCHA_SECRET, salt+maxAge) —— 服务端签发后无需存态；
//   客户端暴力寻找 nonce 使 SHA-256(challenge + nonce) 的前导零比特 ≥ difficulty；
//   提交 (challenge, nonce, signature)，signature = HMAC-SHA256(ALTCHA_SECRET, fp+"|"+ghID)
//   绑定指纹，防跨指纹重放。
// 服务端校验全部本地可完成：解码 → 验 HMAC（防伪造/改难度）→ 查 maxAge（防重放）
// → 重算 PoW（防跳过）→ 验 signature（防换皮）。
//
// 全部为纯函数；时间由调用方注入（now），便于测试。
package altcha

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Params 一次挑战的签发参数。
type Params struct {
	Secret     string
	Difficulty int           // 前导零比特数（12 ≈ 平均 4096 次尝试，毫秒级）
	MaxAge     time.Duration // 挑战有效期
	Now        time.Time
	Salt       string // 可选固定盐（测试用）；空则随机生成
	Fp         string // 签发时绑定的设备指纹（防跨指纹重放）
	GhID       string // 签发时绑定的身份令牌（可选；首访为空）
}

// Challenge 签发结果。
type Challenge struct {
	Algorithm  string `json:"algorithm"` // 固定 "SHA-256"
	Challenge  string `json:"challenge"` // base64url(expiry:difficulty:salt:hmac)
	MaxAgeSec  int    `json:"maxage"`
	Difficulty int    `json:"difficulty"`
	Signature  string `json:"signature"` // 服务端签发时绑定 fp+gh_id，客户端原样回传
}

// Solution 客户端提交的解。
type Solution struct {
	Challenge string
	Nonce     string
	Signature string
}

// Issue 签发一枚挑战。payload 内的时间为**绝对过期时间戳**（epoch 秒）——
// 纯时长没有校验锚点，绝对时间才能让服务端无状态地查"是否过期"。
func Issue(p Params) Challenge {
	salt := p.Salt
	if salt == "" {
		buf := make([]byte, 12)
		_, _ = rand.Read(buf)
		salt = base64.RawURLEncoding.EncodeToString(buf)
	}
	expiry := p.Now.Add(p.MaxAge).Unix()
	mac := hmacSHA256(p.Secret, salt+strconv.FormatInt(expiry, 10))
	payload := fmt.Sprintf("%d:%d:%s:%s", expiry, p.Difficulty, salt, mac)
	challenge := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return Challenge{
		Algorithm:  "SHA-256",
		Challenge:  challenge,
		MaxAgeSec:  int(p.MaxAge / time.Second),
		Difficulty: p.Difficulty,
		Signature:  signature(p.Secret, challenge, p.Fp, p.GhID),
	}
}

// signature 挑战↔指纹绑定：由**服务端在签发时**计算（客户端无密钥、不可自造），
// 校验时与请求声明的 fp 比对——一份解好的 nonce 无法换一个指纹重放。
// （规格书 P4-3 的 "signature=HMAC(fp+gh_id)" 以服务端签发实现：客户端计算无密钥可言。）
func signature(secret, challenge, fp, ghID string) string {
	return hmacSHA256(secret, challenge+"|"+fp+"|"+ghID)
}

// Verify 校验提交的解。错误均返回描述性消息（不计分决策由接口层做）。
func Verify(secret string, now time.Time, s Solution, fp, ghID string) error {
	if s.Challenge == "" || s.Nonce == "" || s.Signature == "" {
		return fmt.Errorf("缺少 challenge/nonce/signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(s.Challenge)
	if err != nil {
		return fmt.Errorf("challenge 解码失败")
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 4 {
		return fmt.Errorf("challenge 格式非法")
	}
	maxAgeSec, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("challenge maxAge 非法")
	}
	difficulty, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("challenge difficulty 非法")
	}
	salt, mac := parts[2], parts[3]
	// 1) HMAC 防伪造/篡改（过期时间与难度都封在签名里）
	if !hmacEqual(hmacSHA256(secret, salt+strconv.Itoa(maxAgeSec)), mac) {
		return fmt.Errorf("challenge HMAC 校验失败")
	}
	// 2) 绝对过期时间戳防重放
	expiry := time.Unix(int64(maxAgeSec), 0)
	if now.After(expiry) {
		return fmt.Errorf("challenge 已过期")
	}
	if difficulty < 0 || difficulty > 64 {
		return fmt.Errorf("difficulty 越界")
	}
	// 3) 重算工作量证明
	if leadingZeroBits(sha256Sum(s.Challenge+s.Nonce)) < difficulty {
		return fmt.Errorf("工作量证明不足（需 %d 前导零比特）", difficulty)
	}
	// 4) 指纹绑定防跨指纹重放（签名由服务端签发时计算，校验时与请求声明的 fp 比对）
	if !hmacEqual(signature(secret, s.Challenge, fp, ghID), s.Signature) {
		return fmt.Errorf("signature 与指纹不匹配")
	}
	return nil
}

// Solve 客户端求解（前端 JS 镜像实现，供测试与 APP 参考）：返回满足难度的最小 nonce。
// 暴力从 0 开始递增，SHA-256(challenge+nonce) 前导零比特 ≥ difficulty 即命中。
func Solve(challenge string, difficulty int) (string, error) {
	if difficulty <= 0 {
		return "", nil
	}
	for n := uint64(0); ; n++ {
		dec := strconv.FormatUint(n, 10)
		if leadingZeroBits(sha256Sum(challenge+dec)) >= difficulty {
			return dec, nil
		}
	}
}

// LeadingZeroBits 供接口层/测试复用的前导零比特计数。
func LeadingZeroBits(digest [32]byte) int { return leadingZeroBits(digest) }

func leadingZeroBits(digest [32]byte) int {
	bits := 0
	for _, b := range digest {
		if b == 0 {
			bits += 8
			continue
		}
		for m := byte(0x80); m != 0 && b&m == 0; m >>= 1 {
			bits++
		}
		break
	}
	return bits
}

func sha256Sum(s string) [32]byte { return sha256.Sum256([]byte(s)) }

func hmacSHA256(secret, msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func hmacEqual(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }
