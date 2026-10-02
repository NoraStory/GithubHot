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

// Save 保存（同日覆盖）。
func (r *DigestRepo) Save(ctx context.Context, d digest.Digest) error {
	stats, err := json.Marshal(d.Stats)
	if err != nil {
		return fmt.Errorf("序列化统计: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO digests (date, markdown, stats, created_at) VALUES (?, ?, ?, ?) ON CONFLICT(date) DO UPDATE SET markdown = excluded.markdown, stats = excluded.stats, created_at = excluded.created_at",
		d.Date, d.Markdown, string(stats), rfc(d.CreatedAt),
	)
	return err
}

// FindByDate 按日期取日报。
func (r *DigestRepo) FindByDate(ctx context.Context, date string) (*digest.Digest, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT date, markdown, stats, created_at FROM digests WHERE date = ?", date)
	return scanDigest(row)
}

// Latest 最新日报。
func (r *DigestRepo) Latest(ctx context.Context) (*digest.Digest, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT date, markdown, stats, created_at FROM digests ORDER BY date DESC LIMIT 1")
	d, err := scanDigest(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// List 最近 N 份日报。
func (r *DigestRepo) List(ctx context.Context, limit int) ([]digest.Digest, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT date, markdown, stats, created_at FROM digests ORDER BY date DESC LIMIT ?", limit)
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
	var stats, created string
	if err := rs.Scan(&d.Date, &d.Markdown, &stats, &created); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(stats), &d.Stats)
	d.CreatedAt = parseTime(created)
	return &d, nil
}

// 接口满足性检查。
var _ digest.Repository = (*DigestRepo)(nil)
