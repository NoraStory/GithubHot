package digest

import "context"

// Repository 日报仓储端口。
type Repository interface {
	Save(ctx context.Context, d Digest) error
	FindByDate(ctx context.Context, date string) (*Digest, error)
	// Latest 取某类（daily/weekly/monthly）最新一期。
	Latest(ctx context.Context, kind Kind) (*Digest, error)
	// List 按种类列出最近 N 期。
	List(ctx context.Context, kind Kind, limit int) ([]Digest, error)
}
