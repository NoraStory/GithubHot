package digest

import "context"

// Repository 日报仓储端口。
type Repository interface {
	Save(ctx context.Context, d Digest) error
	FindByDate(ctx context.Context, date string) (*Digest, error)
	Latest(ctx context.Context) (*Digest, error)
	List(ctx context.Context, limit int) ([]Digest, error)
}
