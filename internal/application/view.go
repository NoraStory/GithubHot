package application

import (
	"context"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/github"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// 榜单规模：页面与 API 展示量；日报各取前 10。
const (
	boardSize       = 20
	historyLookBack = 6 * time.Hour
)

// BuildHotView 构建双榜视图模型（API、网页、日报共用同一份事实来源）。
// 徽章规则："新"= 24h 内首次发现；"上升"= 比约 6h 前热度高 15%+。
func BuildHotView(ctx context.Context, d Deps) (HotView, error) {
	view := HotView{Generated: d.Clock.Now()}
	now := view.Generated

	// ---------- GitHub 项目榜 ----------
	board, err := ProjectBoard(ctx, d, boardSize)
	if err != nil {
		return view, fmt.Errorf("构建项目榜: %w", err)
	}
	for i, row := range board {
		badges := projectBadges(row, now)
		view.GitHub = append(view.GitHub, ProjectRow{
			Rank:         i + 1,
			FullName:     row.Project.FullName,
			URL:          row.Project.HTMLURL,
			Description:  row.Project.Description,
			Language:     row.Project.Language,
			Topics:       row.Project.Topics,
			Stars:        row.Project.Stars,
			StarsGained:  github.StarsGainedIn(row.Snaps, now, 24*time.Hour),
			TrendingRank: row.Project.TrendingRank,
			Hotness:      row.Hotness,
			Badges:       badges,
		})
	}

	// ---------- AI 资讯榜 ----------
	stories, err := d.Stories.Active(ctx, now.Add(-48*time.Hour))
	if err != nil {
		return view, err
	}
	sourceNames, err := loadSourceNames(ctx, d)
	if err != nil {
		return view, err
	}
	var newsStories []*story.Story
	for _, s := range stories {
		if s.Kind == story.KindNews {
			newsStories = append(newsStories, s)
		}
	}
	story.SortByHotness(newsStories)
	if len(newsStories) > boardSize {
		newsStories = newsStories[:boardSize]
	}
	for i, s := range newsStories {
		view.News = append(view.News, buildStoryRow(ctx, d, s, i+1, sourceNames, now))
	}

	// ---------- 融合观察 ----------
	for _, s := range newsStories {
		if len(s.Projects) == 0 {
			continue
		}
		newsRow := view.News[storyIndexByID(view.News, s.ID)]
		for _, fn := range s.Projects {
			for _, row := range board {
				if row.Project.FullName != fn {
					continue
				}
				view.Fusion = append(view.Fusion, FusionRow{
					News:    newsRow,
					Project: projectRowOf(row, now, 0),
				})
			}
		}
	}
	return view, nil
}

func storyIndexByID(rows []StoryRow, id string) int {
	for i, r := range rows {
		if r.StoryID == id {
			return i
		}
	}
	return 0
}

func projectRowOf(row ProjectBoardRow, now time.Time, rank int) ProjectRow {
	return ProjectRow{
		Rank:         rank,
		FullName:     row.Project.FullName,
		URL:          row.Project.HTMLURL,
		Description:  row.Project.Description,
		Language:     row.Project.Language,
		Stars:        row.Project.Stars,
		StarsGained:  github.StarsGainedIn(row.Snaps, now, 24*time.Hour),
		TrendingRank: row.Project.TrendingRank,
		Hotness:      row.Hotness,
		Badges:       projectBadges(row, now),
	}
}

func projectBadges(row ProjectBoardRow, now time.Time) []string {
	var badges []string
	if now.Sub(row.Project.FirstSeenAt) < 24*time.Hour {
		badges = append(badges, "新")
	}
	if row.Project.TrendingRank > 0 && row.Project.TrendingRank <= 10 {
		badges = append(badges, fmt.Sprintf("trending #%d", row.Project.TrendingRank))
	}
	return badges
}

// buildStoryRow 组装资讯榜单行（含评分、徽章、来源名）。
func buildStoryRow(ctx context.Context, d Deps, s *story.Story, rank int, sourceNames map[string]string, now time.Time) StoryRow {
	row := StoryRow{
		Rank:      rank,
		StoryID:   s.ID,
		TitleZh:   s.TitleZh,
		SummaryZh: s.SummaryZh,
		URL:       s.URL,
		Hotness:   s.Hotness,
		Projects:  s.Projects,
	}
	seenSrc := map[string]bool{}
	for _, m := range s.Members {
		row.SourceCount++
		if m.SourceID != "" && !seenSrc[m.SourceID] {
			seenSrc[m.SourceID] = true
			row.SourceNames = append(row.SourceNames, sourceNames[m.SourceID])
		}
	}
	// 取成员条目里的最高均分与写作附件（理由、标签）
	var ids []string
	for _, m := range s.Members {
		if m.ItemID != "" {
			ids = append(ids, m.ItemID)
		}
	}
	if len(ids) > 0 {
		items, err := d.Items.FindByIDs(ctx, ids)
		if err == nil {
			best := -1.0
			for _, it := range items {
				avg := it.AverageScore()
				if avg > best {
					best = avg
				}
				if it.Selection.ReasonZh != "" && row.ReasonZh == "" {
					row.ReasonZh = it.Selection.ReasonZh
				}
				if len(it.Selection.Tags) > 0 && len(row.Tags) == 0 {
					row.Tags = it.Selection.Tags
				}
			}
			if best > 0 {
				row.Score = best
			}
		}
	}
	// 徽章
	if story.IsNew(s.FirstSeenAt, now) {
		row.Badges = append(row.Badges, "新")
	}
	if ref, ok, _ := d.Stories.HistoryNear(ctx, s.ID, now.Add(-historyLookBack), historyLookBack); ok {
		if story.Rising(s.Hotness, ref) {
			row.Badges = append(row.Badges, "上升")
		}
	}
	if len(s.Projects) > 0 {
		row.Badges = append(row.Badges, "GitHub关联")
	}
	return row
}

// loadSourceNames 信源 ID → 展示名。
func loadSourceNames(ctx context.Context, d Deps) (map[string]string, error) {
	all, err := d.Sources.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(all))
	for _, s := range all {
		out[s.ID] = s.Name
	}
	return out, nil
}
