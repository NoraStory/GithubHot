package sqlite

import (
	"context"
	"time"
)

// SettingsRepo 站点设置与每日国内热榜摘要的 SQLite 实现。
type SettingsRepo struct {
	db *DB
}

// NewSettingsRepo 构造。
func NewSettingsRepo(db *DB) *SettingsRepo { return &SettingsRepo{db: db} }

// Get 读设置值，不存在返回空串。
func (r *SettingsRepo) Get(_ context.Context, key string) (string, error) {
	var v string
	err := r.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err != nil {
		return "", nil // 不存在按空串处理
	}
	return v, nil
}

// Set 写设置值（覆盖）。
func (r *SettingsRepo) Set(_ context.Context, key, value string) error {
	_, err := r.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value)
	return err
}

// DomesticSummary 读某日摘要，不存在返回空串。
func (r *SettingsRepo) DomesticSummary(_ context.Context, date string) (string, error) {
	var v string
	err := r.db.QueryRow("SELECT summary FROM domestic_summaries WHERE date = ?", date).Scan(&v)
	if err != nil {
		return "", nil
	}
	return v, nil
}

// SaveDomesticSummary 写某日摘要（覆盖）。
func (r *SettingsRepo) SaveDomesticSummary(_ context.Context, date, summary string) error {
	_, err := r.db.Exec(
		"INSERT INTO domestic_summaries (date, summary, created_at) VALUES (?, ?, ?) ON CONFLICT(date) DO UPDATE SET summary = excluded.summary, created_at = excluded.created_at",
		date, summary, time.Now().UTC().Format(time.RFC3339))
	return err
}
