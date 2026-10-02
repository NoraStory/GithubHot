package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/NoraStory/GithubHot/internal/domain/digest"
)

// DigestRepo 日报仓储实现。
type DigestRepo struct{ db *DB }

// NewDigestRepo 构造。
func NewDigestRepo(db *DB) *DigestRepo { return &DigestRepo{db: db} }

// Save 保存（同期号覆盖）。
func (r *DigestRepo) Save(ctx context.Context, d digest.Digest) error {
	stats, err := json.Marshal(d.Stats)
	if err != nil {
		return fmt.Errorf("序列化统计: %w", err)
	}
	if d.Kind == "" {
		d.Kind = digest.KindDaily
	}
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO digests (date, kind, markdown, stats, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(date) DO UPDATE SET kind = excluded.kind, markdown = excluded.markdown, stats = excluded.stats, created_at = excluded.created_at",
		d.Date, string(d.Kind), d.Markdown, string(stats), rfc(d.CreatedAt),
	)
	return err
}

// FindByDate 按期号取日报。
func (r *DigestRepo) FindByDate(ctx context.Context, date string) (*digest.Digest, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT date, COALESCE(kind, 'daily'), markdown, stats, created_at FROM digests WHERE date = ?", date)
	return scanDigest(row)
}

// Latest 取某类最新一期。
func (r *DigestRepo) Latest(ctx context.Context, kind digest.Kind) (*digest.Digest, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT date, COALESCE(kind, 'daily'), markdown, stats, created_at FROM digests WHERE COALESCE(kind, 'daily') = ? ORDER BY date DESC LIMIT 1", string(kind))
	d, err := scanDigest(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// List 按种类列出最近 N 期。
func (r *DigestRepo) List(ctx context.Context, kind digest.Kind, limit int) ([]digest.Digest, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT date, COALESCE(kind, 'daily'), markdown, stats, created_at FROM digests WHERE COALESCE(kind, 'daily') = ? ORDER BY date DESC LIMIT ?",
		string(kind), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []digest.Digest
	for rows.Next() {
		d, err := scanDigest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func scanDigest(rs rowScanner) (*digest.Digest, error) {
	var d digest.Digest
	var kind, stats, created string
	if err := rs.Scan(&d.Date, &kind, &d.Markdown, &stats, &created); err != nil {
		return nil, err
	}
	d.Kind = digest.Kind(kind)
	_ = json.Unmarshal([]byte(stats), &d.Stats)
	d.CreatedAt = parseTime(created)
	return &d, nil
}

// 接口满足性检查。
var _ digest.Repository = (*DigestRepo)(nil)
