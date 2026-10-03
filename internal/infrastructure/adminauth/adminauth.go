// Package adminauth 管理端密码凭证：Argon2id（RFC 9106 / OWASP 推荐）哈希与校验。
// 哈希串使用 PHC 标准编码（$argon2id$v=19$m=...,t=...,p=2$盐$哈希），
// 参数随串保存，未来调参不影响已存哈希校验。
package adminauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	version   = argon2.Version
	memory    = 64 * 1024 // 64 MiB，OWASP 推荐档
	iterations = 3
	parallelism = 2
	saltLen   = 16
	keyLen    = 32
)

// HashPassword 生成 Argon2id PHC 编码串（含随机盐，每次结果不同）。
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成盐: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		version, memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// params 解析出的 Argon2id 参数。
type params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	salt        []byte
	key         []byte
}

// parsePHC 解析 $argon2id$v=19$m=65536,t=3,p=2$<salt>$<key> 格式。
// 任何字段不符合预期都视为无效哈希（校验返回 false，不返回具体原因）。
func parsePHC(encoded string) (*params, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, fmt.Errorf("非 argon2id 编码")
	}
	if !strings.HasPrefix(parts[2], "v=") {
		return nil, fmt.Errorf("缺少版本号")
	}
	ver, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || ver != version {
		return nil, fmt.Errorf("版本不支持: %s", parts[2])
	}
	var p params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	if p.memory < 8*1024 || p.memory > 1<<30 || p.iterations == 0 || p.iterations > 64 || p.parallelism == 0 || p.parallelism > 16 {
		return nil, fmt.Errorf("参数越界")
	}
	p.salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(p.salt) < 8 {
		return nil, fmt.Errorf("盐无效")
	}
	p.key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(p.key) < 16 {
		return nil, fmt.Errorf("哈希无效")
	}
	return &p, nil
}

// VerifyPassword 恒定时间校验密码与 PHC 编码哈希是否匹配。
// 哈希串本身无效（格式错/被篡改）时返回 false，不视为错误。
func VerifyPassword(encoded, password string) (bool, error) {
	p, err := parsePHC(encoded)
	if err != nil {
		return false, nil //nolint:nilerr // 无效哈希按不匹配处理，避免泄露哈希状态
	}
	key := argon2.IDKey([]byte(password), p.salt, p.iterations, p.memory, p.parallelism, uint32(len(p.key)))
	return subtle.ConstantTimeCompare(p.key, key) == 1, nil
}
