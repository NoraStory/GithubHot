package story

import (
	"context"
	"time"
)

// Repository 事件仓储端口。
type Repository interface {
	Save(ctx context.Context, s *Story) error
	FindByID(ctx context.Context, id string) (*Story, error)
	// Active 窗口期内的全部事件（news + project）。
	Active(ctx context.Context, since time.Time) ([]*Story, error)
	Delete(ctx context.Context, id string) error
	// AddHistory 记录一次热度快照，用于"上升"标记。
	AddHistory(ctx context.Context, storyID string, at time.Time, hotness float64) error
	// HistoryAt 取事件在 [at-窗口, at] 内最接近 at 的热度记录（用于上升对比）。
	HistoryNear(ctx context.Context, storyID string, at time.Time, lookBack time.Duration) (float64, bool, error)
	// LinkProjects 建立事件与 GitHub 项目的融合链接。
	LinkProjects(ctx context.Context, storyID string, fullNames []string) error
}
