// githubhot ml 子命令扩展（规格书 §9 P6-1/P6-3）：
//   ml export-graph --out data/ml/graph.jsonl   图快照（fp/ip/ua 节点 + 边）
//   ml import-gnn data/models/graph_gnn_v1.json  GNN 推理结果写回
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
)

// graphExportNode 图快照节点。
type graphExportNode struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // fp | ip | ua
	Features map[string]float64  `json:"features,omitempty"`
	Label    string   `json:"label,omitempty"`
}

// graphExportEdge 图快照边。
type graphExportEdge struct {
	Src    string  `json:"src"`
	Dst    string  `json:"dst"`
	Type   string  `json:"type"`
	Weight float64 `json:"weight"`
}

// MLExportGraph 图快照导出（P6-1）：fp/ip/ua 三类节点 + P4-4 四类边（30 天窗口）。
// 节点：fp 挂 16 维特征向量；ip / ua 为零特征连接子（结构信号经消息传递传播）。
// 边：member_of_ip（fp→ip）+ ua_of（fp→ua）+ P4-4 fp_links 四类。
func MLExportGraph(cfg *config.Config, out string) error {
	_, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	since := time.Now().Add(-30 * 24 * time.Hour)
	rows, err := db.ListFingerprintsSince(ctx, since, 100000)
	if err != nil {
		return err
	}
	labels, err := db.BestFpLabels(ctx)
	if err != nil {
		return err
	}
	if out == "" {
		out = filepath.Join(cfg.DataDir, "ml", "graph.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	// 单快照行：nodes + edges
	nodes := make([]graphExportNode, 0, len(rows))
	edges := make([]graphExportEdge, 0)
	fpSet := map[string]bool{}
	for _, row := range rows {
		fpID := "fp:" + row.Fingerprint
		fpSet[fpID] = true
		feats := mlFeaturesFromRow(row)
		label := ""
		if lb, has := labels[row.Fingerprint]; has {
			label = lb.Label
		}
		nodes = append(nodes, graphExportNode{ID: fpID, Type: "fp", Features: feats, Label: label})
		for _, ip := range row.IPs {
			ipID := "ip:" + ip
			if !fpSet[ipID] {
				fpSet[ipID] = true
				nodes = append(nodes, graphExportNode{ID: ipID, Type: "ip"})
			}
			edges = append(edges, graphExportEdge{Src: fpID, Dst: ipID, Type: "member_of_ip", Weight: 1.0})
		}
	}
	// 关联边（fp-fp）
	links, err := db.ListAllFPLinks(ctx, since, 200000)
	if err != nil {
		return err
	}
	for _, l := range links {
		src, dst := "fp:"+l.Src, "fp:"+l.Dst
		if !fpSet[src] || !fpSet[dst] {
			continue
		}
		kind := l.Kind
		if kind == "phash" || kind == "minhash" {
			kind = "similar"
		}
		edges = append(edges, graphExportEdge{Src: src, Dst: dst, Type: kind, Weight: l.Weight})
	}
	snapshot, _ := json.Marshal(map[string]any{
		"snapshot_ts": time.Now().UTC().Format(time.RFC3339),
		"nodes":       nodes,
		"edges":       edges,
	})
	if _, err := f.Write(append(snapshot, '\n')); err != nil {
		return err
	}
	fmt.Printf("[ml] 图快照 → %s（%d 节点 / %d 边 / 30 天窗口）\n", out, len(nodes), len(edges))
	return nil
}

// MLImportGNN GNN 推理结果写回（P6-3）：读 data/models/graph_gnn_v1.json，
// 每 fp 写回 gnn_score（bot 概率）与 gnn_embedding（64 维 JSON）。
func MLImportGNN(cfg *config.Config, path string) error {
	_, db, err := build(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读 GNN 结果失败: %w", err)
	}
	var card struct {
		Version    string           `json:"version"`
		Scores     map[string]float64 `json:"scores"`
		Embeddings map[string][]float64 `json:"embeddings"`
		TrainedAt  string           `json:"trained_at"`
	}
	if err := json.Unmarshal(raw, &card); err != nil {
		return fmt.Errorf("GNN JSON 解析失败: %w", err)
	}
	rows := make([]sqlite.GNNResultRow, 0, len(card.Scores))
	for fpID, score := range card.Scores {
		fp := strings.TrimPrefix(fpID, "fp:")
		emb := card.Embeddings[fpID]
		rows = append(rows, sqlite.GNNResultRow{FP: fp, Score: score, Embedding: emb})
	}
	n, err := db.ImportGNNResults(ctx, rows)
	if err != nil {
		return err
	}
	fmt.Printf("[ml] GNN 结果写回 %d / %d fp（模型 %s，训练于 %s）\n",
		n, len(card.Scores), card.Version, card.TrainedAt)
	return nil
}
