package source

import (
	"context"
	"time"
)

// Repository 信源仓储端口（在领域层定义，基础设施层实现）。
type Repository interface {
	Save(ctx context.Context, s Source) error
	All(ctx context.Context) ([]Source, error)
	FindByID(ctx context.Context, id string) (Source, error)
	MarkFetched(ctx context.Context, id string, at time.Time) error
}

// AdaptiveRepository 自适应与管理的可选仓储扩展。
type AdaptiveRepository interface {
	// UpdateFetchStats 记录抓取结果并推进自适应间隔。
	UpdateFetchStats(ctx context.Context, id string, inserted, baseIntervalMinutes int, at time.Time) error
	// Delete 删除信源。
	Delete(ctx context.Context, id string) error
}
