package item

import (
	"context"
	"time"
)

// Repository 原始资料仓储端口。
type Repository interface {
	// Upsert 按 ID 判重插入；已存在时返回 inserted=false 且不覆盖。
	Upsert(ctx context.Context, it Item) (inserted bool, err error)
	// CountSince 统计某时间之后采集的条数。
	CountSince(ctx context.Context, since time.Time) (int, error)
	// ByStage 按阶段取一批条目（按发布时间倒序）。
	ByStage(ctx context.Context, stages []Stage, limit int) ([]Item, error)
	// Recent 最近采集的条目，用于预筛批次与调试。
	Recent(ctx context.Context, limit int) ([]Item, error)
	// UpdateSelection 写回精选结果。
	UpdateSelection(ctx context.Context, id string, sel Selection) error
	// FindByIDs 批量取条目。
	FindByIDs(ctx context.Context, ids []string) ([]Item, error)
	// Search 在已写作条目中按关键词检索（中文标题/摘要/原标题）。
	Search(ctx context.Context, q string, limit int) ([]Item, error)
	// PendingContentZh 已写作但还没有本地中文译文的条目（有正文或摘要可译）。
	PendingContentZh(ctx context.Context, limit int) ([]Item, error)
	// SaveContentZh 写入正文/摘要的中文译文（本地存档，幂等可重复调用）。
	SaveContentZh(ctx context.Context, id string, zh string) error
}
