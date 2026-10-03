package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AdminSession 管理端会话行。
type AdminSession struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
	IP        string
}

// CreateAdminSession 新建会话（顺带清理已过期会话）。
func (db *DB) CreateAdminSession(ctx context.Context, id, ip string, ttl time.Duration) error {
	now := time.Now()
	if _, err := db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE expires_at < ?", now.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("清理过期会话: %w", err)
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO admin_sessions (id, created_at, expires_at, ip) VALUES (?, ?, ?, ?)",
		id, now.UTC().Format(time.RFC3339), now.Add(ttl).UTC().Format(time.RFC3339), ip)
	if err != nil {
		return fmt.Errorf("写入会话: %w", err)
	}
	return nil
}

// FindAdminSession 按 ID 查会话；不存在返回 (nil, nil)。
func (db *DB) FindAdminSession(ctx context.Context, id string) (*AdminSession, error) {
	var s AdminSession
	var created, expires string
	err := db.QueryRowContext(ctx, "SELECT id, created_at, expires_at, ip FROM admin_sessions WHERE id = ?", id).
		Scan(&s.ID, &created, &expires, &s.IP)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询会话: %w", err)
	}
	s.CreatedAt, err = time.Parse(time.RFC3339, created)
	if err != nil {
		return nil, fmt.Errorf("解析创建时间: %w", err)
	}
	s.ExpiresAt, err = time.Parse(time.RFC3339, expires)
	if err != nil {
		return nil, fmt.Errorf("解析过期时间: %w", err)
	}
	return &s, nil
}

// RenewAdminSession 滑动续期（会话仍活跃时延长过期时间）。
func (db *DB) RenewAdminSession(ctx context.Context, id string, ttl time.Duration) error {
	_, err := db.ExecContext(ctx,
		"UPDATE admin_sessions SET expires_at = ? WHERE id = ?",
		time.Now().Add(ttl).UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("续期会话: %w", err)
	}
	return nil
}

// DeleteAdminSession 删除会话（退出登录）。
func (db *DB) DeleteAdminSession(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除会话: %w", err)
	}
	return nil
}
