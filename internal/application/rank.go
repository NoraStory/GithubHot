package application

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/github"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// RankStats 热度重算统计。
type RankStats struct {
	NewsStories int                `json:"newsStories"`
	Hotness     map[string]float64 `json:"-"`
}

// RankStories 热度用例：对 48h 内全部资讯事件重算热度并记录历史。
// 融合链接（story.Projects 非空）享受 ×1.25 加成；项目热度在构建
// 榜单视图时由领域服务 github.Hotness 从快照差分现算。
func RankStories(ctx context.Context, d Deps) (RankStats, error) {
	var stats RankStats
	stats.Hotness = map[string]float64{}
	now := d.Clock.Now()

	all, err := d.Sources.All(ctx)
	if err != nil {
		return stats, fmt.Errorf("读取信源: %w", err)
	}
	tiers := map[string]source.Tier{}
	for _, s := range all {
		tiers[s.ID] = s.Tier
	}

	active, err := d.Stories.Active(ctx, now.Add(-48*time.Hour))
	if err != nil {
		return stats, fmt.Errorf("读取活跃事件: %w", err)
	}
	for _, s := range active {
		if s.Kind != story.KindNews {
			continue
		}
		h := story.Hotness(story.HotnessInput{
			Members:       s.Members,
			Tiers:         tiers,
			HasFusionLink: len(s.Projects) > 0,
			Now:           now,
		})
		s.Hotness = h
		if err := d.Stories.Save(ctx, s); err != nil {
			log.Printf("[rank] 保存热度失败 %s: %v", s.ID, err)
			continue
		}
		if err := d.Stories.AddHistory(ctx, s.ID, now, h); err != nil {
			log.Printf("[rank] 记录热度历史失败 %s: %v", s.ID, err)
		}
		stats.Hotness[s.ID] = h
		stats.NewsStories++
	}
	return stats, nil
}

// ProjectBoardRow 项目榜单的中间行：项目 + 快照 + 现算热度。
type ProjectBoardRow struct {
	Project github.Project
	Snaps   []github.Snapshot
	Hotness float64
}

// ProjectBoard 计算项目榜单：全部项目按领域热度服务排序取前 limit。
// 热度每次现算而非落库——快照在，随时可重放任意时刻的榜单。
func ProjectBoard(ctx context.Context, d Deps, limit int) ([]ProjectBoardRow, error) {
	all, err := d.Projects.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取项目: %w", err)
	}
	now := d.Clock.Now()
	snapAll, err := d.Projects.AllSnapshotsSince(ctx, now.Add(-72*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("读取快照: %w", err)
	}
	rows := make([]ProjectBoardRow, 0, len(all))
	for i := range all {
		p := all[i]
		snaps := snapAll[p.FullName]
		rows = append(rows, ProjectBoardRow{
			Project: p,
			Snaps:   snaps,
			Hotness: github.Hotness(github.HotnessInput{Snapshots: snaps, Now: now}),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Hotness > rows[j].Hotness })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}
