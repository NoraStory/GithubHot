// Package fpgraph — P6-2 Louvain 社区发现（规格书 §9 P6-2，gonum 纯 Go 实现）。
//
// 相对 P4-4 连通分量的增益：连通分量被桥接节点并成巨团；Louvain 按模块度分层，
// 能区分"核心团伙"与"边缘关联"。桥接单测：双团伙 + 单桥接节点 → Louvain 切成
// 两团而连通分量只有一团。
//
// 社区风险分（规格）：risk = ban成员占比 × min(1, size/10)；risk > 0.5 或模块度
// > 0.3 的社区入 review 关注（由引擎侧消费）。
package fpgraph

import (
	"math"
	"sort"

	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/community"
)

// Edge 加权无向边（fp 节点间；weight 为关联强度 0-1）。
type Edge struct {
	A, B   string
	Weight float64
}

// Community 一个社区（模块度分层后的成员 fp）。
type Community struct {
	Members []string
	Size    int
}

// Louvain 对加权无向图跑 Louvain 模块度社区发现。
// fpIDs：节点全量；edges：加权边（重复边取最大权重）。
// resolution=1 为标准模块度。
func Louvain(fpIDs []string, edges []Edge, resolution float64) []Community {
	g := simple.NewWeightedUndirectedGraph(0, math.Inf(1))
	idOf := map[string]int64{}
	for i, fp := range fpIDs {
		node := g.NewNode()
		g.AddNode(node)
		idOf[fp] = node.ID()
		_ = i
	}
	seen := map[[2]int64]float64{}
	for _, e := range edges {
		a, okA := idOf[e.A]
		b, okB := idOf[e.B]
		if !okA || !okB || a == b {
			continue
		}
		key := [2]int64{a, b}
		if a > b {
			key = [2]int64{b, a}
		}
		if w, ok := seen[key]; ok && w >= e.Weight {
			continue
		}
		seen[key] = e.Weight
		g.SetWeightedEdge(simple.WeightedEdge{F: g.Node(a), T: g.Node(b), W: e.Weight})
	}
	if g.Edges().Len() == 0 {
		// 无边：每个 fp 自成一社区（Louvain 需要边才能分层）
		out := make([]Community, 0, len(fpIDs))
		for _, fp := range fpIDs {
			out = append(out, Community{Members: []string{fp}, Size: 1})
		}
		return out
	}

	reduced := community.Modularize(g, resolution, nil)
	communities := reduced.Communities()

	out := make([]Community, 0, len(communities))
	for _, comm := range communities {
		members := make([]string, 0, len(comm))
		for _, n := range comm {
			for fp, id := range idOf {
				if id == n.ID() {
					members = append(members, fp)
					break
				}
			}
		}
		sort.Strings(members)
		if len(members) == 0 {
			continue
		}
		out = append(out, Community{Members: members, Size: len(members)})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Size != out[b].Size {
			return out[a].Size > out[b].Size
		}
		return out[a].Members[0] < out[b].Members[0]
	})
	return out
}


