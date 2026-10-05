// Package tlsfp — P3-2 JA4 TLS 客户端指纹（规格书 §6 P3-2）。
//
// 按 FoxIO-LLC/ja4 公开技术规格（technical_details/JA4.md，BSD-3，规格书 §0.8 允许）
// 自实现，不引入 ja4plus 等第三方依赖；捕捉点用 stdlib 的
// tls.Config.GetConfigForClient(*tls.ClientHelloInfo)（含 Extensions 原始顺序表），
// 不需要包装 net.Listener 解析原始 ClientHello 字节。
//
// 归一化再哈希原则（规格书附录 A）：cipher/extension 排序后哈希、GREASE 全域剔除——
// 这是 JA4 相对 JA3 的抗对抗核心：随机化扩展顺序不改变指纹。
package tlsfp

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Input JA4 计算的输入（与 crypto/tls.ClientHelloInfo 字段对齐，方便测试构造）。
type Input struct {
	ServerName          string   // SNI（有值 → "d"，无 → "i"）
	CipherSuites        []uint16 // 客户端候选套件（原样，含 GREASE，由本包剔除）
	Extensions          []uint16 // 扩展 ID（原始顺序，含 GREASE/SNI/ALPN）
	SignatureAlgorithms []uint16 // 签名算法（原始顺序——JA4_c 保序追加）
	ALPNProtocols       []string // ALPN 候选（第一个参与 part a）
	SupportedVersions   []uint16 // supported_versions 候选（GREASE 剔除后取最大）
}

// isGREASE draft-davidben-tls-grease-01：0x?a?a 形式的值。
func isGREASE(v uint16) bool { return v&0x0f0f == 0x0a0a }

// filterGREASE 剔除 GREASE 值（保序）。
func filterGREASE(in []uint16) []uint16 {
	out := make([]uint16, 0, len(in))
	for _, v := range in {
		if !isGREASE(v) {
			out = append(out, v)
		}
	}
	return out
}

// versionChars 版本 → 2 字符（规格映射表；supported_versions 取最大非 GREASE 值）。
func versionChars(in []uint16) string {
	vals := filterGREASE(in)
	if len(vals) == 0 {
		return "00"
	}
	max := vals[0]
	for _, v := range vals[1:] {
		if v > max {
			max = v
		}
	}
	switch max {
	case 0x0304:
		return "13"
	case 0x0303:
		return "12"
	case 0x0302:
		return "11"
	case 0x0301:
		return "10"
	case 0x0300:
		return "s3"
	case 0x0002:
		return "s2"
	case 0xfeff:
		return "d1"
	case 0xfefd:
		return "d2"
	case 0xfefc:
		return "d3"
	default:
		return "00"
	}
}

// sniChar SNI 有值 → "d"（domain），无 → "i"（IP/无 SNI）。
func sniChar(serverName string) string {
	if strings.TrimSpace(serverName) != "" {
		return "d"
	}
	return "i"
}

// count2 计数 → 2 位十进制，>99 记 99。
func count2(n int) string {
	if n > 99 {
		n = 99
	}
	if n < 0 {
		n = 0
	}
	return fmt.Sprintf("%02d", n)
}

// alpnChars ALPN 第一个值的首/尾 ASCII 字母数字字符；任一端不是字母数字 →
// 用首/尾字节的十六进制首/尾字符替代（规格示例 0x30 0x31 0xab 0xcd → "3d"）；
// 无 ALPN → "00"。
func alpnChars(prots []string) string {
	if len(prots) == 0 || prots[0] == "" {
		return "00"
	}
	p := prots[0]
	first, last := p[0], p[len(p)-1]
	if isASCIIAlnum(first) && isASCIIAlnum(last) {
		return strings.ToLower(string(first) + string(last))
	}
	// 非字母数字：取首字节十六进制的首字符 + 尾字节十六进制的尾字符
	hf := fmt.Sprintf("%02x", first)
	hl := fmt.Sprintf("%02x", last)
	return hf[:1] + hl[1:2]
}

func isASCIIAlnum(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// hash12 规格的 12 位截断 SHA256（小写 hex）。
func hash12(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// hashList 排序后的 4 位小写 hex 逗号串（无尾逗号）。
func hashList(vals []uint16) string {
	sorted := filterGREASE(vals)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, 0, len(sorted))
	for _, v := range sorted {
		parts = append(parts, fmt.Sprintf("%04x", v))
	}
	return strings.Join(parts, ",")
}

// JA4 计算完整指纹：t<版本><d|i><cipher 计数><ext 计数><ALPN>_<cipher 哈希>_<ext 哈希>。
// 扩展哈希剔除 SNI(0000) 与 ALPN(0010)（已由 part a 表达），签名算法按原始顺序追加；
// 无签名算法时字符串不带下划线结尾；空列表的哈希段为 000000000000。
func JA4(in Input) string {
	// part a
	ciphers := filterGREASE(in.CipherSuites)
	exts := filterGREASE(in.Extensions)
	a := "t" + versionChars(in.SupportedVersions) + sniChar(in.ServerName) +
		count2(len(ciphers)) + count2(len(exts)) + alpnChars(in.ALPNProtocols)

	// part b：cipher 排序哈希
	b := "000000000000"
	if len(ciphers) > 0 {
		b = hash12(hashList(in.CipherSuites))
	}

	// part c：扩展排序（剔除 SNI/ALPN/GREASE）+ "_" + 签名算法（保序、剔 GREASE）
	c := "000000000000"
	extHash := ""
	{
		kept := make([]uint16, 0, len(exts))
		for _, v := range exts {
			if v == 0x0000 || v == 0x0010 {
				continue
			}
			kept = append(kept, v)
		}
		if len(kept) > 0 {
			extHash = hashList(kept)
		}
	}
	sigs := filterGREASE(in.SignatureAlgorithms)
	switch {
	case extHash == "" && len(sigs) == 0:
		// 两个列表都空：保持 000000000000
	case len(sigs) == 0:
		c = hash12(extHash)
	default:
		sigParts := make([]string, 0, len(sigs))
		for _, v := range sigs {
			sigParts = append(sigParts, fmt.Sprintf("%04x", v))
		}
		if extHash == "" {
			c = hash12(strings.Join(sigParts, ","))
		} else {
			c = hash12(extHash + "_" + strings.Join(sigParts, ","))
		}
	}
	return a + "_" + b + "_" + c
}

// FromClientHello crypto/tls 适配：从服务端握手回调构造 Input 并计算。
func FromClientHello(hello *tls.ClientHelloInfo) string {
	if hello == nil {
		return ""
	}
	alpn := hello.SupportedProtos
	if alpn == nil {
		alpn = []string{}
	}
	return JA4(Input{
		ServerName:          hello.ServerName,
		CipherSuites:        hello.CipherSuites,
		Extensions:          hello.Extensions,
		SignatureAlgorithms: sigAlgs(hello.SignatureSchemes),
		ALPNProtocols:       alpn,
		SupportedVersions:   hello.SupportedVersions,
	})
}

func sigAlgs(in []tls.SignatureScheme) []uint16 {
	out := make([]uint16, 0, len(in))
	for _, s := range in {
		out = append(out, uint16(s))
	}
	return out
}
