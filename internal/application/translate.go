package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NoraStory/GithubHot/internal/domain/prompts"
)

// translateBatchSize 每次翻译调用的仓库数。
const translateBatchSize = 20

// TranslateProjectDescriptions 翻译用例：把 GitHub 项目英文描述批量翻译成
// 中文（一次调用翻译一批），只翻译缺失项，可断点续跑。
// 翻译结果持久化到 projects.description_zh —— 展示层直接读库，不重复消耗。
func TranslateProjectDescriptions(ctx context.Context, d Deps) (int, error) {
	all, err := d.Projects.All(ctx)
	if err != nil {
		return 0, fmt.Errorf("读取项目: %w", err)
	}
	var pending []string
	byName := map[string]string{} // fullName -> 原始描述
	for _, p := range all {
		if strings.TrimSpace(p.Description) == "" || strings.TrimSpace(p.DescriptionZh) != "" {
			continue
		}
		pending = append(pending, p.FullName)
		byName[p.FullName] = p.Description
	}
	if len(pending) == 0 {
		return 0, nil
	}

	written := 0
	for start := 0; start < len(pending); start += translateBatchSize {
		end := min(start+translateBatchSize, len(pending))
		batch := pending[start:end]
		var lines strings.Builder
		for _, name := range batch {
			fmt.Fprintf(&lines, "[%s] %s\n", name, byName[name])
		}
		user := prompts.RenderPrompt(prompts.TranslateDesc, "ITEMS", lines.String())
		raw, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.2)
		if err != nil {
			return written, err
		}
		var out struct {
			Items []struct {
				FullName string `json:"fullName"`
				Zh       string `json:"zh"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			continue
		}
		for _, it := range out.Items {
			zh := strings.TrimSpace(it.Zh)
			if zh == "" || byName[it.FullName] == "" {
				continue
			}
			if err := d.Projects.SaveDescriptionZh(ctx, it.FullName, zh); err == nil {
				written++
			}
		}
	}
	return written, nil
}
