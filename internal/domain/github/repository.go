package github

import (
	"context"
	"time"
)

// Repository 项目仓储端口。
type Repository interface {
	Upsert(ctx context.Context, p Project) error
	FindByFullName(ctx context.Context, fullName string) (*Project, error)
	All(ctx context.Context) ([]Project, error)
	AddSnapshot(ctx context.Context, s Snapshot) error
	// SnapshotsSince 取某项目自某时刻起的全部快照（升序）。
	SnapshotsSince(ctx context.Context, fullName string, since time.Time) ([]Snapshot, error)
	// AllSnapshotsSince 全部项目自某时刻起的快照，按项目分组。
	AllSnapshotsSince(ctx context.Context, since time.Time) (map[string][]Snapshot, error)
	// Touch 更新最近发现时间与 trending 排名等派生字段。
	Touch(ctx context.Context, p Project) error
	// SaveDescriptionZh 更新中文描述（翻译阶段回写）。
	SaveDescriptionZh(ctx context.Context, fullName, zh string) error
}
