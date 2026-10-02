package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/domain/prompts"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// BuildDigest 日报/周报/月报：按 kind 取窗口化榜单，经渲染端口产出 Markdown，
// 以期号为聚合身份入库（同期覆盖）。日期按 Asia/Shanghai 自然日计算。
func BuildDigest(ctx context.Context, d Deps, kind digest.Kind, digestTime time.Time, stats DigestStats) (digest.Digest, error) {
	var zero digest.Digest
	view, err := BuildHotView(ctx, d, kind)
	if err != nil {
		return zero, fmt.Errorf("构建榜单视图: %w", err)
	}

	key, label := periodKey(kind, digestTime)
	dv := DigestView{
		Kind:        string(kind),
		Date:        key,
		PeriodLabel: label,
		GitHub:      topN(view.GitHub, 10),
		News:        topNRows(view.News, 10),
		Fusion:      view.Fusion,
		Stats:       stats,
		Generated:   view.Generated,
	}
	md, err := d.DigestRenderer.Render(ctx, dv)
	if err != nil {
		return zero, fmt.Errorf("渲染日报: %w", err)
	}
	dig := digest.Digest{
		Date:     key,
		Kind:     kind,
		Markdown: md,
		Stats: digest.Stats{
			GitHubItems: len(dv.GitHub),
			NewsItems:   len(dv.News),
			Fusion:      len(dv.Fusion),
			Sources:     stats.Sources,
			Collected:   stats.Collected,
			ModelA:      stats.ModelA,
			ModelB:      stats.ModelB,
			Duration:    stats.Duration,
		},
		CreatedAt: d.Clock.Now(),
	}
	if err := d.Digests.Save(ctx, dig); err != nil {
		return zero, fmt.Errorf("保存日报: %w", err)
	}
	return dig, nil
}

// digestSpec 榜单窗口参数：日报看 48h 热度与 24h 增长；周报/月报拉长窗口。
type digestSpec struct {
	storyWindow time.Duration // 事件热度窗口
	projWindow  time.Duration // 项目 star 增长窗口
	label       string
}

func digestKindSpec(kind digest.Kind) digestSpec {
	switch kind {
	case digest.KindWeekly:
		return digestSpec{storyWindow: 7 * 24 * time.Hour, projWindow: 7 * 24 * time.Hour, label: "周报"}
	case digest.KindMonthly:
		return digestSpec{storyWindow: 30 * 24 * time.Hour, projWindow: 30 * 24 * time.Hour, label: "月报"}
	default:
		return digestSpec{storyWindow: 48 * time.Hour, projWindow: 24 * time.Hour, label: "日报"}
	}
}

// periodKey 期号与展示标签（东八区）。
func periodKey(kind digest.Kind, t time.Time) (key, label string) {
	lt := t.In(shanghaiLoc())
	switch kind {
	case digest.KindWeekly:
		y, w := lt.ISOWeek()
		start := lt.AddDate(0, 0, -6)
		return fmt.Sprintf("w-%d-W%02d", y, w),
			fmt.Sprintf("%d 年第 %02d 周（%s ~ %s）", y, w, start.Format("01-02"), lt.Format("01-02"))
	case digest.KindMonthly:
		return fmt.Sprintf("m-%s", lt.Format("2006-01")),
			fmt.Sprintf("%s 月", lt.Format("2006 年 01 月"))
	default:
		return lt.Format("2006-01-02"), lt.Format("2006-01-02")
	}
}

func topN[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func topNRows(xs []StoryRow, n int) []StoryRow { return topN(xs, n) }

// shanghaiLoc 日报日期用东八区。
func shanghaiLoc() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// 精简系统可能没有 tzdata；退化为固定 +8
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// SynthesizeOverviews 事件综述用例：对热度 TopN 的资讯事件，把多条报道
// 整合成一段综述（LLM，每事件一次调用）。已有综述的跳过，可断点续跑。
func SynthesizeOverviews(ctx context.Context, d Deps, topNCount int) (int, error) {
	now := d.Clock.Now()
	stories, err := d.Stories.Active(ctx, now.Add(-48*time.Hour))
	if err != nil {
		return 0, fmt.Errorf("读取活跃事件: %w", err)
	}
	var news []*story.Story
	for _, s := range stories {
		if s.Kind == story.KindNews && strings.TrimSpace(s.Overview) == "" {
			news = append(news, s)
		}
	}
	story.SortByHotness(news)
	if len(news) > topNCount {
		news = news[:topNCount]
	}
	written := 0
	for _, s := range news {
		var ids []string
		for _, m := range s.Members {
			if m.ItemID != "" {
				ids = append(ids, m.ItemID)
			}
		}
		items, err := d.Items.FindByIDs(ctx, ids)
		if err != nil || len(items) == 0 {
			continue
		}
		var lines strings.Builder
		for _, it := range items {
			fmt.Fprintf(&lines, "- %s：%s\n", it.Selection.TitleZh, it.Selection.SummaryZh)
		}
		user := prompts.RenderPrompt(prompts.Overview, "ITEMS", lines.String())
		raw, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.3)
		if err != nil {
			return written, err
		}
		var out struct {
			Overview string `json:"overview"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			continue
		}
		overview := strings.TrimSpace(out.Overview)
		if overview == "" {
			continue
		}
		if err := d.Stories.SaveOverview(ctx, s.ID, overview); err != nil {
			continue
		}
		written++
	}
	return written, nil
}
