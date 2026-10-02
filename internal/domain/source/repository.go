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
