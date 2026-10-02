// Package source 是信源上下文：一切热点资料的来源抽象。
// 六类信源统一建模，抓取能力由适配器在基础设施层实现（依赖倒置）。
package source

import (
	"fmt"
	"time"
)

// Kind 信源种类。github_search / github_trending 是 GitHub 域的一等信源；
// x_account 与 wechat_oa 是预留扩展点（需付费 API，适配器未随仓库分发）。
type Kind string

const (
	KindRSS            Kind = "rss"
	KindJSONAPI        Kind = "json_api"
	KindWebList        Kind = "web_list"
	KindHackerNews     Kind = "hacker_news"
	KindGitHubSearch   Kind = "github_search"
	KindGitHubTrending Kind = "github_trending"
	KindXAccount       Kind = "x_account"
	KindWechatOA       Kind = "wechat_oa"
)

// Implemented 报告某信源种类是否随仓库附带适配器。
func (k Kind) Implemented() bool {
	switch k {
	case KindRSS, KindJSONAPI, KindWebList, KindHackerNews, KindGitHubSearch, KindGitHubTrending:
		return true
	default:
		return false
	}
}

// Tier 信源分级：官方一手（T1）权重高于媒体个人（T2），影响精选门槛与热度权重。
type Tier string

const (
	TierFirstParty Tier = "T1"
	TierMedia      Tier = "T2"
)

// Weight 返回该分级在事件热度中的独立来源权重。
func (t Tier) Weight() float64 {
	if t == TierFirstParty {
		return 1.0
	}
	return 0.6
}

// ScoreThreshold 返回该分级的双评分入选门槛（两次独立评分的均值）。
// 一手信源门槛更低：官方博客即使写得克制也值得看。
func (t Tier) ScoreThreshold() float64 {
	if t == TierFirstParty {
		return 6.0
	}
	return 7.0
}

// Source 信源聚合根。
type Source struct {
	ID              string
	Name            string
	Kind            Kind
	Config          map[string]string
	Tier            Tier
	Tags            []string
	IntervalMinutes int
	Enabled         bool
	CreatedAt       time.Time
	LastFetchedAt   *time.Time
}

// Validate 检查信源不变量。
func (s *Source) Validate() error {
	if s.ID == "" || s.Name == "" {
		return fmt.Errorf("信源 id 与 name 不能为空")
	}
	if !s.Kind.Implemented() && s.Kind != KindXAccount && s.Kind != KindWechatOA {
		return fmt.Errorf("未知信源种类 %q", s.Kind)
	}
	if s.IntervalMinutes <= 0 {
		s.IntervalMinutes = 120
	}
	return nil
}

// DueForFetch 报告按抓取间隔该信源是否到期。
// 抓取间隔按产出自动调整的策略在应用层，这里只做基础判断。
func (s *Source) DueForFetch(now time.Time) bool {
	if !s.Enabled {
		return false
	}
	if s.LastFetchedAt == nil {
		return true
	}
	return now.Sub(*s.LastFetchedAt) >= time.Duration(s.IntervalMinutes)*time.Minute
}

// ConfigValue 取配置项，缺省返回 fallback。
func (s *Source) ConfigValue(key, fallback string) string {
	if v, ok := s.Config[key]; ok && v != "" {
		return v
	}
	return fallback
}

// ConfigInt 取整数配置项。
func (s *Source) ConfigInt(key string, fallback int) int {
	if v, ok := s.Config[key]; ok && v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

// MarkFetched 记录最近一次抓取时间。
func (s *Source) MarkFetched(at time.Time) { s.LastFetchedAt = &at }
