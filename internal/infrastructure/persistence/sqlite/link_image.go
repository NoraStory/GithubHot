package sqlite

import (
	"context"
	"time"
)

// GetLinkImage 查询链接封面缓存；无记录返回 found=false。
func (db *DB) GetLinkImage(ctx context.Context, url string) (image string, found bool, err error) {
	var fetchedAt string
	err = db.QueryRowContext(ctx, "SELECT image, fetched_at FROM link_images WHERE url = ?", url).
		Scan(&image, &fetchedAt)
	if err != nil {
		return "", false, nil // 无记录按未命中处理
	}
	return image, true, nil
}

// SetLinkImage 写入/刷新链接封面缓存（image 为空串表示已探测但无图，负缓存防重复抓取）。
func (db *DB) SetLinkImage(ctx context.Context, url, image string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO link_images (url, image, fetched_at) VALUES (?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET image = excluded.image, fetched_at = excluded.fetched_at`,
		url, image, time.Now().UTC().Format(time.RFC3339))
	return err
}
