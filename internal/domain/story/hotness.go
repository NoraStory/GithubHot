package story

import (
	"math"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// HotnessInput 资讯事件热度计算输入。
type HotnessInput struct {
	Members       []Member
	Tiers         map[string]source.Tier // sourceID -> 分级（缺失按 T2）
	HasFusionLink bool                   // 是否已融合链接到 GitHub 项目
	Now           time.Time
}

// 热度公式参数（与 docs/architecture.md 保持一致）。
const (
	// HotnessWindowHours 热度时间窗：48 小时之外的报道不再贡献热度。
	HotnessWindowHours = 48.0
	// HalfLifeHours 衰减半衰期：每 24 小时权重减半。
	HalfLifeHours = 24.0
	// FusionBoost 融合加成：一条资讯与 GitHub 项目互相印证时，可信度更高。
	FusionBoost = 1.25
	// SourceWeight 每个独立来源的基础热度。
	SourceWeight = 10.0
)

// Hotness 计算资讯事件热度：
//
//	H = Σ(每个独立来源 w(tier) × 0.5^(年龄h/24)) × 10 × fusion
//
// 独立来源按（信源 ID, 域名）去重——同一媒体发十篇只算一次；
// 48h 窗口外的成员不参与；来源分级 T1=1.0、T2=0.6；
// 与 GitHub 项目建立融合链接的事件 ×1.25。
func Hotness(in HotnessInput) float64 {
	type srcKey struct{ source, domain string }
	seen := map[srcKey]bool{}
	h := 0.0
	for _, m := range in.Members {
		key := srcKey{m.SourceID, m.Domain}
		if m.SourceID != "" || m.Domain != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		age := ageOf(m, in.Now)
		if age < 0 || age > HotnessWindowHours {
			continue
		}
		w := source.TierMedia.Weight()
		if t, ok := in.Tiers[m.SourceID]; ok {
			w = t.Weight()
		}
		decay := math.Pow(0.5, age/HalfLifeHours)
		h += w * decay
	}
	if h == 0 && len(in.Members) > 0 {
		// 全部成员超出窗口时保底为一条，避免事件从榜单上凭空消失
		h = source.TierMedia.Weight() * math.Pow(0.5, HotnessWindowHours/HalfLifeHours)
	}
	h *= SourceWeight
	if in.HasFusionLink {
		h *= FusionBoost
	}
	return math.Round(h*10) / 10
}

// ageOf 成员年龄。发布时间缺失时按 0 岁处理（视为刚发生，不衰减）。
func ageOf(m Member, now time.Time) float64 {
	if m.PublishedAt.IsZero() {
		return 0
	}
	return now.Sub(m.PublishedAt).Hours()
}

// Rising 判断事件是否在上升：当前热度比 ref（约 6 小时前）高出 15% 以上。
func Rising(current, ref float64) bool {
	return ref > 0 && current >= ref*1.15
}

// IsNew 事件是否为"新"：首次发现不足 12 小时。
func IsNew(firstSeen, now time.Time) bool {
	return now.Sub(firstSeen) < 12*time.Hour
}
