package application

import (
	"context"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// 国内热榜轻管道：热榜条目（StageHotBoard）不走 LLM，用
// 标题相似度聚簇 + 多源共振 + 榜位分 + 时间衰减直接出榜。
// 与 LLM 聚簇（cluster.go）完全隔离，互不影响。
const (
	// domesticWindowHours 热榜原料时间窗：24h 外的条目自动过期（查询级清理，无需调度任务）。
	domesticWindowHours = 24.0
	// domesticBoardSize 榜单展示条数。
	domesticBoardSize = 30
	// domesticClusterThreshold 标题聚簇相似度阈值（CJK 二元组 Jaccard）。
	domesticClusterThreshold = 0.45
	// domesticMinTitleRunes 标题过短时提高阈值，避免"国足""金价"这类短词误并。
	domesticMinTitleRunes = 6
	// domesticShortTitleThreshold 短标题的更高阈值。
	domesticShortTitleThreshold = 0.7
	// domesticResonanceSources 多源共振门槛：至少 2 个独立信源同报一件事才入榜。
	domesticResonanceSources = 2
	// domesticMinBoard 单源兜底：共振结果太少时用单源高分条目补足，保证板块不空。
	domesticMinBoard = 15
	// domesticDecayHalfLifeHours 热度衰减半衰期：6h。
	domesticDecayHalfLifeHours = 6.0
	// domesticSourceBonus 每多一个独立信源的加成系数（×(1+0.15·(n-1))）。
	domesticSourceBonus = 0.15
)

// DomesticRow 国内热榜行。
type DomesticRow struct {
	Rank        int       `json:"rank"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Hotness     float64   `json:"hotness"`
	Sources     []string  `json:"sources"` // 信源展示名（去重）
	SourceCount int       `json:"sourceCount"`
	MemberCount int       `json:"memberCount"`
	Badges      []string  `json:"badges,omitempty"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// DomesticView 国内热榜视图。
type DomesticView struct {
	Generated time.Time     `json:"generatedAt"`
	Items     []DomesticRow `json:"items"`
}

// BuildDomesticView 构建国内热榜：读取 24h 窗口内热榜条目，
// 聚簇打分排序。每次调用实时计算，过期条目由查询窗口天然剔除。
func BuildDomesticView(ctx context.Context, d Deps, limit int) (DomesticView, error) {
	view := DomesticView{Generated: d.Clock.Now(), Items: []DomesticRow{}}
	if limit <= 0 {
		limit = domesticBoardSize
	}
	since := view.Generated.Add(-time.Duration(domesticWindowHours * float64(time.Hour)))
	items, err := d.Items.HotBoardSince(ctx, since, 500)
	if err != nil {
		return view, err
	}
	if len(items) == 0 {
		return view, nil
	}
	sourceNames, err := loadSourceNames(ctx, d)
	if err != nil {
		return view, err
	}
	clusters := clusterDomestic(items, view.Generated)
	for i, c := range clusters {
		if i >= limit {
			break
		}
		view.Items = append(view.Items, c.row(i+1, sourceNames))
	}
	return view, nil
}

// domesticCluster 一个国内热榜聚簇（同一件事的多家报道）。
type domesticCluster struct {
	members []item.Item
	sources map[string]bool // sourceID 去重
	heat    float64
	newest  time.Time
	oldest  time.Time
}

// row 转展示行：标题/链接取榜位最好的成员（rank 最小）。
func (c *domesticCluster) row(rank int, sourceNames map[string]string) DomesticRow {
	best := c.members[0]
	bestRank := boardRank(best)
	for _, m := range c.members[1:] {
		if r := boardRank(m); r < bestRank {
			bestRank = r
			best = m
		}
	}
	names := make([]string, 0, len(c.sources))
	for id := range c.sources {
		names = append(names, sourceNames[id])
	}
	sort.Strings(names)
	row := DomesticRow{
		Rank:        rank,
		Title:       best.Title,
		URL:         best.URL,
		Hotness:     math.Round(c.heat*10) / 10,
		Sources:     names,
		SourceCount: len(c.sources),
		MemberCount: len(c.members),
		FirstSeenAt: c.oldest,
		UpdatedAt:   c.newest,
	}
	if len(c.sources) >= 3 {
		row.Badges = append(row.Badges, "沸")
	}
	return row
}

// clusterDomestic 聚簇 + 打分 + 排序 + 共振过滤（含单源兜底）。
func clusterDomestic(items []item.Item, now time.Time) []*domesticCluster {
	var clusters []*domesticCluster
	for _, it := range items {
		title := it.Title
		threshold := domesticClusterThreshold
		if len([]rune(title)) < domesticMinTitleRunes {
			threshold = domesticShortTitleThreshold
		}
		var target *domesticCluster
		bestSim := 0.0
		for _, c := range clusters {
			for _, m := range c.members {
				if sim := story.LexicalSimilarity(title, m.Title); sim > bestSim {
					bestSim = sim
					target = c
				}
			}
		}
		if target == nil || bestSim < threshold {
			target = &domesticCluster{sources: map[string]bool{}}
			clusters = append(clusters, target)
		}
		target.members = append(target.members, it)
		target.sources[it.SourceID] = true
	}

	// 打分：Σ 100/√rank × 2^(-年龄h/6)；独立信源加成。
	for _, c := range clusters {
		for _, m := range c.members {
			age := math.Max(0, now.Sub(m.FetchedAt).Hours())
			c.heat += 100 / math.Sqrt(float64(boardRank(m))) * math.Pow(0.5, age/domesticDecayHalfLifeHours)
			if m.FetchedAt.After(c.newest) {
				c.newest = m.FetchedAt
			}
			if c.oldest.IsZero() || m.FetchedAt.Before(c.oldest) {
				c.oldest = m.FetchedAt
			}
		}
		c.heat *= 1 + domesticSourceBonus*float64(len(c.sources)-1)
	}
	sort.SliceStable(clusters, func(i, j int) bool { return clusters[i].heat > clusters[j].heat })

	// 共振门槛：≥2 独立信源；不足 domesticMinBoard 时用单源高分条目兜底。
	var resonant, single []*domesticCluster
	for _, c := range clusters {
		if len(c.sources) >= domesticResonanceSources {
			resonant = append(resonant, c)
		} else {
			single = append(single, c)
		}
	}
	if len(resonant) < domesticMinBoard {
		need := domesticMinBoard - len(resonant)
		if need > len(single) {
			need = len(single)
		}
		resonant = append(resonant, single[:need]...)
		sort.SliceStable(resonant, func(i, j int) bool { return resonant[i].heat > resonant[j].heat })
	}
	return resonant
}

// boardRank 条目榜位（Meta.rank），缺省按 99 处理。
func boardRank(it item.Item) int {
	if it.Meta == nil {
		return 99
	}
	if r, err := strconv.Atoi(it.Meta["rank"]); err == nil && r > 0 {
		return r
	}
	return 99
}
