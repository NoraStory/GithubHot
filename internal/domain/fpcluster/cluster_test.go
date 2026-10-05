package fpcluster

import "testing"

func f64(v float64) *float64 { return &v }

// TestBuildSharedIP 两个指纹共享 ≥2 个 IP → 一簇（规格边 1）。
func TestBuildSharedIP(t *testing.T) {
	nodes := []NodeInput{
		{FP: "fp-a", IPs: []string{"1.1.1.1", "2.2.2.2"}, UA: "UA-A"},
		{FP: "fp-b", IPs: []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}, UA: "UA-B"},
	}
	clusters := Build(nodes, nil)
	if len(clusters) != 1 || clusters[0].Size != 2 || clusters[0].Reason != "共享IP" {
		t.Fatalf("应 1 簇 2 成员（共享IP）：%+v", clusters)
	}
	// 只共享 1 个 IP → 不连边
	clusters = Build([]NodeInput{
		{FP: "fp-a", IPs: []string{"1.1.1.1"}, UA: "A"},
		{FP: "fp-b", IPs: []string{"1.1.1.1", "9.9.9.9"}, UA: "B"},
	}, nil)
	if len(clusters) != 0 {
		t.Fatalf("共享 1 IP 不应成簇：%+v", clusters)
	}
}

// TestBuildLinksAndChaining 既有关联边成簇 + 簇经传递闭包合并（连通分量的核心价值）。
func TestBuildLinksAndChaining(t *testing.T) {
	nodes := []NodeInput{{FP: "fp-a"}, {FP: "fp-b"}, {FP: "fp-c"}}
	links := []LinkInput{{Src: "fp-a", Dst: "fp-b", Kind: "phash"}}
	clusters := Build(nodes, links)
	if len(clusters) != 1 || clusters[0].Reason != "pHash" {
		t.Fatalf("pHash 边应成簇：%+v", clusters)
	}
	// fp-c 经 minhash 边连到 fp-b → 三者合并为一簇
	links = append(links, LinkInput{Src: "fp-b", Dst: "fp-c", Kind: "minhash"})
	clusters = Build(nodes, links)
	if len(clusters) != 1 || clusters[0].Size != 3 {
		t.Fatalf("传递闭包应合并 3 成员：%+v", clusters)
	}
	if !strings_contains(clusters[0].Reason, "pHash") || !strings_contains(clusters[0].Reason, "MinHash") {
		t.Fatalf("reason 应含两类边：%q", clusters[0].Reason)
	}
}

func strings_contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestBuildPhysicalAndJA4 边 3（时钟偏移+行为）与边 4（JA4 同 + UA 异）。
func TestBuildPhysicalAndJA4(t *testing.T) {
	// 边 3：skew 差 2ppm + 余弦相似 1.0 → 簇
	nodes := []NodeInput{
		{FP: "fp-a", BehaviorJSON: `{"mouse":{"speed_mean":1,"speed_var":2,"curvature_mean":3,"jerk_var":4,"dir_change_rate":5,"events":100},"keys":{"dwell_mean":1,"dwell_var":2,"flight_mean":3,"flight_var":4,"events":50}}`, ClockSkew: f64(11.0)},
		{FP: "fp-b", BehaviorJSON: `{"mouse":{"speed_mean":1,"speed_var":2,"curvature_mean":3,"jerk_var":4,"dir_change_rate":5,"events":100},"keys":{"dwell_mean":1,"dwell_var":2,"flight_mean":3,"flight_var":4,"events":50}}`, ClockSkew: f64(13.0)},
	}
	clusters := Build(nodes, nil)
	if len(clusters) != 1 || clusters[0].Reason != "时钟偏移+行为" {
		t.Fatalf("边 3 应成簇：%+v", clusters)
	}
	// skew 差超 5ppm → 不连
	nodes[1].ClockSkew = f64(20.0)
	if clusters := Build(nodes, nil); len(clusters) != 0 {
		t.Fatalf("skew 差过大不应成簇：%+v", clusters)
	}
	// 行为缺失（任一）→ 不连
	nodes[1].ClockSkew = f64(12.0)
	nodes[1].BehaviorJSON = ""
	if clusters := Build(nodes, nil); len(clusters) != 0 {
		t.Fatalf("行为数据缺失不应成簇：%+v", clusters)
	}

	// 边 4：JA4 相同 + UA 不同 → 簇；UA 相同 → 不连
	ja4Nodes := []NodeInput{
		{FP: "fp-x", JA4: "t13d1516h2_8daaf6152771_b186095e22b6", UA: "Chrome"},
		{FP: "fp-y", JA4: "t13d1516h2_8daaf6152771_b186095e22b6", UA: "curl/8.0"},
	}
	if clusters := Build(ja4Nodes, nil); len(clusters) != 1 || clusters[0].Reason != "JA4同" {
		t.Fatalf("边 4 应成簇：%+v", clusters)
	}
	ja4Nodes[1].UA = "Chrome"
	if clusters := Build(ja4Nodes, nil); len(clusters) != 0 {
		t.Fatalf("UA 相同不应成簇：%+v", clusters)
	}
}

// TestBuildIgnoresLoners 孤立节点不成簇；空输入安全。
func TestBuildIgnoresLoners(t *testing.T) {
	nodes := []NodeInput{{FP: "lonely"}}
	if clusters := Build(nodes, nil); len(clusters) != 0 {
		t.Fatalf("孤立节点不应成簇")
	}
	if clusters := Build(nil, nil); len(clusters) != 0 {
		t.Fatalf("空输入应返回空")
	}
}
