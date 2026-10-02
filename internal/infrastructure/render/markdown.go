// Package render 实现应用层的 DigestRenderer / SiteRenderer 端口：
// 日报 Markdown 与双榜网页（同一份视图模型，两种皮肤）。
package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
)

// Markdown 日报渲染器。
type Markdown struct{}

// NewMarkdown 构造。
func NewMarkdown() *Markdown { return &Markdown{} }

// Render 生成 Markdown 日报。
func (Markdown) Render(_ context.Context, v application.DigestView) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# GithubHot 双热点日报 · %s\n\n", v.Date)
	fmt.Fprintf(&b, "> 生成于 %s · 模型 %s", v.Generated.Format("2006-01-02 15:04 MST"), v.Stats.ModelA)
	if v.Stats.ModelB != "" && v.Stats.ModelB != v.Stats.ModelA {
		b.WriteString(" / " + v.Stats.ModelB)
	}
	fmt.Fprintf(&b, " · 今日采集 %d 条 · 耗时 %.0fs\n\n", v.Stats.Collected, v.Stats.Duration)

	// ---------- GitHub 项目榜 ----------
	b.WriteString("## 🔥 GitHub 项目热点 Top 10\n\n")
	if len(v.GitHub) == 0 {
		b.WriteString("（本轮无数据）\n\n")
	} else {
		b.WriteString("| # | 项目 | 语言 | 24h★ | 热度 | 说明 |\n|---|------|------|------|------|------|\n")
		for _, p := range v.GitHub {
			badges := strings.Join(p.Badges, " ")
			if badges != "" {
				badges = " " + badges
			}
			fmt.Fprintf(&b, "| %d | [%s](%s)%s | %s | +%d | %.1f | %s |\n",
				p.Rank, p.FullName, p.URL, badges, orDash(p.Language), p.StarsGained, p.Hotness, mdEscape(p.Description))
		}
		b.WriteString("\n")
	}

	// ---------- AI 资讯榜 ----------
	b.WriteString("## 🤖 AI 资讯热点 Top 10\n\n")
	if len(v.News) == 0 {
		b.WriteString("（本轮无数据）\n\n")
	} else {
		for _, s := range v.News {
			badges := ""
			if len(s.Badges) > 0 {
				badges = " `" + strings.Join(s.Badges, "` `") + "`"
			}
			fmt.Fprintf(&b, "### %d. [%s](%s)%s\n\n", s.Rank, mdEscape(s.TitleZh), s.URL, badges)
			if s.SummaryZh != "" {
				fmt.Fprintf(&b, "%s\n\n", s.SummaryZh)
			}
			meta := make([]string, 0, 4)
			if len(s.SourceNames) > 0 {
				meta = append(meta, "来源: "+strings.Join(s.SourceNames, "、"))
			}
			if s.ReasonZh != "" {
				meta = append(meta, s.ReasonZh)
			}
			if s.Score > 0 {
				meta = append(meta, fmt.Sprintf("评分 %.1f/10 · 热度 %.1f", s.Score, s.Hotness))
			}
			if len(s.Tags) > 0 {
				meta = append(meta, "#"+strings.Join(s.Tags, " #"))
			}
			if len(meta) > 0 {
				fmt.Fprintf(&b, "> %s\n\n", strings.Join(meta, " · "))
			}
		}
	}

	// ---------- 融合观察 ----------
	b.WriteString("## 🔗 融合观察：资讯 × 项目互相印证\n\n")
	if len(v.Fusion) == 0 {
		b.WriteString("（本轮未发现资讯与项目的直接对应）\n\n")
	} else {
		for _, f := range v.Fusion {
			fmt.Fprintf(&b, "- %s × [%s](%s)\n", f.News.TitleZh, f.Project.FullName, f.Project.URL)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")
	b.WriteString("*由 [GithubHot](https://github.com/NoraStory/GithubHot) 自动生成：GitHub 开源热点 × AI 资讯热点，双热度追踪。*\n")
	return b.String(), nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// mdEscape 转义 Markdown 表格敏感字符。
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:80]) + "…"
	}
	return s
}

// 编译期接口满足性检查。
var _ application.DigestRenderer = Markdown{}
