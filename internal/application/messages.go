package application

import (
	"context"
	"time"
)

// MessageRow 留言。
type MessageRow struct {
	ID        int64     `json:"id"`
	Nickname  string    `json:"nickname"`
	Content   string    `json:"content"`
	IPHash    string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// MessageRepo 留言仓储端口。
type MessageRepo interface {
	List(ctx context.Context, limit int) ([]MessageRow, error)
	Add(ctx context.Context, m MessageRow) error
}
