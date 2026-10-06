// §10.2 Review 队列存储（规格书 §8 P5-2 / §11.1 review_items 表）。
// 异常分超标（iForest 99.9 / MIDAS 5σ / 聚类 risk >0.5 / ML shadow 高）入队；
// 管理端批量处理（标注/封禁/忽略）。
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ReviewItemRow review_items 行。
type ReviewItemRow struct {
	ID         int64
	FP         string
	Reasons    []string // JSON 数组：anomaly_p999 / midas_5sigma / cluster_risk / ml_shadow_high
	Scores     map[string]float64
	Status     string // pending | done | ignored
	CreatedAt  time.Time
	ResolvedAt *time.Time
	ResolvedBy string
}

// AddReviewItem 入队一条 review 项（同 fp + pending 去重）。
func (db *DB) AddReviewItem(ctx context.Context, fp string, reasons []string, scores map[string]float64) error {
	// 同 fp 已有 pending 项 → 不重复入队
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM review_items WHERE fp = ? AND status = 'pending'", fp).Scan(&count); err != nil {
		return fmt.Errorf("查重: %w", err)
	}
	if count > 0 {
		return nil
	}
	rj, _ := json.Marshal(reasons)
	sj, _ := json.Marshal(scores)
	_, err := db.ExecContext(ctx,
		"INSERT INTO review_items (fp, reasons, scores, status, created_at) VALUES (?, ?, ?, 'pending', ?)",
		fp, string(rj), string(sj), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("入队 review: %w", err)
	}
	return nil
}

// ListReviewItems 按 status 查队列。
func (db *DB) ListReviewItems(ctx context.Context, status string, limit int) ([]ReviewItemRow, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT id, fp, reasons, scores, status, created_at, resolved_at, resolved_by FROM review_items"
	args := []any{}
	if status != "" {
		q += " WHERE status = ?"
		args = append(args, status)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询 review: %w", err)
	}
	defer rows.Close()
	out := []ReviewItemRow{}
	for rows.Next() {
		var r ReviewItemRow
		var rj, sj, created string
		var resolvedAt, resolvedBy sql.NullString
		if err := rows.Scan(&r.ID, &r.FP, &rj, &sj, &r.Status, &created, &resolvedAt, &resolvedBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rj), &r.Reasons)
		_ = json.Unmarshal([]byte(sj), &r.Scores)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if resolvedAt.Valid {
			t, _ := time.Parse(time.RFC3339, resolvedAt.String)
			r.ResolvedAt = &t
		}
		r.ResolvedBy = resolvedBy.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResolveReviewItems 批量处理（status → done/ignored）。
func (db *DB) ResolveReviewItems(ctx context.Context, ids []int64, action string, resolvedBy string) error {
	for _, id := range ids {
		if _, err := db.ExecContext(ctx,
			"UPDATE review_items SET status = ?, resolved_at = ?, resolved_by = ? WHERE id = ? AND status = 'pending'",
			action, time.Now().UTC().Format(time.RFC3339), resolvedBy, id); err != nil {
			return fmt.Errorf("处理 review %d: %w", id, err)
		}
	}
	return nil
}

// CountReviewItems pending 计数（管理端角标）。
func (db *DB) CountReviewItems(ctx context.Context, status string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM review_items WHERE status = ?", status).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
