// P5-1 标注体系存储（规格书 §8 P5-1 / §11.1）。
// 弱标签（source=rule，每日 cron 生成）+ 金标签（source=admin，管理端人工）双轨；
// 训练导出时按 confidence 取每 fp 最高置信的标签。
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UpsertFpLabel 写入/覆盖一条标注（fp+source 主键）。
func (db *DB) UpsertFpLabel(ctx context.Context, fp, label, source string, confidence float64, notes string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO fp_labels (fp, label, source, confidence, labeled_at, notes)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(fp, source) DO UPDATE SET label = excluded.label,
		   confidence = excluded.confidence, labeled_at = excluded.labeled_at, notes = excluded.notes`,
		fp, label, source, confidence, time.Now().UTC().Format(time.RFC3339), notes)
	if err != nil {
		return fmt.Errorf("写标注: %w", err)
	}
	return nil
}

// FpLabelRow 一条标注。
type FpLabelRow struct {
	FP         string
	Label      string // human | bot | uncertain
	Source     string // admin | rule | model
	Confidence float64
	LabeledAt  time.Time
	Notes      string
}

// ListFpLabels 全量标注（ml export 用；规模 = 有标注的指纹数）。
func (db *DB) ListFpLabels(ctx context.Context) ([]FpLabelRow, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT fp, label, source, confidence, labeled_at, notes FROM fp_labels")
	if err != nil {
		return nil, fmt.Errorf("查询标注: %w", err)
	}
	defer rows.Close()
	out := []FpLabelRow{}
	for rows.Next() {
		var r FpLabelRow
		var at string
		if err := rows.Scan(&r.FP, &r.Label, &r.Source, &r.Confidence, &at, &r.Notes); err != nil {
			return nil, err
		}
		r.LabeledAt, _ = time.Parse(time.RFC3339, at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// BestFpLabel 每 fp 取最高置信标注（跨 source；训练导出用）。
// 返回 map[fp] = {label, confidence}；无标注的 fp 不在 map 中。
func (db *DB) BestFpLabels(ctx context.Context) (map[string]struct {
	Label      string
	Confidence float64
}, error) {
	rows, err := db.ListFpLabels(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]struct {
		Label      string
		Confidence float64
	}{}
	for _, r := range rows {
		if cur, ok := out[r.FP]; !ok || r.Confidence > cur.Confidence {
			out[r.FP] = struct {
				Label      string
				Confidence float64
			}{r.Label, r.Confidence}
		}
	}
	return out, nil
}

// DeleteFpRuleLabels 清空弱标签（每日 cron 重建前调用；admin 金标签保留）。
func (db *DB) DeleteFpRuleLabels(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM fp_labels WHERE source = 'rule'"); err != nil {
		return fmt.Errorf("清空弱标签: %w", err)
	}
	return nil
}

var _ = sql.ErrNoRows // 保留占位（未来按 fp 精查用）

// UpdateAnomalyScore 写回 iForest 异常分（P5-2）。
func (db *DB) UpdateAnomalyScore(ctx context.Context, fp string, score float64) error {
	_, err := db.ExecContext(ctx, "UPDATE ip_fingerprints SET anomaly_score = ? WHERE fp = ?", score, fp)
	if err != nil {
		return fmt.Errorf("写异常分: %w", err)
	}
	return nil
}
