package sqlite

import (
	"context"
	"fmt"
	"time"
)

// probeKeepPerTarget 每个探针目标在库中保留的历史条数。
const probeKeepPerTarget = 20

// ProbeRecord 一次探测结果行。
type ProbeRecord struct {
	ID        int64
	Target    string
	Kind      string // source | endpoint
	OK        bool
	LatencyMS int64
	Detail    string
	CheckedAt string // RFC3339 UTC
}

// RecordProbe 记录一次探测结果，并裁剪该目标的历史（每目标保留最近 N 条）。
func (db *DB) RecordProbe(target, kind string, ok bool, latency time.Duration, detail string, at time.Time) error {
	if _, err := db.Exec(
		"INSERT INTO probe_results (target, kind, ok, latency_ms, detail, checked_at) VALUES (?, ?, ?, ?, ?, ?)",
		target, kind, ok, latency.Milliseconds(), detail, at.UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM probe_results WHERE target = ? AND id NOT IN (
		SELECT id FROM probe_results WHERE target = ? ORDER BY id DESC LIMIT ?)`, target, target, probeKeepPerTarget)
	return err
}

// LatestProbes 每个探针目标的最近一条结果（新在前，按目标名排序稳定输出）。
func (db *DB) LatestProbes(ctx context.Context) ([]ProbeRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.id, r.target, r.kind, r.ok, r.latency_ms, r.detail, r.checked_at
		FROM probe_results r
		JOIN (SELECT target, MAX(id) AS max_id FROM probe_results GROUP BY target) t
		  ON t.target = r.target AND t.max_id = r.id
		ORDER BY r.target`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProbes(rows)
}

// ProbeHistory 指定目标的最近 N 条探测历史（新在前）。
func (db *DB) ProbeHistory(ctx context.Context, target string, limit int) ([]ProbeRecord, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, target, kind, ok, latency_ms, detail, checked_at FROM probe_results WHERE target = ? ORDER BY id DESC LIMIT ?",
		target, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProbes(rows)
}

func scanProbes(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]ProbeRecord, error) {
	var out []ProbeRecord
	for rows.Next() {
		var r ProbeRecord
		var ok int
		if err := rows.Scan(&r.ID, &r.Target, &r.Kind, &ok, &r.LatencyMS, &r.Detail, &r.CheckedAt); err != nil {
			return nil, fmt.Errorf("扫描探测记录: %w", err)
		}
		r.OK = ok != 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
