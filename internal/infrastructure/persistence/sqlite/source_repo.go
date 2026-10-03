package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// SourceRepo 信源仓储实现。
type SourceRepo struct{ db *DB }

// NewSourceRepo 构造。
func NewSourceRepo(db *DB) *SourceRepo { return &SourceRepo{db: db} }

// Save 新增或更新信源（全字段覆盖）。
func (r *SourceRepo) Save(ctx context.Context, s source.Source) error {
	cfg, _ := json.Marshal(s.Config)
	tags, _ := json.Marshal(s.Tags)
	lastFetched := ""
	if s.LastFetchedAt != nil {
		lastFetched = rfc(*s.LastFetchedAt)
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO sources (id, name, kind, config, tier, tags, interval_minutes, enabled, created_at, last_fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET name = excluded.name, kind = excluded.kind, config = excluded.config, tier = excluded.tier, tags = excluded.tags, interval_minutes = excluded.interval_minutes, enabled = excluded.enabled, last_fetched_at = CASE WHEN excluded.last_fetched_at != '' THEN excluded.last_fetched_at ELSE sources.last_fetched_at END",
		s.ID, s.Name, string(s.Kind), string(cfg), string(s.Tier), string(tags), s.IntervalMinutes, boolInt(s.Enabled), rfc(s.CreatedAt), lastFetched,
	)
	if err != nil {
		return fmt.Errorf("保存信源 %s: %w", s.ID, err)
	}
	return nil
}

// All 全部信源。
func (r *SourceRepo) All(ctx context.Context) ([]source.Source, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, kind, config, tier, tags, interval_minutes, enabled, created_at, last_fetched_at, COALESCE(current_interval_minutes, 0), COALESCE(empty_streak, 0) FROM sources ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("查询信源: %w", err)
	}
	defer rows.Close()
	var out []source.Source
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FindByID 按 ID 查信源，不存在返回 sql.ErrNoRows。
func (r *SourceRepo) FindByID(ctx context.Context, id string) (source.Source, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, kind, config, tier, tags, interval_minutes, enabled, created_at, last_fetched_at, COALESCE(current_interval_minutes, 0), COALESCE(empty_streak, 0) FROM sources WHERE id = ?", id)
	return scanSource(row)
}

// MarkFetched 记录抓取时间。
func (r *SourceRepo) MarkFetched(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, "UPDATE sources SET last_fetched_at = ? WHERE id = ?", rfc(at), id)
	return err
}

// UpdateFetchStats 记录抓取结果并推进自适应间隔。
func (r *SourceRepo) UpdateFetchStats(ctx context.Context, id string, inserted, baseIntervalMinutes int, at time.Time) error {
	interval, streak := source.ComputeAdaptive(baseIntervalMinutes, 0, inserted)
	// 读当前 streak 推进（单连接串行化下无竞态）
	var cur int
	if err := r.db.QueryRowContext(ctx, "SELECT COALESCE(empty_streak, 0) FROM sources WHERE id = ?", id).Scan(&cur); err == nil {
		interval, streak = source.ComputeAdaptive(baseIntervalMinutes, cur, inserted)
	}
	if _, err := r.db.ExecContext(ctx,
		"UPDATE sources SET last_fetched_at = ?, current_interval_minutes = ?, empty_streak = ? WHERE id = ?",
		rfc(at), interval, streak, id); err != nil {
		return err
	}
	return nil
}

// Delete 删除信源。
func (r *SourceRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sources WHERE id = ?", id)
	return err
}

type rowScanner interface{ Scan(dest ...any) error }

func scanSource(rs rowScanner) (source.Source, error) {
	var s source.Source
	var kind, tier, cfg, tags string
	var enabled, curInterval, emptyStreak int
	var created, lastFetched string
	if err := rs.Scan(&s.ID, &s.Name, &kind, &cfg, &tier, &tags, &s.IntervalMinutes, &enabled, &created, &lastFetched, &curInterval, &emptyStreak); err != nil {
		return s, err
	}
	s.CurrentIntervalMinutes = curInterval
	s.EmptyStreak = emptyStreak
	s.Kind = source.Kind(kind)
	s.Tier = source.Tier(tier)
	s.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(cfg), &s.Config)
	_ = json.Unmarshal([]byte(tags), &s.Tags)
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	if lastFetched != "" {
		if t, err := time.Parse(time.RFC3339, lastFetched); err == nil {
			s.LastFetchedAt = &t
		}
	}
	return s, nil
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// 接口满足性检查。
var _ source.Repository = (*SourceRepo)(nil)
var _ source.AdaptiveRepository = (*SourceRepo)(nil)

var _ = sql.ErrNoRows
