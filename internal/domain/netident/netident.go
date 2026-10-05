// Package netident 客户端网络身份值对象（纯算法，零 IO、零依赖）。
//
// 用途：把"同一个客户端出口"判定为可比对的作用域，而不是精确 IP 相等——
// 移动网络与 CGNAT 出口会在同网段内漂移（重拨一次就换个 IP），
// 按精确 IP 绑定会话会把正常用户踢下线。
package netident

import (
	"bytes"
	"net"
	"strings"
)

// 作用域前缀长度：IPv4 按 /24、IPv6 按 /64。
const (
	IPv4ScopeBits = 24
	IPv6ScopeBits = 64
)

// SameScope 判断两个 IP 是否属于同一客户端作用域。
//
//   - strict=true：必须完全一致（ADMIN_SESSION_IP_STRICT=1）；
//   - strict=false：IPv4 比 /24、IPv6 比 /64；
//   - 任一为空：要求两者都为空才视为一致（缺失信息不放宽）；
//   - 无法解析或地址族不同（v4 对 v6）：不放宽，仅完全相等才为 true
//     （注意 IPv4-mapped IPv6 会先归一成 v4 再比较）。
func SameScope(a, b string, strict bool) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	if a == b {
		return true
	}
	if strict {
		return false
	}
	ipa, ipb := net.ParseIP(a), net.ParseIP(b)
	if ipa == nil || ipb == nil {
		return false
	}
	va, vb := ipa.To4(), ipb.To4()
	if (va == nil) != (vb == nil) {
		return false // 地址族不同，不是同一个出口
	}
	if va != nil {
		return va.Mask(net.CIDRMask(IPv4ScopeBits, 32)).Equal(vb.Mask(net.CIDRMask(IPv4ScopeBits, 32)))
	}
	mask := net.CIDRMask(IPv6ScopeBits, 128)
	return bytes.Equal(ipa.Mask(mask), ipb.Mask(mask))
}
