package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/prompts"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
)

// 正文翻译参数：单轮处理上限与送入模型的最大字符数。
const (
	contentTranslateLimit = 8
	maxTranslateChars     = 6000
)

// ContentTranslateStats 原文翻译统计。
type ContentTranslateStats struct {
	Candidates int `json:"candidates"`
	Translated int `json:"translated"`
	LLMErrors  int `json:"llmErrors"`
}

// TranslateItemContents 原文本地存档用例：把已写作条目的正文（无正文时退化
// 为摘要）忠实翻译成中文并存回 items.content_zh——资讯标题链接到原站之外，
// 本地保留一份 AI 译文，原站打不开也能读，Agent 抓取也有干净语料。
// 已译条目自动跳过，任何中断下轮续跑；预算熔断返回 ErrBudgetExceeded。
func TranslateItemContents(ctx context.Context, d Deps, limit int) (ContentTranslateStats, error) {
	var stats ContentTranslateStats
	if d.LLM == nil {
		return stats, fmt.Errorf("LLM 必选：未配置 LLM 网关（LLM_API_KEY）")
	}
	if limit <= 0 {
		limit = contentTranslateLimit
	}
	pending, err := d.Items.PendingContentZh(ctx, limit)
	if err != nil {
		return stats, fmt.Errorf("读取待翻译条目: %w", err)
	}
	stats.Candidates = len(pending)
	for _, it := range pending {
		zh, err := translateOneContent(ctx, d, it)
		if err != nil {
			if errors.Is(err, ErrBudgetExceeded) {
				return stats, err
			}
			stats.LLMErrors++
			log.Printf("[translate] 正文翻译失败 %s: %v", it.ID, err)
			continue
		}
		if err := d.Items.SaveContentZh(ctx, it.ID, zh); err != nil {
			log.Printf("[translate] 译文写回失败 %s: %v", it.ID, err)
			continue
		}
		stats.Translated++
	}
	return stats, nil
}

// translateOneContent 单条翻译：正文优先，摘要兜底。
func translateOneContent(ctx context.Context, d Deps, it item.Item) (string, error) {
	raw := strings.TrimSpace(it.Content)
	if raw == "" {
		raw = strings.TrimSpace(it.Summary)
	}
	if raw == "" {
		return "", fmt.Errorf("无原文可译")
	}
	user := prompts.RenderPrompt(prompts.TranslateContent, "CONTENT", shared.Truncate(raw, maxTranslateChars))
	resp, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.2)
	if err != nil {
		return "", err
	}
	var out struct {
		Zh string `json:"zh"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return "", fmt.Errorf("译文 JSON 解析: %w", err)
	}
	zh := strings.TrimSpace(out.Zh)
	if zh == "" {
		return "", fmt.Errorf("译文为空")
	}
	return zh, nil
}
