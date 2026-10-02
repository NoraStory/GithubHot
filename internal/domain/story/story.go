// Package story 是事件上下文：同一件事的多方报道聚成一个 Story（事件），
// 热度按事件算、不按文章算；AI 资讯事件与 GitHub 项目之间可以建立融合链接。
package story

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Kind 事件种类：news=AI 资讯事件；project=GitHub 项目（项目自身即事件）。
type Kind string

const (
	KindNews    Kind = "news"
	KindProject Kind = "project"
)

// Member 事件的成员：一条精选资料或一个 GitHub 项目。
type Member struct {
	ItemID      string // news 成员的资料 ID
	URL         string
	TitleZh     string
	SummaryZh   string
	SourceID    string    // 判定"独立来源数"的依据之一
	Domain      string    // 判定"独立来源数"的依据之二：同一媒体多篇文章只算一次
	PublishedAt time.Time // 用于热度时间衰减；缺失时按 0 年龄（不衰减）处理
}

// Story 事件聚合根。不变量：
//   - 至少有一个成员；
//   - 热度按独立来源计（同一信源、同一域名多次出现只算一次）。
type Story struct {
	ID          string
	Kind        Kind
	TitleZh     string
	SummaryZh   string
	URL         string // 事件主链接（热度最高成员的 URL）
	Overview    string // 事件综述（LLM 整合多源报道生成，可选）
	Members     []Member
	Projects    []string // 融合链接的 GitHub 仓库（owner/repo）
	Hotness     float64
	FirstSeenAt time.Time
	UpdatedAt   time.Time
}

// NewNews 由一条精选资料构造资讯事件。
func NewNews(id string, m Member, now time.Time) (*Story, error) {
	if m.ItemID == "" && m.URL == "" {
		return nil, fmt.Errorf("事件成员必须有 itemID 或 url")
	}
	return &Story{
		ID:          id,
		Kind:        KindNews,
		TitleZh:     m.TitleZh,
		SummaryZh:   m.SummaryZh,
		URL:         m.URL,
		Members:     []Member{m},
		FirstSeenAt: now,
		UpdatedAt:   now,
	}, nil
}

// NewProject 由一个 GitHub 项目构造项目事件。
func NewProject(fullName, titleZh, summaryZh, url string, now time.Time) *Story {
	return &Story{
		ID:          "story-proj-" + strings.ReplaceAll(fullName, "/", "-"),
		Kind:        KindProject,
		TitleZh:     titleZh,
		SummaryZh:   summaryZh,
		URL:         url,
		Members:     []Member{{URL: url, TitleZh: titleZh, SummaryZh: summaryZh, Domain: "github.com"}},
		Projects:    []string{fullName},
		FirstSeenAt: now,
		UpdatedAt:   now,
	}
}

// Merge 吸收另一个事件：成员并集，标题摘要保留展示价值更高的一方。
// 调用方负责把被吸收方从仓储删除（应用层编排）。
func (s *Story) Merge(other *Story, now time.Time) {
	s.Members = append(s.Members, other.Members...)
	s.Projects = unionStrings(s.Projects, other.Projects)
	otherBest := bestMember(other.Members)
	sBest := bestMember(s.Members)
	if memberRank(otherBest) > memberRank(sBest) {
		s.TitleZh = otherBest.TitleZh
		s.SummaryZh = otherBest.SummaryZh
		s.URL = otherBest.URL
	}
	s.UpdatedAt = now
}

// IndependentSourceCount 独立来源数：同一信源 ID 只算一次，同一域名也只算一次。
func (s *Story) IndependentSourceCount() int {
	seen := map[string]bool{}
	n := 0
	for _, m := range s.Members {
		if m.SourceID == "" && m.Domain == "" {
			continue
		}
		key := "src:" + m.SourceID + "|dom:" + m.Domain
		if seen[key] {
			continue
		}
		seen[key] = true
		n++
	}
	if n == 0 {
		n = 1
	}
	return n
}

func bestMember(ms []Member) Member {
	best := ms[0]
	for _, m := range ms[1:] {
		if memberRank(m) > memberRank(best) {
			best = m
		}
	}
	return best
}

// memberRank 成员展示优先级：有中文标题的 > 没有的；同样有则比标题长度（信息量）。
func memberRank(m Member) int {
	r := 0
	if m.TitleZh != "" {
		r += 10
		r += len([]rune(m.TitleZh))
	}
	if m.URL != "" {
		r++
	}
	return r
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(a, b...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// SortByHotness 按热度降序排列事件。
func SortByHotness(stories []*Story) {
	sort.SliceStable(stories, func(i, j int) bool { return stories[i].Hotness > stories[j].Hotness })
}

// DomainOf 从 URL 提取域名（用于独立来源判定），非法 URL 返回空。
func DomainOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
