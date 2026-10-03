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
	// SaveOverview 写入事件综述。
	SaveOverview(ctx context.Context, storyID string, overview string) error
	// SetManual 设置/取消人工锁定。
	SetManual(ctx context.Context, storyID string, manual bool) error
	// HotnessHistory 事件热度历史（升序，用于详情页）。
	HotnessHistory(ctx context.Context, storyID string, limit int) ([]HotnessPoint, error)
	// ListPage 全量事件分页（归档页用）：按首次收录时间降序，
	// 返回当页事件与总条数。kind 为空表示全部种类。
	ListPage(ctx context.Context, kind Kind, offset, limit int) ([]*Story, int, error)
}

// HotnessPoint 热度历史点。
type HotnessPoint struct {
	At      time.Time `json:"at"`
	Hotness float64   `json:"hotness"`
}
