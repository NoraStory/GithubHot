package sqlite

import (
	"context"

	"github.com/NoraStory/GithubHot/internal/application"
)

// MessageRepo 留言仓储。
type MessageRepo struct{ db *DB }

// NewMessageRepo 构造。
func NewMessageRepo(db *DB) *MessageRepo { return &MessageRepo{db: db} }

// List 最近 N 条（新在前）。
func (r *MessageRepo) List(ctx context.Context, limit int) ([]application.MessageRow, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, nickname, content, created_at FROM messages ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.MessageRow
	for rows.Next() {
		var m application.MessageRow
		var created string
		if err := rows.Scan(&m.ID, &m.Nickname, &m.Content, &created); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Add 新增留言。
func (r *MessageRepo) Add(ctx context.Context, m application.MessageRow) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO messages (nickname, content, ip_hash, created_at) VALUES (?, ?, ?, ?)",
		m.Nickname, m.Content, m.IPHash, rfc(m.CreatedAt))
	return err
}

var _ application.MessageRepo = (*MessageRepo)(nil)
