package httpapi

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// 客户端 IP 提取与受信代理解析。
//
// 安全要求：X-Forwarded-For / X-Real-IP **只在直连方属于 TRUSTED_PROXY 时才采信**，
// 且从右往左取第一个非受信地址（最接近真实客户端）。否则攻击者可自填 XFF：
//
//	curl -H "X-Forwarded-For: <他人 IP>" ...   → 把违规记到别人头上（栽赃）
//	                                           → 自身真实 IP 不累积（绕过封禁）
//
// 旧实现无条件采信 XFF 第一个值，两种攻击都成立。
//
// TRUSTED_PROXY 为空（默认）= 不信任任何代理头，一律使用 RemoteAddr。
// 配置示例（反向代理部署）：
//
//	TRUSTED_PROXY=127.0.0.1/32          # 本机 nginx
//	TRUSTED_PROXY=127.0.0.1/32,10.0.0.0/8  # 多级内网代理

type proxyTrust struct {
	env  string
	nets []*net.IPNet
}

var (
	trustMu    sync.Mutex
	trustCache = proxyTrust{}
	warnedXFF  sync.Once
)

// trustedProxy 解析 TRUSTED_PROXY（以 env 字符串为缓存键，便于测试切换）。
func trustedProxy() proxyTrust {
	env := strings.TrimSpace(os.Getenv("TRUSTED_PROXY"))
	trustMu.Lock()
	defer trustMu.Unlock()
	if trustCache.env == env {
		return trustCache
	}
	t := proxyTrust{env: env}
	for _, part := range strings.Split(env, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") { // 裸 IP → 补全前缀长度
			if ip := net.ParseIP(part); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				part = part + "/" + strconv.Itoa(bits)
			}
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			t.nets = append(t.nets, n)
		}
	}
	trustCache = t
	return t
}

func (t proxyTrust) trusted(ip string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return false
	}
	for _, n := range t.nets {
		if n.Contains(p) {
			return true
		}
	}
	return false
}

// clientIPFromRequest 取客户端 IP：默认只信任 RemoteAddr；配置 TRUSTED_PROXY 后
// 才解析代理头，并从右往左取第一个非受信地址。
func clientIPFromRequest(r *http.Request) string {
	remote := clientIP(r.RemoteAddr)
	t := trustedProxy()
	if len(t.nets) == 0 || !t.trusted(remote) {
		if r.Header.Get("X-Forwarded-For") != "" {
			warnedXFF.Do(func() {
				log.Printf("[ipguard] 检测到 X-Forwarded-For 但未配置 TRUSTED_PROXY，已忽略该头" +
					"（若部署在反向代理后请配置，如 TRUSTED_PROXY=127.0.0.1/32）")
			})
		}
		return remote
	}
	// 直连方受信：XFF 形如 "客户端, 代理1, 代理2"；从右往左跳过受信代理，
	// 第一个非受信地址即最近的真实客户端
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(parts[i])
			if ip == "" {
				continue
			}
			if !t.trusted(ip) {
				return ip
			}
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" && !t.trusted(xri) {
		return xri
	}
	return remote
}
