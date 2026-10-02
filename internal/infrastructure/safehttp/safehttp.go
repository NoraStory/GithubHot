// Package safehttp 是所有出站 HTTP 的唯一通道：仅允许 http/https，
// 请求前校验 host 并拒绝 localhost、环回、私有与保留地址（SSRF 防护）。
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultTimeout 单次请求默认超时。
const DefaultTimeout = 30 * time.Second

// UserAgent 出站请求标识。
const UserAgent = "GithubHot/0.1 (+https://github.com/NoraStory/GithubHot)"

var (
	// ErrSchemeNotAllowed 仅允许 http/https。
	ErrSchemeNotAllowed = errors.New("仅允许 http/https 协议")
	// ErrHostForbidden host 命中禁用名单。
	ErrHostForbidden = errors.New("host 被拒绝（localhost/私有/保留地址）")
	// ErrDNSRejected DNS 解析结果包含非公网地址。
	ErrDNSRejected = errors.New("DNS 解析结果包含私有/保留地址")
)

// forbiddenHostNames 主机名禁用名单（含常见云元数据端点）。
var forbiddenHostNames = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true,
	"metadata.goog":            true,
	"instance-data":            true,
}

// ValidateURL 校验 URL 的 scheme 与 host 字面量（不发 DNS）。
// 返回解析后的 *url.URL；host 名的 DNS 级校验在 Connect 阶段执行。
func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("URL 解析失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: %q", ErrSchemeNotAllowed, u.Scheme)
	}
	host := u.Hostname() // 不带端口、不带方括号
	if host == "" {
		return nil, fmt.Errorf("URL 缺少 host: %q", raw)
	}
	lower := strings.ToLower(host)
	if forbiddenHostNames[lower] || strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return nil, fmt.Errorf("%w: %s", ErrHostForbidden, host)
	}
	// IP 字面量直接判段
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("%w: %s", ErrHostForbidden, host)
		}
	}
	return u, nil
}

// ValidateAndResolve 完整校验：scheme/host 字面量 + DNS 解析结果全部为公网地址。
// 返回校验通过的 URL（供调用方发请求前复核）。
func ValidateAndResolve(ctx context.Context, raw string) (*url.URL, error) {
	u, err := ValidateURL(raw)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return u, nil // 字面量已在 ValidateURL 判过
	}
	resolver := &net.Resolver{}
	ipCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ipCtx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS 解析 %s 失败: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("DNS 解析 %s 无结果", host)
	}
	for _, a := range addrs {
		if !isPublicIP(a.IP) {
			return nil, fmt.Errorf("%w: %s -> %s", ErrDNSRejected, host, a.IP)
		}
	}
	return u, nil
}

// isPublicIP 判定是否公网地址：拒绝环回、私有、链路本地、组播、未指定等。
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// IPv4 保留段（net.IP.IsPrivate 未覆盖的部分）
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // 100.64/10 CGNAT
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 0: // 192.0.0/24
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 2: // 192.0.2/24 TEST-NET-1
			return false
		case v4[0] == 198 && v4[1] == 51 && v4[2] == 100: // 198.51.100/24 TEST-NET-2
			return false
		case v4[0] == 203 && v4[1] == 0 && v4[2] == 113: // 203.0.113/24 TEST-NET-3
			return false
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // 198.18/15 基准测试
			return false
		case v4[0] >= 240: // 240/4 保留 + 255.255.255.255
			return false
		}
		return true
	}
	// IPv6 保留段
	if v6 := ip.To16(); v6 != nil {
		// 2001:db8::/32 文档段；fc00::/7 已由 IsPrivate 覆盖
		if v6[0] == 0x20 && v6[1] == 0x01 && v6[2] == 0x0d && v6[3] == 0xb8 {
			return false
		}
	}
	return true
}

// Fetch 经过 SSRF 校验的 GET：先 ValidateAndResolve，再请求（带超时与 UA）。
func Fetch(ctx context.Context, rawURL string, headers map[string]string) ([]byte, int, error) {
	return Do(ctx, http.MethodGet, rawURL, headers, nil)
}

// Do 经过 SSRF 校验的任意方法请求。
// 直连模式（默认）：scheme/host 字面量 + DNS 解析结果全为公网地址才放行。
// 代理模式（设置了 HTTPS_PROXY/HTTP_PROXY）：跳过 DNS 预解析、由代理负责
// 出口解析——Clash TUN/fake-ip 等环境解析出的 198.18/15 是代理伪 IP 而非
// 真实目标；主机名禁用名单与 IP 字面量校验保持不变。
func Do(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader) ([]byte, int, error) {
	if _, err := ValidateURL(rawURL); err != nil {
		return nil, 0, err
	}
	if !UsingProxy() {
		if _, err := ValidateAndResolve(ctx, rawURL); err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, 0, fmt.Errorf("构造请求: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("请求 %s: %w", req.Host, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB 上限
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读取响应: %w", err)
	}
	return data, resp.StatusCode, nil
}

// UsingProxy 报告是否配置了代理环境变量（Go 默认传输层会遵循它们）。
func UsingProxy() bool {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}
