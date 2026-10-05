package tlsfp

import (
	"crypto/tls"
	"strings"
	"testing"
)

// TestCipherHashSpecVector 规格书 §2 的官方向量：15 个套件排序哈希 = 8daaf6152771。
// 输入故意乱序并混入 GREASE（0x0a0a），验证"排序 + 剔 GREASE"的归一化。
func TestCipherHashSpecVector(t *testing.T) {
	in := []uint16{
		0x1301, 0x0a0a, 0xc02f, 0x0035, 0x1302, 0xc013, 0x009c, 0x1303,
		0xc02b, 0x009d, 0xc014, 0xc02c, 0x002f, 0xc030, 0xcca8, 0xcca9,
	}
	got := hashList(in)
	want := "002f,0035,009c,009d,1301,1302,1303,c013,c014,c02b,c02c,c02f,c030,cca8,cca9"
	if got != want {
		t.Fatalf("排序串不符：\n got %s\nwant %s", got, want)
	}
	if h := hash12(got); h != "8daaf6152771" {
		t.Fatalf("cipher 哈希 = %s，规格向量 8daaf6152771", h)
	}
}

// TestPartA 规格书 §1 首段：t13d1516h2（TLS1.3 + SNI + 15 套件 + 16 扩展 + ALPN h2）。
// 17 个扩展里混 1 个 GREASE → 计数 16；supported_versions 全 GREASE 外取最大 0x0304。
func TestPartA(t *testing.T) {
	in := Input{
		ServerName:        "example.com",
		CipherSuites:      make([]uint16, 15),
		Extensions:        make([]uint16, 17),
		ALPNProtocols:     []string{"h2", "http/1.1"},
		SupportedVersions: []uint16{0x0a0a, 0x0304, 0x0303},
	}
	for i := range in.Extensions {
		in.Extensions[i] = uint16(0x100 + i)
	}
	in.Extensions[3] = 0x0a0a
	if got := JA4(in); !strings.HasPrefix(got, "t13d1516h2_") {
		t.Fatalf("part a = %s，应 t13d1516h2_…", got)
	}
}

// TestExtensionHashRules 扩展哈希：SNI(0000)/ALPN(0010) 被剔除；签名算法保序追加。
func TestExtensionHashRules(t *testing.T) {
	base := []uint16{0x0005, 0x000a, 0x0010, 0x0000, 0x002b, 0xff01}
	sigs := []uint16{0x0403, 0x0804, 0x0401}
	mk := func(exts []uint16, sigs []uint16) string {
		return JA4(Input{ServerName: "a", Extensions: exts, SignatureAlgorithms: sigs,
			SupportedVersions: []uint16{0x0304}, ALPNProtocols: []string{"h2"}})
	}

	withExtras := mk(append(append([]uint16{}, base...), 0x0010, 0x0000), sigs)
	without := mk(base, sigs)
	// part a 的扩展计数不同（8 vs 6），part b/c 应完全一致
	if withExtras[:12] == without[:12] {
		t.Fatalf("扩展计数应不同（8 vs 6）")
	}
	if withExtras[strings.Index(withExtras, "_")+1:] != without[strings.Index(without, "_")+1:] {
		t.Fatalf("SNI/ALPN 剔除后 part b/c 应一致")
	}

	// 签名算法保序：仅调换顺序 → part c 变化
	reversed := mk(base, []uint16{0x0401, 0x0804, 0x0403})
	if strings.Split(without, "_")[2] == strings.Split(reversed, "_")[2] {
		t.Fatalf("签名算法保序：调换顺序应改变 part c")
	}

	// 无签名算法：指纹不同但 part c 仍可计算
	noSig := mk(base, nil)
	if noSig == without {
		t.Fatalf("有无签名算法的指纹应不同")
	}
}

// TestALPNChars 规格：h2→h2、http/1.1→h1、缺失→00、非字母数字回退十六进制（0x30 0x31 0xab 0xcd → 3d）。
func TestALPNChars(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"h2", "http/1.1"}, "h2"},
		{[]string{"http/1.1"}, "h1"},
		{nil, "00"},
		{[]string{""}, "00"},
		{[]string{"a"}, "aa"},
		{[]string{"\x30\x31\xab\xcd"}, "3d"},
	}
	for _, c := range cases {
		if got := alpnChars(c.in); got != c.want {
			t.Errorf("alpnChars(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestVersionChars 版本映射表 + 全 GREASE → 00。
func TestVersionChars(t *testing.T) {
	cases := []struct {
		in   []uint16
		want string
	}{
		{[]uint16{0x0304, 0x0303}, "13"},
		{[]uint16{0x0303}, "12"},
		{[]uint16{0x0a0a}, "00"},
		{[]uint16{0x0302, 0x0a0a}, "11"},
		{nil, "00"},
	}
	for _, c := range cases {
		if got := versionChars(c.in); got != c.want {
			t.Errorf("versionChars(% x) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCount2 计数两位化与 99 封顶。
func TestCount2(t *testing.T) {
	if count2(6) != "06" || count2(99) != "99" || count2(120) != "99" {
		t.Fatalf("count2 计数化异常")
	}
}

// TestSNIChar SNI 有无 → d/i。
func TestSNIChar(t *testing.T) {
	if sniChar("example.com") != "d" || sniChar("") != "i" || sniChar("  ") != "i" {
		t.Fatalf("SNI 判定异常")
	}
}

// TestFromClientHello crypto/tls 适配层与 Input 路径结果一致，且形状合法。
func TestFromClientHello(t *testing.T) {
	hello := &tls.ClientHelloInfo{
		ServerName:        "example.com",
		CipherSuites:      []uint16{0x1301, 0x1302},
		Extensions:        []uint16{0x0000, 0x0010, 0x002b},
		SignatureSchemes:  []tls.SignatureScheme{0x0403, 0x0804},
		SupportedProtos:   []string{"h2"},
		SupportedVersions: []uint16{0x0304, 0x0303},
	}
	direct := JA4(Input{
		ServerName: "example.com", CipherSuites: []uint16{0x1301, 0x1302},
		Extensions: []uint16{0x0000, 0x0010, 0x002b}, SignatureAlgorithms: []uint16{0x0403, 0x0804},
		ALPNProtocols: []string{"h2"}, SupportedVersions: []uint16{0x0304, 0x0303},
	})
	if got := FromClientHello(hello); got != direct {
		t.Fatalf("FromClientHello = %s, want %s", got, direct)
	}
	if !strings.HasPrefix(direct, "t13d") {
		t.Fatalf("格式异常：%s", direct)
	}
	if empty := FromClientHello(nil); empty != "" {
		t.Fatalf("nil 输入应返回空串")
	}
}
