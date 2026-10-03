package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// SearchResult 搜索结果行。
type SearchResult struct {
	TitleZh     string  `json:"titleZh"`
	URL         string  `json:"url"`
	SummaryZh   string  `json:"summaryZh"`
	Score       float64 `json:"score"`
	Hotness     float64 `json:"hotness"`
	SourceNames string  `json:"sourceNames"`
	Kind        string  `json:"kind"` // news / project
}

// Search 站内搜索：在已写作条目（中文标题/摘要/原标题）与活跃事件标题中
// 检索，按"事件优先、热度排序"合并返回。
func Search(ctx context.Context, d Deps, q string, limit int) ([]SearchResult, error) {
	q = strings.TrimSpace(q)
	if q == "" || limit <= 0 {
		return nil, nil
	}
	if limit > 50 {
		limit = 50
	}
	sourceNames, err := loadSourceNames(ctx, d)
	if err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, limit)
	seenURL := map[string]bool{}

	// 事件标题优先
	now := d.Clock.Now()
	stories, err := d.Stories.Active(ctx, now.Add(-7*24*time.Hour))
	if err == nil {
		for _, s := range stories {
			if s.Kind != story.KindNews {
				continue
			}
			if containsFold(s.TitleZh, q) || containsFold(s.SummaryZh, q) {
				if s.URL != "" {
					seenURL[s.URL] = true // 成员条目与事件同 URL 时去重
				}
				out = append(out, SearchResult{
					TitleZh: s.TitleZh, URL: s.URL, SummaryZh: s.SummaryZh,
					Hotness: s.Hotness, Kind: "story",
				})
			}
		}
	}

	items, err := d.Items.Search(ctx, q, limit)
	if err != nil {
		return out, nil
	}
	for _, it := range items {
		if seenURL[it.URL] {
			continue
		}
		seenURL[it.URL] = true
		out = append(out, SearchResult{
			TitleZh: it.Selection.TitleZh, URL: it.URL,
			SummaryZh: it.Selection.SummaryZh, Score: it.AverageScore(),
			SourceNames: sourceNames[it.SourceID], Kind: "item",
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Hotness > out[j].Hotness })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
