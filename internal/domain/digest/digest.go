// Package digest 是日报上下文：每日热点榜单的固化产物（Markdown 与统计）。
package digest

import "time"

// Stats 日报统计（随 Markdown 一起持久化，JSON 标签即 APP 消费形状）。
type Stats struct {
	GitHubItems int     `json:"githubItems"`
	NewsItems   int     `json:"newsItems"`
	Fusion      int     `json:"fusion"`
	Sources     int     `json:"sources"`
	Collected   int     `json:"collected"`
	ModelA      string  `json:"modelA"`
	ModelB      string  `json:"modelB"`
	Duration    float64 `json:"durationSeconds"`
}

// Digest 日报聚合根。Date（东八区自然日）是身份标识，一天一份。
type Digest struct {
	Date      string
	Markdown  string
	Stats     Stats
	CreatedAt time.Time
}
