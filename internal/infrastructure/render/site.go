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

// RenderIndex 渲染双榜静态页。
func (Site) RenderIndex(_ context.Context, v application.HotView) (string, error) {
	tpl, err := templates.ReadFile("templates/index.html")
	if err != nil {
		return "", fmt.Errorf("读取模板: %w", err)
	}
	out := string(tpl)
	out = strings.ReplaceAll(out, "{{GENERATED}}", v.Generated.Format("2006-01-02 15:04 MST"))
	out = strings.ReplaceAll(out, "{{GITHUB_ROWS}}", githubRowsHTML(v.GitHub))
	out = strings.ReplaceAll(out, "{{NEWS_ROWS}}", newsRowsHTML(v.News))
	out = strings.ReplaceAll(out, "{{FUSION_ROWS}}", fusionRowsHTML(v.Fusion))
	return out, nil
}

func githubRowsHTML(rows []application.ProjectRow) string {
	if len(rows) == 0 {
		return emptyRow("暂无数据——先运行一次 `githubhot run`")
	}
	var b strings.Builder
	for _, p := range rows {
		badges := ""
		for _, bd := range p.Badges {
			badges += fmt.Sprintf(`<span class="badge">%s</span>`, html.EscapeString(bd))
		}
		topics := ""
		for i, t := range p.Topics {
			if i >= 4 {
				break
			}
			topics += fmt.Sprintf(`<span class="topic">%s</span>`, html.EscapeString(t))
		}
		fmt.Fprintf(&b, `<tr>
<td class="rank">%d</td>
<td class="repo"><a href="%s" target="_blank" rel="noopener">%s</a>%s<div class="desc">%s</div><div class="topics">%s</div></td>
<td>%s</td>
<td class="num">+%d</td>
<td class="num hot">%.1f</td>
</tr>`+"\n",
			p.Rank, html.EscapeString(p.URL), html.EscapeString(p.FullName), badges,
			html.EscapeString(p.Description), topics,
			html.EscapeString(orDash(p.Language)), p.StarsGained, p.Hotness)
	}
	return b.String()
}

func newsRowsHTML(rows []application.StoryRow) string {
	if len(rows) == 0 {
		return emptyRow("暂无数据——先运行一次 `githubhot run`")
	}
	var b strings.Builder
	for _, s := range rows {
		badges := ""
		for _, bd := range s.Badges {
			badges += fmt.Sprintf(`<span class="badge">%s</span>`, html.EscapeString(bd))
		}
		url := s.URL
		if url == "" {
			url = "#"
		}
		fmt.Fprintf(&b, `<div class="story">
<div class="story-head"><span class="rank">%d</span> <a href="%s" target="_blank" rel="noopener">%s</a>%s</div>
<div class="summary">%s</div>
<div class="meta">来源 %s · 评分 %.1f · 热度 %.1f · %s</div>
</div>`+"\n",
			s.Rank, html.EscapeString(url), html.EscapeString(s.TitleZh), badges,
			html.EscapeString(s.SummaryZh),
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
		fmt.Fprintf(&b, `<div class="fusion"><span class="news">%s</span><span class="x">×</span><a href="%s" target="_blank" rel="noopener">%s</a></div>`+"\n",
			html.EscapeString(f.News.TitleZh), html.EscapeString(f.Project.URL), html.EscapeString(f.Project.FullName))
	}
	return b.String()
}

func emptyRow(msg string) string {
	return fmt.Sprintf(`<div class="empty">%s</div>`, html.EscapeString(msg))
}

// 编译期检查与日志占位。
var _ application.SiteRenderer = Site{}

func init() { log.SetFlags(log.LstdFlags | log.LUTC) }
