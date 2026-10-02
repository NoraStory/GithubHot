package render

import (
	"context"
	"embed"
	"fmt"
	"html"
	"log"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
)

//go:embed templates/index.html
var templates embed.FS

// Site 双榜网页渲染器（嵌入模板，单二进制自带前端）。
type Site struct{}

// NewSite 构造。
func NewSite() *Site { return &Site{} }

// RenderIndex 渲染双榜静态页（AnZhiYu 布局：文章卡片 + 侧栏）。
func (Site) RenderIndex(_ context.Context, v application.HotView) (string, error) {
	tpl, err := templates.ReadFile("templates/index.html")
	if err != nil {
		return "", fmt.Errorf("读取模板: %w", err)
	}
	out := string(tpl)
	out = strings.ReplaceAll(out, "{{GENERATED}}", v.Generated.Format("2006-01-02 15:04"))
	out = strings.ReplaceAll(out, "{{GITHUB_ROWS}}", githubRowsHTML(v.GitHub))
	out = strings.ReplaceAll(out, "{{NEWS_ROWS}}", newsRowsHTML(v.News))
	out = strings.ReplaceAll(out, "{{FUSION_ROWS}}", fusionRowsHTML(v.Fusion))
	out = strings.ReplaceAll(out, "{{STAT_PROJECTS}}", fmt.Sprint(len(v.GitHub)))
	out = strings.ReplaceAll(out, "{{STAT_STORIES}}", fmt.Sprint(len(v.News)))
	out = strings.ReplaceAll(out, "{{STAT_FUSION}}", fmt.Sprint(len(v.Fusion)))
	out = strings.ReplaceAll(out, "{{DIGEST_LIST}}", digestListHTML(v.Digests))
	return out, nil
}

// rankMedal 前三名金银铜奖牌，其余纯数字。
func rankMedal(rank int) string {
	if rank >= 1 && rank <= 3 {
		return fmt.Sprintf(`<span class="medal m%d">%d</span>`, rank, rank)
	}
	return fmt.Sprint(rank)
}

func badgeClass(b string) string {
	switch b {
	case "新":
		return "badge new"
	case "上升":
		return "badge rise"
	case "GitHub关联":
		return "badge gh"
	default:
		return "badge"
	}
}

var _ = log.LUTC

func githubRowsHTML(rows []application.ProjectRow) string {
	if len(rows) == 0 {
		return emptyRow("暂无数据——先运行一次 `githubhot run`")
	}
	var b strings.Builder
	for _, p := range rows {
		descZh, descEn := "", ""
		if p.DescriptionZh != "" {
			descZh = p.DescriptionZh
			if p.Description != "" {
				descEn = p.Description
			}
		} else {
			descEn = p.Description
		}
		var badges strings.Builder
		for _, bd := range p.Badges {
			fmt.Fprintf(&badges, `<span class="%s">%s</span>`, badgeClass(bd), html.EscapeString(bd))
		}
		var topics strings.Builder
		for i, t := range p.Topics {
			if i >= 4 {
				break
			}
			fmt.Fprintf(&topics, `<span class="langchip" style="margin-left:4px">%s</span>`, html.EscapeString(t))
		}
		fmt.Fprintf(&b, `<tr>
<td class="rank">%s</td>
<td><a class="repo-name" href="%s" target="_blank" rel="noopener">%s</a> %s
<div class="repo-desc"><span class="zh">%s</span>%s</div></td>
<td><span class="langchip">%s</span></td>
<td class="num gain">+%d</td>
<td class="num hot">%.1f</td>
</tr>`+"\n",
			rankMedal(p.Rank), html.EscapeString(p.URL), html.EscapeString(p.FullName), badges.String(),
			html.EscapeString(descZh), enLine(descEn),
			html.EscapeString(orDash(p.Language)), p.StarsGained, p.Hotness)
	}
	return b.String()
}

func enLine(en string) string {
	if en == "" {
		return ""
	}
	return ` <span class="en">` + html.EscapeString(trunc(en, 60)) + `</span>`
}

func newsRowsHTML(rows []application.StoryRow) string {
	if len(rows) == 0 {
		return emptyRow("暂无数据——先运行一次 `githubhot run`")
	}
	var b strings.Builder
	for _, s := range rows {
		var chips strings.Builder
		for _, bd := range s.Badges {
			fmt.Fprintf(&chips, `<span class="%s">%s</span>`, badgeClass(bd), html.EscapeString(bd))
		}
		url := s.URL
		if url == "" {
			url = "#"
		}
		fmt.Fprintf(&b, `<div class="story">
<div class="story-head"><span class="rank">%s</span><a class="title" href="%s" target="_blank" rel="noopener">%s</a>%s</div>
<div class="summary">%s</div>
<div class="overview">%s</div>
<div class="meta">来源 %s · 评分 %.1f · 热度 %.1f · %s</div>
</div>`+"\n",
			rankMedal(s.Rank), html.EscapeString(url), html.EscapeString(s.TitleZh), chips.String(),
			html.EscapeString(s.SummaryZh),
			html.EscapeString(s.Overview),
			html.EscapeString(strings.Join(s.SourceNames, "、")), s.Score, s.Hotness,
			html.EscapeString(strings.Join(s.Tags, " / ")))
	}
	return b.String()
}

func fusionRowsHTML(rows []application.FusionRow) string {
	if len(rows) == 0 {
		return emptyRow("本轮未发现资讯与项目的直接对应")
	}
	var b strings.Builder
	for _, f := range rows {
		fmt.Fprintf(&b, `<div class="fusion-item"><span>%s</span><span class="x">×</span><a href="%s" target="_blank" rel="noopener">%s</a></div>`+"\n",
			html.EscapeString(trunc(f.News.TitleZh, 50)), html.EscapeString(f.Project.URL), html.EscapeString(f.Project.FullName))
	}
	return b.String()
}

// digestListHTML 侧栏期刊列表。
func digestListHTML(rows []application.DigestMeta) string {
	if len(rows) == 0 {
		return `<div class="empty">暂无期刊</div>`
	}
	var b strings.Builder
	for _, d := range rows {
		label := "日报"
		if d.Kind == "weekly" {
			label = "周报"
		} else if d.Kind == "monthly" {
			label = "月报"
		}
		fmt.Fprintf(&b, `<a href="/api/v1/digest/%s?format=raw" target="_blank" rel="noopener"><span>%s %s</span><span class="date">%s</span></a>`+"\n",
			html.EscapeString(d.Date), label, html.EscapeString(d.Date), "")
	}
	return b.String()
}

func emptyRow(msg string) string {
	return fmt.Sprintf(`<div class="empty">%s</div>`, html.EscapeString(msg))
}

func init() { log.SetFlags(log.LstdFlags | log.LUTC) }
