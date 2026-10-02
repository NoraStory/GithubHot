package application

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/NoraStory/GithubHot/internal/domain/github"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// DiscoverStats GitHub 双轨发现统计。
type DiscoverStats struct {
	SearchRepos   int `json:"searchRepos"`
	TrendingRepos int `json:"trendingRepos"`
	NewProjects   int `json:"newProjects"`
	Merged        int `json:"merged"`
}

// DiscoverProjects GitHub 热点双轨发现用例：
//   - 轨道 A（官方 Search API）：近 N 天创建、star 过阈值的新项目，按 star 排序；
//   - 轨道 B（trending 页）：社区每日榜。
//
// 两轨结果合并去重进项目仓储，并落一张快照——热度由快照差分推导，
// 重复抓取不虚增。trending 抓取失败时自动降级为仅 Search 轨道。
func DiscoverProjects(ctx context.Context, d Deps) (DiscoverStats, error) {
	stats := DiscoverStats{}
	all, err := d.Sources.All(ctx)
	if err != nil {
		return stats, fmt.Errorf("读取信源: %w", err)
	}
	now := d.Clock.Now()

	enabled := map[string]bool{}
	for _, s := range all {
		if s.Enabled && s.Kind.Implemented() {
			enabled[string(s.Kind)] = true
		}
	}

	var repos []GitHubRepo
	if enabled[string(source.KindGitHubSearch)] {
		for _, s := range all {
			if s.Kind != source.KindGitHubSearch || !s.Enabled {
				continue
			}
			rs, err := d.GitHub.SearchNewRising(ctx,
				s.ConfigInt("since_days", 7), s.ConfigInt("min_stars", 50), s.ConfigInt("per_page", 25))
			if err != nil {
				log.Printf("[discover] search 轨道失败: %v", err)
				continue
			}
			for i := range rs {
				rs[i].SearchRank = i + 1
			}
			stats.SearchRepos += len(rs)
			repos = append(repos, rs...)
		}
	}
	if enabled[string(source.KindGitHubTrending)] {
		rs, err := d.GitHub.FetchTrending(ctx)
		if err != nil {
			// 降级而非失败：trending 页是社区口径的补充信号
			log.Printf("[discover] trending 轨道失败（已降级为仅 Search 轨道）: %v", err)
		} else {
			stats.TrendingRepos = len(rs)
			repos = append(repos, rs...)
		}
	}

	for _, r := range repos {
		if strings.TrimSpace(r.FullName) == "" {
			continue
		}
		existing, err := d.Projects.FindByFullName(ctx, r.FullName)
		if err != nil || existing == nil {
			p, cerr := github.New(r.FullName, r.HTMLURL, r.Stars, now)
			if cerr != nil {
				continue
			}
			p.Description = github.NormalizeDescription(r.Description, 300)
			p.Language = r.Language
			p.Topics = r.Topics
			p.Forks = r.Forks
			p.TrendingRank = r.TrendingRank
			if serr := d.Projects.Upsert(ctx, *p); serr != nil {
				continue
			}
			stats.NewProjects++
			continue
		}
		merged := *existing
		before := merged.LastSeenAt
		merged.Merge(github.Project{
			FullName: r.FullName, Description: r.Description, Language: r.Language,
			Topics: r.Topics, Stars: r.Stars, Forks: r.Forks,
			TrendingRank: r.TrendingRank, LastSeenAt: now,
		})
		if !merged.LastSeenAt.Equal(before) {
			stats.Merged++
		}
		if terr := d.Projects.Touch(ctx, merged); terr != nil {
			log.Printf("[discover] 更新项目 %s: %v", r.FullName, terr)
		}
	}

	// 落快照：同一项目被两轨同时观测时，取信息最丰富的一条
	//（优先带 trending 排名的观测，其次 star 更高的），供热度差分
	best := map[string]GitHubRepo{}
	for _, r := range repos {
		if strings.TrimSpace(r.FullName) == "" {
			continue
		}
		cur, ok := best[r.FullName]
		if !ok || betterObservation(r, cur) {
			best[r.FullName] = r
		}
	}
	for _, r := range best {
		if serr := d.Projects.AddSnapshot(ctx, github.Snapshot{
			FullName: r.FullName, At: now, Stars: r.Stars,
			TrendingRank: r.TrendingRank, SearchRank: r.SearchRank,
		}); serr != nil {
			log.Printf("[discover] 落快照 %s: %v", r.FullName, serr)
		}
	}
	return stats, nil
}

// betterObservation 观测择优：trending 排名优先（承载榜单信号），其次 star 数。
func betterObservation(a, b GitHubRepo) bool {
	if (a.TrendingRank > 0) != (b.TrendingRank > 0) {
		return a.TrendingRank > 0
	}
	return a.Stars > b.Stars
}
