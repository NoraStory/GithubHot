package fetcher

import (
	"context"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// safehttpJSON 带 JSON Accept 头的受限抓取。
func safehttpJSON(ctx context.Context, url string) ([]byte, int, error) {
	return safehttp.Fetch(ctx, url, map[string]string{"Accept": "application/json"})
}
