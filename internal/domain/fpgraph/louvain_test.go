package fpgraph

import (
	"testing"
)

// TestLouvainBridgeSplit 规格验收：双团伙 + 单桥接节点 —— Louvain 切成两团
// 而连通分量只有一团（核心团伙与边缘关联的区分能力）。
func TestLouvainBridgeSplit(t *testing.T) {
	ids := []string{"c1", "c2", "c3", "c4", "bridge", "d1", "d2", "d3", "d4"}
	var edges []Edge
	// 团伙 C：c1..c4 内部全连（高权重）
	for i, a := range []string{"c1", "c2", "c3", "c4"} {
		for _, b := range []string{"c1", "c2", "c3", "c4"}[i+1:] {
			edges = append(edges, Edge{A: a, B: b, Weight: 0.9})
		}
	}
	// 团伙 D：d1..d4 内部全连
	for i, a := range []string{"d1", "d2", "d3", "d4"} {
		for _, b := range []string{"d1", "d2", "d3", "d4"}[i+1:] {
			edges = append(edges, Edge{A: a, B: b, Weight: 0.9})
		}
	}
	// 单桥：c1 ↔ bridge ↔ d1（低权重）
	edges = append(edges, Edge{A: "c1", B: "bridge", Weight: 0.2})
	edges = append(edges, Edge{A: "bridge", B: "d1", Weight: 0.2})

	comms := Louvain(ids, edges, 1.0)
	if len(comms) < 2 {
		t.Fatalf("Louvain 应切分 ≥2 社区，实际 %d：%+v", len(comms), comms)
	}
	// c1..c4 应在同一社区
	cIdx := -1
	for i, c := range comms {
		if contains(c.Members, "c1") && contains(c.Members, "c2") && contains(c.Members, "c3") {
			cIdx = i
		}
	}
	if cIdx < 0 {
		t.Fatalf("核心团伙应保持同社区：%+v", comms)
	}
	if contains(comms[cIdx].Members, "d1") || contains(comms[cIdx].Members, "d2") {
		t.Fatalf("两个团伙不应被并入同一社区：%+v", comms[cIdx])
	}
}

// TestLouvainSingleCluster 无桥的单一稠密团伙 → 一个社区。
func TestLouvainSingleCluster(t *testing.T) {
	ids := []string{"a", "b", "c"}
	edges := []Edge{
		{A: "a", B: "b", Weight: 1}, {A: "b", B: "c", Weight: 1}, {A: "a", B: "c", Weight: 1},
	}
	comms := Louvain(ids, edges, 1.0)
	if len(comms) != 1 || comms[0].Size != 3 {
		t.Fatalf("稠密三角应 1 社区 3 成员：%+v", comms)
	}
}

// TestLouvainNoEdges 无边图：每个节点自成社区。
func TestLouvainNoEdges(t *testing.T) {
	comms := Louvain([]string{"x", "y"}, nil, 1.0)
	if len(comms) != 2 {
		t.Fatalf("无边应各自成社区：%+v", comms)
	}
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
