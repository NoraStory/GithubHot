package render

import (
	"context"
	"embed"
	"fmt"
	"html"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
)

//go:embed templates/console.html templates/search.html
var extraTemplates embed.FS

// Console 控制台页。
func (Site) RenderConsole(_ context.Context, v application.ConsoleView) (string, error) {
	tpl, err := extraTemplates.ReadFile("templates/console.html")
	if err != nil {
		return "", fmt.Errorf("读取模板: %w", err)
	}
	out := string(tpl)
	out = strings.ReplaceAll(out, "{{USAGE_BUDGET}}", fmt.Sprintf("%d", v.Usage.BudgetTokens))
	out = strings.ReplaceAll(out, "{{USAGE_PROMPT}}", fmt.Sprintf("%d", v.Usage.UsedPrompt))
	out = strings.ReplaceAll(out, "{{USAGE_COMPLETION}}", fmt.Sprintf("%d", v.Usage.UsedCompletion))
	out = strings.ReplaceAll(out, "{{USAGE_TOTAL}}", fmt.Sprintf("%d", v.Usage.UsedTotal))
	out = strings.ReplaceAll(out, "{{USAGE_COST}}", fmt.Sprintf("%.4f", v.Usage.EstimatedCost))
	budgetPct := 0
	if v.Usage.BudgetTokens > 0 {
		budgetPct = v.Usage.UsedTotal * 100 / v.Usage.BudgetTokens
		if budgetPct > 100 {
			budgetPct = 100
		}
	}
	out = strings.ReplaceAll(out, "{{BUDGET_PCT}}", fmt.Sprintf("%d", budgetPct))
	if v.Usage.Exceeded {
		out = strings.ReplaceAll(out, "{{BUDGET_STATE}}", `<span class="badge" style="background:#b62324">已熔断</span>`)
	} else if v.Usage.BudgetTokens > 0 {
		out = strings.ReplaceAll(out, "{{BUDGET_STATE}}", `<span class="badge">正常</span>`)
	} else {
		out = strings.ReplaceAll(out, "{{BUDGET_STATE}}", `<span class="topic">未启用预算</span>`)
	}
	out = strings.ReplaceAll(out, "{{USAGE_PHASES}}", phaseRowsHTML(v.Usage.ByPhase))
	out = strings.ReplaceAll(out, "{{USAGE_DAYS}}", dayRowsHTML(v.Usage.Days))
	out = strings.ReplaceAll(out, "{{RUNS}}", runsRowsHTML(v.Runs))
	out = strings.ReplaceAll(out, "{{STAGE}}", html.EscapeString(v.Stage))
	out = strings.ReplaceAll(out, "{{STAGE_OPTIONS}}", stageOptionsHTML(v.Stages, v.Stage))
	out = strings.ReplaceAll(out, "{{DIAGNOSTICS}}", diagRowsHTML(v.Diagnostics))
	out = strings.ReplaceAll(out, "{{SOURCES}}", sourceRowsHTML(v.Sources))
	out = strings.ReplaceAll(out, "{{DIGESTS}}", digestRowsHTML(v.Digests))
	return out, nil
}

// Search 搜索结果页。
func (Site) RenderSearch(_ context.Context, v application.SearchView) (string, error) {
	tpl, err := extraTemplates.ReadFile("templates/search.html")
	if err != nil {
		return "", fmt.Errorf("读取模板: %w", err)
	}
	out := string(tpl)
	out = strings.ReplaceAll(out, "{{QUERY}}", html.EscapeString(v.Query))
	out = strings.ReplaceAll(out, "{{TOOK}}", html.EscapeString(v.Took))
	out = strings.ReplaceAll(out, "{{RESULTS}}", searchRowsHTML(v.Results))
	return out, nil
}

func phaseRowsHTML(rows []application.PhaseUsage) string {
	if len(rows) == 0 {
		return emptyRow("暂无调用记录")
	}
	var b strings.Builder
	for _, p := range rows {
		fmt.Fprintf(&b, `<tr><td>%s</td><td class="num">%d</td><td class="num">%d</td><td class="num">%d</td></tr>`+"\n",
			html.EscapeString(p.Phase), p.Calls, p.PromptTokens, p.CompletionTokens)
	}
	return b.String()
}

func dayRowsHTML(rows []application.DayUsage) string {
	if len(rows) == 0 {
		return emptyRow("暂无记录")
	}
	var b strings.Builder
	for _, d := range rows {
		cost := fmt.Sprintf("%.4f", 0.0)
		_ = cost
		fmt.Fprintf(&b, `<tr><td>%s</td><td class="num">%d</td><td class="num">%d</td><td class="num hot">%d</td></tr>`+"\n",
			html.EscapeString(d.Day), d.PromptTokens, d.CompletionTokens, d.PromptTokens+d.CompletionTokens)
	}
	return b.String()
}

func runsRowsHTML(rows []application.RunRow) string {
	if len(rows) == 0 {
		return emptyRow("尚无运行记录")
	}
	var b strings.Builder
	for _, r := range rows {
		statusBadge := `<span class="badge" style="background:#238636">ok</span>`
		if r.Status != "ok" {
			statusBadge = `<span class="badge" style="background:#b62324">error</span>`
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td class="num">%.0fs</td><td class="num">%d</td><td class="num">%d</td><td class="num">%d</td></tr>`+"\n",
			html.EscapeString(r.StartedAt), statusBadge, r.Duration, r.Collected, r.Written, r.Stories)
	}
	return b.String()
}

func diagRowsHTML(rows []application.DiagRow) string {
	if len(rows) == 0 {
		return emptyRow("该阶段暂无条目")
	}
	var b strings.Builder
	for _, r := range rows {
		title := r.TitleZh
		if title == "" {
			title = r.Title
		}
		link := title
		if r.URL != "" {
			link = fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener">%s</a>`, html.EscapeString(r.URL), html.EscapeString(trunc(title, 40)))
		}
		fmt.Fprintf(&b, `<tr><td><span class="topic">%s</span></td><td>%s<div class="desc">%s</div></td><td class="num">%.1f / %.1f</td><td class="desc">%s</td></tr>`+"\n",
			html.EscapeString(r.Stage), link, html.EscapeString(r.SourceName),
			r.ScoreA, r.ScoreB, html.EscapeString(trunc(r.Reason, 60)))
	}
	return b.String()
}

func sourceRowsHTML(rows []application.SourceInfo) string {
	if len(rows) == 0 {
		return emptyRow("无信源")
	}
	var b strings.Builder
	for _, s := range rows {
		state := `<span class="badge" style="background:#238636">启用</span>`
		if !s.Enabled {
			state = `<span class="topic">停用</span>`
		}
		adapter := "builtin"
		if s.Adapter != "" {
			adapter = s.Adapter
		}
		fmt.Fprintf(&b, `<tr><td>%s<div class="desc">%s · %s</div></td><td>%s</td><td>%s</td></tr>`+"\n",
			html.EscapeString(s.Name), html.EscapeString(s.Kind), html.EscapeString(s.Tier), state, html.EscapeString(adapter))
	}
	return b.String()
}

func digestRowsHTML(rows []application.DigestMeta) string {
	var b strings.Builder
	for _, d := range rows {
		label := "日报"
		if d.Kind == "weekly" {
			label = "周报"
		} else if d.Kind == "monthly" {
			label = "月报"
		}
		fmt.Fprintf(&b, `<tr><td><span class="topic">%s</span></td><td><a href="/api/v1/digest/%s?format=raw" target="_blank" rel="noopener">%s</a></td></tr>`+"\n",
			label, html.EscapeString(d.Date), html.EscapeString(d.Date))
	}
	return b.String()
}

func searchRowsHTML(rows []application.SearchResult) string {
	if len(rows) == 0 {
		return emptyRow("没有匹配结果")
	}
	var b strings.Builder
	for _, r := range rows {
		kind := "AI 资讯"
		if r.Kind == "story" {
			kind = "事件"
		}
		fmt.Fprintf(&b, `<div class="story">
<div class="story-head"><span class="topic">%s</span> <a href="%s" target="_blank" rel="noopener">%s</a></div>
<div class="summary">%s</div>
<div class="meta">%s · 热度 %.1f</div>
</div>`+"\n",
			html.EscapeString(kind), html.EscapeString(orDash(r.URL)), html.EscapeString(trunc(r.TitleZh, 60)),
			html.EscapeString(trunc(r.SummaryZh, 120)),
			html.EscapeString(r.SourceNames), r.Hotness)
	}
	return b.String()
}

func stageOptionsHTML(stages []string, selected string) string {
	var b strings.Builder
	for _, st := range stages {
		sel := ""
		if st == selected {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`+"\n", html.EscapeString(st), sel, html.EscapeString(stageLabel(st)))
	}
	return b.String()
}

func stageLabel(s string) string {
	return map[string]string{
		"new": "新采集", "prefiltered": "已预筛", "filtered": "预筛通过",
		"dropped": "预筛淘汰", "scored": "评分通过", "rejected": "评分淘汰",
		"written": "已写作",
	}[s]
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
