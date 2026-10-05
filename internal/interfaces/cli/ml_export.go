// githubhot ml 子命令（规格书 §8 P5-1）：训练数据导出与模型装载检查。
//   ml export --out data/ml/behavior.jsonl  每行 {fp, features{16 项}, label, weight}
//   ml check                                校验行为模型文件是否达标（gate 判定透传）
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
	"github.com/NoraStory/GithubHot/internal/domain/behavior"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
)

// MLExport 训练数据导出（P5-1）：近 30 天指纹 × 特征向量 × 最高置信标注。
// 输出行与 scripts/ml/train_behavior.py 的 load_jsonl 契约严格对齐：
//   {"fp": "...", "features": {16 项}, "label": "human|bot", "weight": 0.6-1.0, "ts": ...}
// uncertainty（无标注）行不输出——监督训练忽略它们（规格 §8 P5-3）。
func MLExport(cfg *config.Config, out string) error {
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
		out = filepath.Join(cfg.DataDir, "ml", "behavior.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	humanN, botN := 0, 0
	for _, row := range rows {
		lb, has := labels[row.Fingerprint]
		if !has || (lb.Label != "human" && lb.Label != "bot") {
			continue // uncertain 不导出
		}
		feats := mlFeaturesFromRow(row)
		if feats == nil {
			continue
		}
		line, _ := json.Marshal(map[string]any{
			"fp":       row.Fingerprint,
			"features": feats,
			"label":    lb.Label,
			"weight":   lb.Confidence,
			"ts":       row.LastSeen.UTC().Format(time.RFC3339),
		})
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
		if lb.Label == "human" {
			humanN++
		} else {
			botN++
		}
	}
	fmt.Printf("[ml] 导出 %s：human %d / bot %d（窗口 30 天，标注数 %d）\n",
		out, humanN, botN, len(labels))
	return nil
}

// MLCheck 校验行为模型文件（gate 透传）：达标 → exit 0；未达标 → exit 1。
func MLCheck(cfg *config.Config) error {
	path := filepath.Join(cfg.DataDir, "models", "behavior_lr_v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("模型文件不存在 %s（先运行 scripts/ml/run_train.ps1）", path)
	}
	var m struct {
		Version string `json:"version"`
		Metrics struct {
			AUC float64 `json:"auc"`
			FPR float64 `json:"fpr"`
		} `json:"metrics"`
		Active bool `json:"active"`
		Gate   struct {
			Pass bool `json:"pass"`
		} `json:"gate"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("模型 JSON 解析失败: %w", err)
	}
	fmt.Printf("[ml] 模型 %s：AUC=%.3f FPR=%.3f active=%v gate_pass=%v\n",
		m.Version, m.Metrics.AUC, m.Metrics.FPR, m.Active, m.Gate.Pass)
	if !m.Gate.Pass {
		return fmt.Errorf("模型未达出门槛（AUC≥0.85 且 FPR≤0.5%%），不应启用")
	}
	return nil
}

// mlFeaturesFromRow 指纹行 → 16 维特征 map（键名与 train_behavior.FEATURE_NAMES 严格一致）。
func mlFeaturesFromRow(row sqlite.FingerprintRow) map[string]float64 {
	f := behavior.Parse(row.BehaviorJSON)
	if f == nil {
		return nil
	}
	flagsHit := 0.0
	for _, flag := range row.Flags {
		if strings.HasPrefix(flag, "fpb_") || strings.HasPrefix(flag, "botd_") {
			flagsHit++
		}
	}
	skew := 0.0
	if row.ClockSkewPPM != nil {
		skew = *row.ClockSkewPPM
	}
	sessionMinutes := row.LastSeen.Sub(row.FirstSeen).Minutes()
	if sessionMinutes < 0 {
		sessionMinutes = 0
	}
	return map[string]float64{
		"dwell_mean":       f.Keys.DwellMean,
		"dwell_var":        f.Keys.DwellVar,
		"flight_mean":      f.Keys.FlightMean,
		"flight_var":       f.Keys.FlightVar,
		"speed_mean":       f.Mouse.SpeedMean,
		"speed_var":        f.Mouse.SpeedVar,
		"curvature_mean":   f.Mouse.CurvatureMean,
		"jerk_var":         f.Mouse.JerkVar,
		"dir_change_rate":  f.Mouse.DirChangeRate,
		"event_count":      float64(f.Mouse.Events + f.Keys.Events),
		"entropy_bits":     row.EntropyBits,
		"stability":        row.Stability,
		"clock_skew_ppm":   skew,
		"flags_hit":        flagsHit,
		"session_minutes":  sessionMinutes,
		"ip_count_30d":     float64(len(row.IPs)),
	}
}
