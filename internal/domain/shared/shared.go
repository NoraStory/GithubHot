// Package shared 提供跨限界上下文的通用值对象与端口。
package shared

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Clock 是时间端口。领域层与测试通过它取当前时间，
// 保证热度计算（时间窗、半衰期）可以确定性测试。
type Clock interface {
	Now() time.Time
}

// SystemClock 是生产环境的 Clock 实现。
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FixedClock 供测试使用。
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }

var utmParam = regexp.MustCompile(`(?i)^[ub]tm_[a-z]+`)

// NormalizeURL 归一化 URL 用于判重：去 utm_* / fbclid 追踪参数，
// 统一 scheme 与 host 小写、去掉默认端口与末尾斜杠。
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("解析 URL %q: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("URL 缺少 scheme 或 host: %q", raw)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if p := u.Port(); p == "80" && u.Scheme == "http" || p == "443" && u.Scheme == "https" {
		u.Host = u.Hostname()
	}
	q := u.Query()
	for k := range q {
		if utmParam.MatchString(k) || strings.EqualFold(k, "fbclid") || strings.EqualFold(k, "ref") {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Fragment = ""
	return u.String(), nil
}

// DedupeKey 返回内容的判重键：URL 归一化后的 SHA-256（仅用于判重，非安全用途）。
func DedupeKey(rawURL string) (string, error) {
	n, err := NormalizeURL(rawURL)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(n))
	return hex.EncodeToString(sum[:]), nil
}

// NewID 用前缀 + 随机数生成可读 ID。
func NewID(prefix string) string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%x", prefix, b)
}

// Truncate 按字节数截断字符串，避免把多字节字符截成半个。
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	out := make([]rune, 0, max)
	n := 0
	for _, c := range r {
		sz := len(string(c))
		if n+sz > max {
			break
		}
		out = append(out, c)
		n += sz
	}
	return string(out)
}
