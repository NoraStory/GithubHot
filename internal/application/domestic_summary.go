package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// SettingDomesticSummaryEnabled 国内热榜 Top10 摘要开关的设置键（"1"=开，"0"=关，缺省开）。
const SettingDomesticSummaryEnabled = "domestic_summary_enabled"

// DomesticSummaryEnabled 读摘要开关，缺省开（设置缺失或读取失败时按开处理）。
func DomesticSummaryEnabled(ctx context.Context, d Deps) bool {
	if d.Settings == nil {
		return true
	}
	v, err := d.Settings.Get(ctx, SettingDomesticSummaryEnabled)
	if err != nil || v == "" {
		return true
	}
	return v == "1"
}

// GenerateDomesticSummary 为国内热榜 Top10 生成一段每日综述（LLM 一次调用）。
// 幂等：当天已有摘要或条目为空时直接跳过；开关关闭时跳过。
// 任何错误都只记日志不致命——摘要不是榜单的依赖。
func GenerateDomesticSummary(ctx context.Context, d Deps) {
	if d.Settings == nil || d.LLM == nil {
		return
	}
	now := d.Clock.Now()
	date := now.Format("2006-01-02")
	if existing, _ := d.Settings.DomesticSummary(ctx, date); existing != "" {
		return // 今天已生成
	}
	if !DomesticSummaryEnabled(ctx, d) {
		return
	}
	view, err := BuildDomesticView(ctx, d, 10)
	if err != nil || len(view.Items) == 0 {
		return
	}
	var b strings.Builder
	for _, it := range view.Items {
		fmt.Fprintf(&b, "%d. %s（%s）\n", it.Rank, it.Title, strings.Join(it.Sources, "、"))
	}
	system := "你是中文热点资讯编辑。根据用户给出的国内热榜 Top10 条目，写一段 150-220 字的今日热点综述：" +
		"概括今天最受关注的 2-4 个主题，点出事件间的关联，语气客观克制，不用标题、列表和 Markdown 标记，段落间用换行分隔。" +
		"严格只输出 JSON：{\"summary\": \"综述文本\"}"
	raw, err := d.LLM.ChatJSON(ctx, system, b.String(), d.LLM.ModelA(), 0.5)
	if err != nil {
		log.Printf("[domestic] 摘要生成失败（跳过，不影响榜单）: %v", err)
		return
	}
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || strings.TrimSpace(out.Summary) == "" {
		log.Printf("[domestic] 摘要 JSON 解析失败（跳过）: %.80q", raw)
		return
	}
	if err := d.Settings.SaveDomesticSummary(ctx, date, strings.TrimSpace(out.Summary)); err != nil {
		log.Printf("[domestic] 摘要落库失败: %v", err)
		return
	}
	log.Printf("[domestic] 已生成 %s 国内热榜综述（%d 字）", date, len([]rune(out.Summary)))
}

// TodayDomesticSummary 读今天的摘要（无则空串）。
func TodayDomesticSummary(ctx context.Context, d Deps) string {
	if d.Settings == nil {
		return ""
	}
	v, _ := d.Settings.DomesticSummary(ctx, d.Clock.Now().Format("2006-01-02"))
	return v
}
