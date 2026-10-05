package geoip

import (
	"net"
	"os"
	"testing"
)

// TestOpenRealDatabases 真库探测（可选测试）：设置了 GEOIP_DB_PATH 时打开真实
// mmdb，验证 geoip2 读取器与 DBIP 库 schema 的兼容性，并用已知 IP 抽查判定。
// 文件缺失（离线环境/CI）自动跳过。
func TestOpenRealDatabases(t *testing.T) {
	country, asn := DefaultPaths()
	if _, err := os.Stat(country); err != nil {
		t.Skipf("国家库不存在（%s），跳过真库测试：githubhot geo download 可拉取", country)
	}
	s := Open(country, asn)
	if !s.Enabled() {
		t.Fatalf("库文件存在但打开失败（mmdb 格式/权限问题）：%s", country)
	}
	ip := net.ParseIP("223.5.5.5") // 阿里公共 DNS（中国，AS37963 阿里云）
	got := s.Country(ip)
	if got == "" {
		t.Fatalf("Country(223.5.5.5) 返回空——读取器与库 schema 不兼容或库过期")
	}
	asnNum, org := s.ASN(ip)
	t.Logf("223.5.5.5 → country=%q asn=%d org=%q hosting=%v", got, asnNum, org, HostingOrg(org))
	if asnNum == 0 && org == "" && asn != "" {
		t.Fatalf("ASN 库存在但查询失败（schema 兼容性？）：%s", asn)
	}
	// 时区大洲核验的对照用例：中国 IP + 美洲时区 → 必须判跨洲不符
	if !ContinentMismatch(got, "America/New_York") {
		t.Fatalf("country=%s + America/New_York 应判跨洲不符", got)
	}
	if ContinentMismatch(got, "Asia/Shanghai") {
		t.Fatalf("country=%s + Asia/Shanghai 不应判不符", got)
	}
}

// TestContinentMismatch 判定表纯逻辑（不依赖真库）。
func TestContinentMismatch(t *testing.T) {
	cases := []struct {
		country, tz string
		want        bool
	}{
		{"CN", "America/New_York", true},   // 跨洲：命中
		{"CN", "Asia/Shanghai", false},     // 同洲：放行
		{"CN", "UTC", false},               // 无大洲信息：跳过
		{"XX", "America/New_York", false},  // 未知国家：跳过
		{"US", "Europe/Berlin", true},      // 跨洲
		{"RU", "Europe/Moscow", false},     // 跨洲国家宽松集合：欧洲部分放行
		{"RU", "Asia/Novosibirsk", false},  // 亚洲部分放行
		{"", "Asia/Shanghai", false},       // 空国家：跳过
	}
	for _, c := range cases {
		if got := ContinentMismatch(c.country, c.tz); got != c.want {
			t.Errorf("ContinentMismatch(%q, %q) = %v, want %v", c.country, c.tz, got, c.want)
		}
	}
}

// TestHostingOrg 机房组织名关键词判定。
func TestHostingOrg(t *testing.T) {
	yes := []string{"Alibaba (US) Technology Co., Ltd.", "OVH SAS", "Hetzner Online GmbH",
		"Google Cloud Platform (GCP)", "Amazon.com, Inc.", "Microsoft Corporation"}
	no := []string{"China Telecom", "CHINANET", "Comcast Cable", ""}
	for _, org := range yes {
		if !HostingOrg(org) {
			t.Errorf("%q 应判为 hosting", org)
		}
	}
	for _, org := range no {
		if HostingOrg(org) {
			t.Errorf("%q 不应判为 hosting", org)
		}
	}
}
