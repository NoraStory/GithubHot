// Package github 是 GitHub 开源项目热点上下文。
// 项目热度不看绝对 star 数，看"窗口期内的 star 增长"——
// 300 star 的新项目比静态 30 万 star 的老项目更"热"。
package github

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var fullNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Project 仓库项目实体。FullName（owner/repo）是身份标识。
type Project struct {
	FullName      string
	HTMLURL       string
	Description   string
	DescriptionZh string // 中文描述（LLM 翻译，发现阶段批量补全）
	Language      string
	Topics        []string
	Stars         int
	Forks         int
	TrendingRank  int // 最近一次 trending 页排名（0=未上榜）
	FirstSeenAt   time.Time
	LastSeenAt    time.Time
}

// New 校验并构造项目。
func New(fullName, htmlURL string, stars int, now time.Time) (*Project, error) {
	if !fullNameRe.MatchString(fullName) {
		return nil, fmt.Errorf("仓库名非法（应为 owner/repo）: %q", fullName)
	}
	if htmlURL == "" {
		htmlURL = "https://github.com/" + fullName
	}
	return &Project{
		FullName:    fullName,
		HTMLURL:     htmlURL,
		Stars:       stars,
		FirstSeenAt: now,
		LastSeenAt:  now,
	}, nil
}

// Merge 双轨发现（Search + Trending）合并同一项目，保留信息更丰富的字段。
func (p *Project) Merge(other Project) {
	if other.Description != "" {
		p.Description = other.Description
	}
	if other.Language != "" {
		p.Language = other.Language
	}
	if len(other.Topics) > 0 {
		p.Topics = other.Topics
	}
	if other.Stars > p.Stars {
		p.Stars = other.Stars
	}
	if other.Forks > p.Forks {
		p.Forks = other.Forks
	}
	if other.TrendingRank > 0 && (p.TrendingRank == 0 || other.TrendingRank < p.TrendingRank) {
		p.TrendingRank = other.TrendingRank
	}
	p.LastSeenAt = other.LastSeenAt
}

// Snapshot 项目在某个时刻的观测值。热度由快照差分推导，重复抓取不会虚增热度。
type Snapshot struct {
	FullName     string
	At           time.Time
	Stars        int
	TrendingRank int
	SearchRank   int
}

// sortSnaps 返回按时间升序的快照副本。
func sortSnaps(snaps []Snapshot) []Snapshot {
	out := make([]Snapshot, len(snaps))
	copy(out, snaps)
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// growthBaseline 计算窗口增长基线：窗口内最早的观测；
// 窗口内只有一个观测时，退化为窗口外最近的观测；再没有则用全局最早。
func growthBaseline(snaps []Snapshot, now time.Time, window time.Duration) (base, latest Snapshot) {
	s := sortSnaps(snaps)
	latest = s[len(s)-1]
	windowStart := now.Add(-window)
	inWindow := -1
	for i, x := range s {
		if !x.At.Before(windowStart) {
			inWindow = i
			break
		}
	}
	if inWindow >= 0 && !s[inWindow].At.Equal(latest.At) {
		return s[inWindow], latest
	}
	// 窗口内只有一个观测：取窗口外最近的观测做基线
	for i := inWindow - 1; i >= 0; i-- {
		if s[i].At.Before(windowStart) {
			return s[i], latest
		}
	}
	if len(s) > 1 {
		return s[0], latest
	}
	return latest, latest
}

// HotnessInput 热度领域服务的输入。
type HotnessInput struct {
	Snapshots []Snapshot // 任意顺序
	Now       time.Time
	// Window 增长统计窗口（日报 24h / 周报 7d / 月报 30d）。0 = 24h。
	Window time.Duration
	// Resonance 窗口内引用该项目的独立资讯事件数（多源共振，0 = 无）。
	// 共振加成 H ×= 1 + 0.08×min(Resonance,5)：1 家 ×1.08 → 5+ 家封顶 ×1.4，
	// 与故事侧融合加成（×1.25）同数量级——star 增量之外吸收"被多家媒体报道"的信号。
	Resonance int
}

// resonanceStep 每个独立资讯事件的共振步长（封顶 5 家 → ×1.4）。
const resonanceStep = 0.08

// Hotness 计算项目热度（0-100 量级）：
//
//	gained = 窗口内（默认 24h）star 增量（快照差分，重复抓取不虚增）
//	base   = 10 × log2(1 + gained)           —— 对数抑制头部碾压
//	trend  = trending 排名加成（1-3 名 +6，4-10 +3，11-25 +1）
//	novel  = 首次发现不足 24h 且有增长 +5     —— "新爆"信号
//	reson  = 多源共振乘数（1 + 0.08×min(k,5)）
//	H      = (base + trend + novel) × reson
func Hotness(in HotnessInput) float64 {
	if len(in.Snapshots) == 0 {
		return 0
	}
	window := in.Window
	if window <= 0 {
		window = 24 * time.Hour
	}
	base, latest := growthBaseline(in.Snapshots, in.Now, window)
	gained := latest.Stars - base.Stars
	if gained < 0 {
		gained = 0
	}

	h := 10 * math.Log2(1+float64(gained))

	if r := latest.TrendingRank; r > 0 {
		switch {
		case r <= 3:
			h += 6
		case r <= 10:
			h += 3
		case r <= 25:
			h += 1
		}
	}
	firstSeen := in.Snapshots[0].At
	for _, s := range in.Snapshots {
		if s.At.Before(firstSeen) {
			firstSeen = s.At
		}
	}
	if in.Now.Sub(firstSeen) < 24*time.Hour && gained > 0 {
		h += 5
	}
	if k := in.Resonance; k > 0 {
		if k > 5 {
			k = 5
		}
		h *= 1 + resonanceStep*float64(k)
	}
	return math.Round(h*10) / 10
}

// StarsGainedIn 报告窗口时长内的 star 增量（用于展示）。
func StarsGainedIn(snaps []Snapshot, now time.Time, window time.Duration) int {
	if len(snaps) == 0 {
		return 0
	}
	base, latest := growthBaseline(snaps, now, window)
	g := latest.Stars - base.Stars
	if g < 0 {
		g = 0
	}
	return g
}

// NormalizeDescription 清理仓库描述（合并空白、按 rune 截断）。
func NormalizeDescription(d string, max int) string {
	d = strings.Join(strings.Fields(d), " ")
	r := []rune(d)
	if len(r) <= max {
		return d
	}
	return string(r[:max]) + "…"
}
