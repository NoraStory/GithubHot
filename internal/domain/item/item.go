// Package item 是原始资料上下文：从信源进来的一条条资料，以及它被
// 精选流程（预筛 → 双评分 → 中文写作）逐步加工后的状态。
package item

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/shared"
)

// Stage 精选流程阶段。
type Stage string

const (
	StageNew       Stage = "new"         // 刚采集
	StageFiltered  Stage = "prefiltered" // 预筛通过
	StageDropped   Stage = "dropped"     // 预筛淘汰
	StageScored    Stage = "scored"      // 双评分通过
	StageRejected  Stage = "rejected"    // 双评分未过门槛
	StageWritten   Stage = "written"     // 已完成中文写作
	StageClustered Stage = "clustered"   // 已聚入事件（聚簇只处理一次，防重复合并/重复消耗 LLM）
)

// Selection 精选结果值对象。不可变，只能整体替换。
type Selection struct {
	Stage     Stage
	Pass      bool
	Reason    string  // 预筛理由
	ScoreA    float64 // 第一次独立评分 0-10
	ScoreB    float64 // 第二次独立评分 0-10
	TitleZh   string
	SummaryZh string
	ReasonZh  string
	Tags      []string
}

// Item 原始资料实体。ID 是 URL 归一化后的判重键，天然防重复入库。
type Item struct {
	ID          string
	SourceID    string
	SourceTier  string
	URL         string
	Title       string
	Summary     string
	Content     string
	ContentZh   string // 正文/摘要的忠实中文翻译（本地存档，LLM 产出）
	Author      string
	PublishedAt time.Time
	FetchedAt   time.Time
	Selection   Selection
}

// New 构造资料并校验不变量：URL 必须是 http/https，标题不能为空。
func New(id, sourceID, sourceTier, rawURL, title string, publishedAt, fetchedAt time.Time) (*Item, error) {
	if id == "" {
		return nil, fmt.Errorf("item id 不能为空")
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("资料 URL 非法: %q", rawURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("资料 URL 仅允许 http/https: %q", rawURL)
	}
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("资料标题不能为空: %s", rawURL)
	}
	return &Item{
		ID:          id,
		SourceID:    sourceID,
		SourceTier:  sourceTier,
		URL:         rawURL,
		Title:       strings.TrimSpace(title),
		PublishedAt: publishedAt,
		FetchedAt:   fetchedAt,
		Selection:   Selection{Stage: StageNew},
	}, nil
}

// AgeHours 资料年龄（小时，相对 now）。
func (i *Item) AgeHours(now time.Time) float64 {
	base := i.PublishedAt
	if base.IsZero() {
		base = i.FetchedAt
	}
	return now.Sub(base).Hours()
}

// AverageScore 双评分均值。
func (i *Item) AverageScore() float64 { return (i.Selection.ScoreA + i.Selection.ScoreB) / 2 }

// TextForLLM 拼接供大模型阅读的正文（标题 + 摘要 + 截断正文）。
func (i *Item) TextForLLM(maxContent int) string {
	var b strings.Builder
	b.WriteString("标题: ")
	b.WriteString(i.Title)
	if i.Summary != "" && i.Summary != i.Title {
		b.WriteString("\n摘要: ")
		b.WriteString(i.Summary)
	}
	if i.Content != "" {
		b.WriteString("\n正文: ")
		b.WriteString(shared.Truncate(i.Content, maxContent))
	}
	return b.String()
}
