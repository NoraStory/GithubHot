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

// Kind 日报种类。
type Kind string

const (
	KindDaily   Kind = "daily"
	KindWeekly  Kind = "weekly"
	KindMonthly Kind = "monthly"
)

// Digest 日报聚合根。Date 是期号身份：daily=YYYY-MM-DD（东八区自然日），
// weekly=w-YYYY-Www（ISO 周），monthly=m-YYYY-MM。一天/周/月一份。
type Digest struct {
	Date      string    `json:"date"`
	Kind      Kind      `json:"kind"`
	Markdown  string    `json:"markdown"`
	Stats     Stats     `json:"stats"`
	CreatedAt time.Time `json:"createdAt"`
}
