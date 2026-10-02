package render

import (
	"context"
	"embed"
	"fmt"
	"html"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

//go:embed templates/story.html
var storyTemplate embed.FS

// RenderStory 事件详情页。
func (Site) RenderStory(_ context.Context, v application.StoryDetailView) (string, error) {
	tpl, err := storyTemplate.ReadFile("templates/story.html")
	if err != nil {
		return "", fmt.Errorf("读取模板: %w", err)
	}
	out := string(tpl)
	title := v.Story.TitleZh
	if title == "" {
		title = "事件详情"
	}
	out = strings.ReplaceAll(out, "{{TITLE}}", html.EscapeString(title))
	out = strings.ReplaceAll(out, "{{URL}}", html.EscapeString(orDash(v.Story.URL)))
	out = strings.ReplaceAll(out, "{{OVERVIEW}}", html.EscapeString(v.Story.Overview))
	out = strings.ReplaceAll(out, "{{SUMMARY}}", html.EscapeString(v.Story.SummaryZh))
	out = strings.ReplaceAll(out, "{{BADGES}}", storyBadgesHTML(v))
	out = strings.ReplaceAll(out, "{{HISTORY}}", historyHTML(v.History))
	out = strings.ReplaceAll(out, "{{PROJECTS}}", storyProjectsHTML(v.Projects))
	out = strings.ReplaceAll(out, "{{MEMBERS}}", storyMembersHTML(v.Members))
	out = strings.ReplaceAll(out, "{{MEMBER_COUNT}}", fmt.Sprint(len(v.Members)))
	return out, nil
}

func storyBadgesHTML(v application.StoryDetailView) string {
	var b strings.Builder
	for _, bd := range v.Story.Badges {
		fmt.Fprintf(&b, `<span class="badge">%s</span>`, html.EscapeString(bd))
	}
	for _, fn := range v.Story.Projects {
		fmt.Fprintf(&b, `<span class="badge">GitHub: %s</span>`, html.EscapeString(fn))
	}
	if v.Story.Hotness > 0 {
		fmt.Fprintf(&b, `<span class="badge">热度 %.1f</span>`, v.Story.Hotness)
	}
	return b.String()
}

func historyHTML(points []story.HotnessPoint) string {
	if len(points) < 2 {
		return `<div class="empty">历史数据不足两次观测，暂无走势</div>`
	}
	max := 0.0
	for _, p := range points {
		if p.Hotness > max {
			max = p.Hotness
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="hist">`)
	for _, p := range points {
		pct := 0
		if max > 0 {
			pct = int(p.Hotness / max * 100)
			if pct < 4 {
				pct = 4
			}
		}
		fmt.Fprintf(&b, `<div class="bar" style="height:%d%%"><span>%.0f</span></div>`, pct, p.Hotness)
	}
	b.WriteString(`</div><div class="desc" style="text-align:center">最近热度快照（每次流水线运行记录一次）</div>`)
	return b.String()
}

func storyProjectsHTML(rows []application.ProjectRow) string {
	if len(rows) == 0 {
		return `<div class="empty">未关联项目</div>`
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>项目</th><th>24h ★</th><th>热度</th></tr></thead><tbody>`)
	for _, p := range rows {
		desc := p.DescriptionZh
		if desc == "" {
			desc = p.Description
		}
		fmt.Fprintf(&b, `<tr><td><a class="repo-name" style="color:var(--blue);font-weight:700" href="%s" target="_blank" rel="noopener">%s</a><div class="desc">%s</div></td><td class="num" style="color:var(--green);font-weight:700">+%d</td><td class="num hot">%.1f</td></tr>`+"\n",
			html.EscapeString(p.URL), html.EscapeString(p.FullName), html.EscapeString(trunc(desc, 70)), p.StarsGained, p.Hotness)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func storyMembersHTML(rows []application.DiagRow) string {
	if len(rows) == 0 {
		return `<div class="empty">无成员条目</div>`
	}
	var b strings.Builder
	for _, m := range rows {
		title := m.TitleZh
		if title == "" {
			title = m.Title
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td><a href="%s" target="_blank" rel="noopener">%s</a></td><td class="num">%.1f / %.1f</td><td class="desc">%s</td></tr>`+"\n",
			html.EscapeString(m.SourceName), html.EscapeString(m.URL), html.EscapeString(trunc(title, 46)),
			m.ScoreA, m.ScoreB, html.EscapeString(m.Published))
	}
	return b.String()
}
