package netident

import "testing"

func TestSameScopeIPv4(t *testing.T) {
	cases := []struct {
		name    string
		a, b    string
		strict  bool
		want    bool
	}{
		{"完全一致", "203.0.113.7", "203.0.113.7", false, true},
		{"同 /24", "203.0.113.7", "203.0.113.99", false, true},
		{"同 /24（严格模式拒绝）", "203.0.113.7", "203.0.113.99", true, false},
		{"跨 /24", "203.0.113.7", "203.0.114.7", false, false},
		{"跨 /16", "10.0.1.5", "10.9.1.5", false, false},
		{"裸 IP 的 /25 邻居属同 /24", "198.51.100.1", "198.51.100.200", false, true},
		{"空值不视为一致", "", "203.0.113.7", false, false},
		{"双空一致", "", "", false, true},
		{"非法输入不放宽", "not-an-ip", "203.0.113.7", false, false},
		{"非法输入相等则通过", "x", "x", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SameScope(c.a, c.b, c.strict); got != c.want {
				t.Fatalf("SameScope(%q,%q,strict=%v) = %v, want %v", c.a, c.b, c.strict, got, c.want)
			}
		})
	}
}

func TestSameScopeIPv6(t *testing.T) {
	cases := []struct {
		name   string
		a, b   string
		strict bool
		want   bool
	}{
		{"同 /64", "2001:db8:1:2::1", "2001:db8:1:2::beef", false, true},
		{"同 /64 压缩写法", "2001:db8:1:2:0:0:0:1", "2001:db8:1:2::beef", false, true},
		{"跨 /64", "2001:db8:1:2::1", "2001:db8:1:3::1", false, false},
		{"同 /64 严格拒绝", "2001:db8:1:2::1", "2001:db8:1:2::beef", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SameScope(c.a, c.b, c.strict); got != c.want {
				t.Fatalf("SameScope(%q,%q,strict=%v) = %v, want %v", c.a, c.b, c.strict, got, c.want)
			}
		})
	}
}

// TestSameScopeMixedFamily 地址族不同（v4 vs v6）不得视为同一出口，
// 但 IPv4-mapped IPv6（::ffff:1.2.3.4）应先归一成 v4 再按 /24 比较。
func TestSameScopeMixedFamily(t *testing.T) {
	if SameScope("203.0.113.7", "2001:db8::7", false) {
		t.Fatalf("v4 与 v6 不应同作用域")
	}
	if !SameScope("203.0.113.7", "::ffff:203.0.113.7", false) {
		t.Fatalf("IPv4-mapped IPv6 应归一后等价")
	}
	if !SameScope("203.0.113.7", "::ffff:203.0.113.200", false) {
		t.Fatalf("IPv4-mapped IPv6 的 /24 邻居应同作用域")
	}
}
