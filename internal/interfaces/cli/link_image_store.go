package cli

import (
	"context"

	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
)

// linkImageStore 适配 sqlite 封面缓存到 httpapi.LinkImageCache 端口。
type linkImageStore struct{ db *sqlite.DB }

func (l linkImageStore) GetLinkImage(ctx context.Context, url string) (string, bool, error) {
	return l.db.GetLinkImage(ctx, url)
}

func (l linkImageStore) SetLinkImage(ctx context.Context, url, image string) error {
	return l.db.SetLinkImage(ctx, url, image)
}
