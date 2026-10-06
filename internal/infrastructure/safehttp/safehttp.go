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

// DefaultTimeout 单次请求默认超时。LLM 批量判定调用即使关闭思考
// 也可能超过 30s，取宽裕值；RSS/GitHub 正常请求远低于此。
const DefaultTimeout = 90 * time.Second

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
	if _, err := resolvePublic(ctx, host); err != nil {
		return nil, err
	}
	return u, nil
}

// lookupHost 解析函数变量（测试可替换以模拟 rebinding/解析结果）。
// 生产路径 = 标准 Resolver.LookupIPAddr；预校验与拨号时刻共用同一注入点。
var lookupHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return (&net.Resolver{}).LookupIPAddr(ctx, host)
}

// resolvePublic 解析 host 并要求全部结果为公网（预校验与 safeDialContext
// 共用口径：任一结果非公网即全拒，防 round-robin 混入内网地址）。
func resolvePublic(ctx context.Context, host string) ([]net.IPAddr, error) {
	ipCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := lookupHost(ipCtx, host)
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
	return addrs, nil
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
// 直连模式（默认）：scheme/host 字面量 + DNS 解析结果全为公网地址才放行，
// 且拨号时在连接时刻重新解析校验并直连校验通过的 IP——预校验与实际连接
// 若各自独立解析，攻击者可在两次解析之间切换 A 记录到内网地址（DNS
// rebinding TOCTOU），因此校验必须与拨号同源。
// 代理模式（设置了 HTTPS_PROXY/HTTP_PROXY）：跳过 DNS 预解析、由代理负责
// 出口解析——Clash TUN/fake-ip 等环境解析出的 198.18/15 是代理伪 IP 而非
// 真实目标；主机名禁用名单与 IP 字面量校验保持不变。
//
// 重定向每一跳都重新校验（CheckRedirect）：公网入口 302 到内网地址是经典
// SSRF 绕过，默认跟随重定向必须同样过 scheme/host/DNS 三道检查。
func Do(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader) ([]byte, int, error) {
	if err := validate(ctx, rawURL); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, 0, fmt.Errorf("构造请求: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout: DefaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向超过 5 次，放弃")
			}
			return validate(req.Context(), req.URL.String())
		},
	}
	if !UsingProxy() {
		client.Transport = directTransport
	}
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

// directTransport 直连模式共享传输层：DialContext 在连接时刻解析并校验。
// TLS SNI 与 Host 头由 Transport 依据原始 URL host 生成，直连 IP 不影响。
var directTransport = &http.Transport{
	DialContext:         safeDialContext,
	TLSHandshakeTimeout: 10 * time.Second,
	IdleConnTimeout:     90 * time.Second,
}

// safeDialContext 连接时刻的 SSRF 校验拨号：复用 resolvePublic（任一结果非公网
// 即全拒）→ 直连校验通过的地址。与预校验共用 lookupHost 注入点，但这是独立
// 的第二次解析——攻击者在预校验后切换 DNS，此处仍会拦截（rebinding 防线）。
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("拨号地址非法 %q: %w", addr, err)
	}
	if net.ParseIP(host) != nil {
		// IP 字面量已在 ValidateURL 判定，直接拨
		return (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, addr)
	}
	lower := strings.ToLower(host)
	if forbiddenHostNames[lower] || strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return nil, fmt.Errorf("%w: %s", ErrHostForbidden, host)
	}
	addrs, err := resolvePublic(ctx, host)
	if err != nil {
		return nil, err
	}
	// 优先 IPv4，失败逐个尝试其余公网地址
	var lastErr error
	for _, a := range addrs {
		if a.IP.To4() == nil && len(addrs) > 1 {
			continue
		}
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无可用公网地址连接 %s", host)
	}
	return nil, fmt.Errorf("连接 %s 失败: %w", host, lastErr)
}

// validate 发出请求前（含重定向每一跳）的完整 SSRF 校验。
func validate(ctx context.Context, raw string) error {
	if _, err := ValidateURL(raw); err != nil {
		return err
	}
	if !UsingProxy() {
		if _, err := ValidateAndResolve(ctx, raw); err != nil {
			return err
		}
	}
	return nil
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
