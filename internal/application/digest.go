package application

import (
	"context"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/digest"
)

// BuildDigest 日报用例：取双榜前 10 与融合观察，经渲染端口产出 Markdown，
// 以"日期"为聚合身份入库（一天一份，重复构建覆盖同日旧版）。
// 日期按 Asia/Shanghai 自然日计算——日报是给人看的，跟读者时区走。
func BuildDigest(ctx context.Context, d Deps, digestTime time.Time, stats DigestStats) (digest.Digest, error) {
	var zero digest.Digest
	view, err := BuildHotView(ctx, d)
	if err != nil {
		return zero, fmt.Errorf("构建榜单视图: %w", err)
	}

	dv := DigestView{
		Date:      digestTime.In(shanghaiLoc()).Format("2006-01-02"),
		GitHub:    topN(view.GitHub, 10),
		News:      topNRows(view.News, 10),
		Fusion:    view.Fusion,
		Stats:     stats,
		Generated: view.Generated,
	}
	md, err := d.DigestRenderer.Render(ctx, dv)
	if err != nil {
		return zero, fmt.Errorf("渲染日报: %w", err)
	}
	dig := digest.Digest{
		Date:     dv.Date,
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
