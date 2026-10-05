// Package fpcluster — P4-4 图聚类（马甲合并，连通分量版，规格书 §7 P4-4）。
//
// 节点 = ip_fingerprints.fp；边（30 天窗口，任一成立即连边）：
//   1. 共享 IP：两指纹 IP 交集 ≥ 2（同一 NAT/出口背后的多身份）；
//   2. 数学/物理关联：fp_links 已有关联边（P2-1 pHash / P2-2 MinHash / similar / physical）；
//   3. 物理特征：clock_skew_ppm 差 < 5 且 behavior 余弦相似 > 0.9（双条件同时成立）；
//   4. TLS 栈相同而 UA 声称互异（P3 数据：同 JA4、UA 不同）。
// 算法：union-find 连通分量（数据量 < 1e5 无需 Louvain——那是 P6-2）。纯函数，零 IO。
package fpcluster

import (
	"sort"
	"strings"

	"github.com/NoraStory/GithubHot/internal/domain/behavior"
)

// NodeInput 集群输入的每指纹数据（ListFingerprintsSince 行裁剪）。
type NodeInput struct {
	FP           string
	IPs          []string
	UA           string
	JA4          string
	BehaviorJSON string
	ClockSkew    *float64
}

// LinkInput fp_links 既有关联边。
type LinkInput struct {
	Src, Dst, Kind string
}

// Cluster 一个连通分量。
type Cluster struct {
	Members []string // 排序后的成员 fp
	Size    int
	Reason  string // 边类型组合（去重升序，"/" 连接）
}

const (
	skewTolerancePPM   = 5.0
	behaviorSimilarity = 0.9
	sharedIPMin        = 2
)

// Build 构图并求连通分量。返回按 size 降序、成员数 ≥ 2 的簇。
func Build(nodes []NodeInput, links []LinkInput) []Cluster {
	index := map[string]int{}
	for i, n := range nodes {
		index[n.FP] = i
	}
	parent := make([]int, len(nodes))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	reasons := map[int]map[string]bool{} // 根索引 → 边类型集合（归并到根）
	touch := func(a, b int, kind string) {
		union(a, b)
		root := find(a)
		if reasons[root] == nil {
			reasons[root] = map[string]bool{}
		}
		reasons[root][kind] = true
	}

	// 边 2：既有关联边（pHash / minhash / similar / physical / time_cooccur）
	kindName := map[string]string{
		"phash": "pHash", "minhash": "MinHash", "similar": "相似",
		"physical": "物理特征", "time_cooccur": "时间共现",
	}
	for _, l := range links {
		a, okA := index[l.Src]
		b, okB := index[l.Dst]
		if !okA || !okB || a == b {
			continue
		}
		name := kindName[l.Kind]
		if name == "" {
			name = l.Kind
		}
		touch(a, b, name)
	}

	// 预解析 behavior 特征向量与时钟偏移（边 3 用）
	vec := map[int][]float64{}
	skew := map[int]float64{}
	for i, n := range nodes {
		if v := behavior.Parse(n.BehaviorJSON); v != nil {
			vec[i] = behavior.FeatureVector(v)
		}
		if n.ClockSkew != nil {
			skew[i] = *n.ClockSkew
		}
	}

	// 两两规则（数据量 < 1e5，O(n²) 可接受；>1e5 时应先按 P6 Louvain 演进）
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			// 边 1：共享 IP ≥ 2
			shared := sharedIPCount(nodes[i].IPs, nodes[j].IPs)
			if shared >= sharedIPMin {
				touch(i, j, "共享IP")
				continue
			}
			// 边 3：时钟偏移差 < 5ppm 且行为余弦 > 0.9
			si, iHas := skew[i]
			sj, jHas := skew[j]
			if iHas && jHas && abs(si-sj) < skewTolerancePPM &&
				len(vec[i]) > 0 && len(vec[j]) > 0 &&
				behavior.CosineSimilarity(vec[i], vec[j]) > behaviorSimilarity {
				touch(i, j, "时钟偏移+行为")
				continue
			}
			// 边 4：JA4 相同而 UA 声称互异
			if nodes[i].JA4 != "" && nodes[i].JA4 == nodes[j].JA4 &&
				strings.TrimSpace(nodes[i].UA) != strings.TrimSpace(nodes[j].UA) &&
				nodes[i].UA != "" && nodes[j].UA != "" {
				touch(i, j, "JA4同")
			}
		}
	}

	// 收簇
	groups := map[int][]string{}
	for i, n := range nodes {
		root := find(i)
		groups[root] = append(groups[root], n.FP)
	}
	out := []Cluster{}
	for root, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		kinds := make([]string, 0, len(reasons[root]))
		for k := range reasons[root] {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		out = append(out, Cluster{Members: members, Size: len(members), Reason: strings.Join(kinds, "/")})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Size != out[b].Size {
			return out[a].Size > out[b].Size
		}
		return out[a].Members[0] < out[b].Members[0]
	})
	return out
}

func sharedIPCount(a, b []string) int {
	set := map[string]bool{}
	for _, x := range a {
		if x != "" {
			set[x] = true
		}
	}
	n := 0
	for _, y := range b {
		if set[y] {
			n++
		}
	}
	return n
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
