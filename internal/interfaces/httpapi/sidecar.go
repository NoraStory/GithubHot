// P6-3b GNN sidecar 主服务集成（规格书 §9 P6-3b）+ P6-6 融合规则。
//
// sidecar 集成：
//   - env GNN_SIDECAR_URL（默认空 = 纯离线模式）+ GNN_SIDECAR_TOKEN
//   - fp/report 新指纹入库后 goroutine 异步调用（fire-and-forget，500ms 超时，不阻塞上报）
//   - 失败/超时/未配置 → 回落 P6-3 冷启动（Louvain 社区均值）
//   - 结果写回 gnn_score / gnn_embedding
//
// 融合规则（P6-6）：
//   - behavior_ml_score 与 gnn_score 皮尔逊相关 < 0.6（正交性检查，违规回炉）
//   - 两者均 > 0.8 → severe（三层积分通道）；单项超 → review 关注 + 低权计分
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/behavior"
	"github.com/NoraStory/GithubHot/internal/infrastructure/sidecarclient"
)

// sidecarEnabled 是否配置了 GNN sidecar（GNN_SIDECAR_URL 非空）。
func sidecarEnabled() bool {
	return strings.TrimSpace(os.Getenv("GNN_SIDECAR_URL")) != ""
}

// sidecarClient 惰性创建 sidecar 客户端（env 变化感知）。
type sidecarHolder struct {
	mu     sync.Mutex
	client *sidecarclient.Client
}

func (s *Server) getSidecarClient() *sidecarclient.Client {
	s.sidecarOnce.Do(func() {
		url := strings.TrimSpace(os.Getenv("GNN_SIDECAR_URL"))
		token := strings.TrimSpace(os.Getenv("GNN_SIDECAR_TOKEN"))
		if url != "" {
			s.sidecarClient = sidecarclient.New(url, token, 500*time.Millisecond)
			log.Printf("[INFO] GNN sidecar已配置: %s", url)
		}
	})
	return s.sidecarClient
}

// SidecarHealthStatus 暴露sidecar健康状态（用于§10管理端诊断页）。
func (s *Server) SidecarHealthStatus() map[string]interface{} {
	client := s.getSidecarClient()
	if client == nil {
		return map[string]interface{}{"enabled": false}
	}
	return client.HealthStatus()
}

// gnnSidecarScore 异步调 sidecar 打分（fire-and-forget，不阻塞 fp/report）。
// 成功 → 写回 gnn_score/gnn_embedding；失败/超时/未配置 → 回落冷启动 Louvain 均值。
func (s *Server) gnnSidecarScore(ctx context.Context, ip, fp string) {
	client := s.getSidecarClient()
	if client == nil || !client.Enabled() || s.Guard == nil || s.Guard.store == nil {
		return
	}
	goSafe("gnn-sidecar-score", func() {
		gctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		// 构建子图：目标 fp + 其关联边
		row, err := s.Guard.store.FindFingerprint(gctx, fp)
		if err != nil || row == nil {
			return
		}
		feats := featureVectorFor(*row)
		if feats == nil {
			return
		}
		nodes := []sidecarclient.Node{{ID: fp, Features: toFloat32(feats)}}
		var edges [][2]int
		links, err := s.Guard.store.ListFPLinks(gctx, fp, 10)
		if err == nil {
			for i, l := range links {
				other := l.Dst
				if other == fp {
					other = l.Src
				}
				if other == fp {
					continue
				}
				otherRow, err := s.Guard.store.FindFingerprint(gctx, other)
				if err != nil || otherRow == nil {
					continue
				}
				otherFeats := featureVectorFor(*otherRow)
				if otherFeats == nil {
					continue
				}
				nodes = append(nodes, sidecarclient.Node{ID: other, Features: toFloat32(otherFeats)})
				edges = append(edges, [2]int{0, i + 1}, [2]int{i + 1, 0})
			}
		}
		result, err := client.Score(gctx, sidecarclient.ScoreRequest{Nodes: nodes, Edges: edges})
		if err != nil {
			// 回落：Louvain 社区均值（冷启动路径，规格书 P6-3）
			if clusterScore := s.Guard.communityMeanScore(gctx, fp); clusterScore > 0 {
				_ = s.Guard.store.UpdateGNN(gctx, fp, clusterScore, "[]")
			}
			return
		}
		if score, ok := result.Scores[fp]; ok {
			emb, _ := json.Marshal(result.Embeddings[fp])
			_ = s.Guard.store.UpdateGNN(gctx, fp, float64(score), string(emb))
			// P6-6 融合：behavior_ml_score 与 gnn_score 双高检查
			s.fusionCheck(gctx, ip, fp, float64(score))
		}
	})
}

// fusionCheck P6-6 融合规则：behavior_ml_score 与 gnn_score 正交性与双高判定。
//   - 双高（均 ≥ 0.8）→ severe 违规（三层积分通道，非 severe=false 的 100 分）；
//   - 单高（任一 ≥ 0.8）→ 0 分 review 关注（不直接计分，由运维确认）；
//   - 两者均低 → 不做任何操作。
//
// 正交性检查（皮尔逊 < 0.6）由每日 cron 的 ml diag 批量执行，此处做单点融合判定。
func (s *Server) fusionCheck(ctx context.Context, ip, fp string, gnnScore float64) {
	if s.Guard == nil || s.Guard.store == nil {
		return
	}
	// 读 behavior_ml_score（LR 模型打分结果，由 scoreBehaviorML 异步写入 events）
	// 简化：此处只检查 gnn_score 单点 + LR 模型实时打分
	row, err := s.Guard.store.FindFingerprint(ctx, fp)
	if err != nil || row == nil {
		return
	}
	f := behavior.Parse(row.BehaviorJSON)
	if f == nil {
		return
	}
	m := s.Guard.mlEngine.GetLRModel()
	if m == nil || !m.Active {
		return
	}
	mlScore := m.Predict(featureVectorFor(*row))
	if mlScore < 0 {
		return
	}
	bothHigh := gnnScore >= 0.8 && mlScore >= 0.8
	oneHigh := gnnScore >= 0.8 || mlScore >= 0.8
	if bothHigh {
		// 双高 → severe（走三层积分，非 severe=false 的即时封禁——iprisk 多证据判定）
		s.Guard.Event(ctx, ip, "fusion-severe",
			sprintf("融合双高：behavior_ml %.3f + gnn %.3f", mlScore, gnnScore), 50, false)
	} else if oneHigh {
		// 单高 → review 关注（0 分，运维确认后人工标注）
		which := "gnn"
		if mlScore >= 0.8 {
			which = "behavior_ml"
		}
		s.Guard.Event(ctx, ip, "fusion-review",
			sprintf("融合单高：%s %.3f（review 关注）", which,
				map[bool]float64{true: mlScore, false: gnnScore}[which == "behavior_ml"]), 0, false)
	}
}

// communityMeanScore Louvain 社区均值（P6-3 冷启动路径）：取 fp 所在社区内
// 已有 gnn_score 的节点均值；无社区或无均值 → 0（不写回）。
func (g *IPGuard) communityMeanScore(ctx context.Context, fp string) float64 {
	if g.store == nil {
		return 0
	}
	clusters, err := g.store.ListClusters(ctx, 100)
	if err != nil {
		return 0
	}
	for _, c := range clusters {
		hasFP := false
		for _, m := range c.Members {
			if m == fp {
				hasFP = true
				break
			}
		}
		if !hasFP {
			continue
		}
		// 找社区内其他成员的 gnn_score 均值
		sum, n := 0.0, 0
		for _, m := range c.Members {
			if m == fp {
				continue
			}
			row, err := g.store.FindFingerprint(ctx, m)
			if err != nil || row == nil || row.GNNScore == nil {
				continue
			}
			sum += *row.GNNScore
			n++
		}
		if n == 0 {
			continue
		}
		return sum / float64(n)
	}
	return 0
}

func toFloat32(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

// CheckOrthogonality P6-6 皮尔逊正交性检查（每日 cron）：behavior_ml_score 与
// gnn_score 的皮尔逊相关系数 < 0.6 才通过；≥ 0.6 说明特征泄漏需回炉重做。
func (g *IPGuard) CheckOrthogonality(ctx context.Context) error {
	if g.store == nil || g.mlEngine == nil {
		return nil
	}
	m := g.mlEngine.GetLRModel()
	if m == nil || !m.Active {
		return nil
	}
	rows, err := g.store.ListFingerprintsSince(ctx, time.Now().Add(-30*24*time.Hour), entropyMaxRows)
	if err != nil {
		return err
	}
	var mlScores, gnnScores []float64
	for _, r := range rows {
		if r.GNNScore == nil {
			continue
		}
		f := behavior.Parse(r.BehaviorJSON)
		if f == nil {
			continue
		}
		ml := m.Predict(featureVectorFor(r))
		if ml < 0 {
			continue
		}
		mlScores = append(mlScores, ml)
		gnnScores = append(gnnScores, *r.GNNScore)
	}
	if len(mlScores) < 10 {
		return nil // 样本不足
	}
	// 皮尔逊相关系数
	var sumXY, sumX2, sumY2, sumX, sumY float64
	n := float64(len(mlScores))
	for i := range mlScores {
		sumXY += mlScores[i] * gnnScores[i]
		sumX += mlScores[i]
		sumY += gnnScores[i]
		sumX2 += mlScores[i] * mlScores[i]
		sumY2 += gnnScores[i] * gnnScores[i]
	}
	denomX := n*sumX2 - sumX*sumX
	denomY := n*sumY2 - sumY*sumY
	if denomX <= 0 || denomY <= 0 {
		return nil
	}
	corr := (n*sumXY - sumX*sumY) / math.Sqrt(denomX*denomY)
	abs := math.Abs(corr)
	if abs >= 0.6 {
		log.Printf("[ml] ⚠ 正交性违规：behavior_ml 与 gnn 皮尔逊相关 %.3f ≥ 0.6（特征泄漏，需回炉重做）", corr)
	} else {
		log.Printf("[ml] 正交性 OK：皮尔逊相关 %.3f（< 0.6）", corr)
	}
	return nil
}
