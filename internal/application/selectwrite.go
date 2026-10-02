package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/prompts"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// SelectWriteStats 精选写作统计。
type SelectWriteStats struct {
	Candidates  int `json:"candidates"` // 进入预筛的条数
	Prefiltered int `json:"prefiltered"`
	Dropped     int `json:"dropped"`
	Scored      int `json:"scored"`
	Rejected    int `json:"rejected"`
	Written     int `json:"written"`
	LLMErrors   int `json:"llmErrors"`
}

// 选型流程参数。
const (
	prefilterBatch  = 15 // 预筛每批条数
	writeLimit      = 60 // 单轮写作上限，控制 token 成本
	llmWorkers      = 3  // LLM 并发度
	maxContentChars = 4000
)

// SelectAndWrite 精选用例（LLM 必选）：预筛 → 同一标准独立两次评分 →
// 过门槛者中文写作。三个阶段都写回条目的 Selection，可断点续跑：
// 已处理过的条目（非 new 阶段）自动跳过。
func SelectAndWrite(ctx context.Context, d Deps, limit int) (SelectWriteStats, error) {
	var stats SelectWriteStats
	if d.LLM == nil {
		return stats, fmt.Errorf("LLM 必选：未配置 LLM 网关（LLM_API_KEY）")
	}
	if limit <= 0 {
		limit = 120
	}

	pending, err := d.Items.ByStage(ctx, []item.Stage{item.StageNew}, limit)
	if err != nil {
		return stats, fmt.Errorf("读取待精选条目: %w", err)
	}
	if len(pending) == 0 {
		return stats, nil
	}
	stats.Candidates = len(pending)

	// ---------- 阶段 1：预筛（批量） ----------
	passed := make([]item.Item, 0, len(pending))
	for start := 0; start < len(pending); start += prefilterBatch {
		end := min(start+prefilterBatch, len(pending))
		batch := pending[start:end]
		results, err := prefilterBatchCall(ctx, d, batch)
		if err != nil {
			stats.LLMErrors++
			log.Printf("[select] 预筛批次失败，整批按淘汰处理: %v", err)
			for _, it := range batch {
				markDropped(ctx, d, it, "预筛调用失败，宁缺毋滥")
				stats.Dropped++
			}
			continue
		}
		for _, it := range batch {
			if r, ok := results[it.ID]; ok && r.Pass {
				stats.Prefiltered++
				sel := item.Selection{Stage: item.StageFiltered, Pass: true, Reason: r.Reason}
				_ = d.Items.UpdateSelection(ctx, it.ID, sel)
				it.Selection = sel
				passed = append(passed, it)
			} else if r, ok := results[it.ID]; ok {
				stats.Dropped++
				markDropped(ctx, d, it, r.Reason)
			} else {
				stats.Dropped++
				markDropped(ctx, d, it, "预筛未返回该条")
			}
		}
	}

	// ---------- 阶段 2：双评分（逐条，两次独立调用） ----------
	type scoredItem struct {
		it   item.Item
		a, b float64
	}
	scored := make([]scoredItem, 0, len(passed))
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, llmWorkers)
	)
	for _, it := range passed {
		wg.Add(1)
		go func(it item.Item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			a, b, err := doubleScore(ctx, d, it)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				stats.LLMErrors++
				log.Printf("[select] 评分失败 %s: %v", it.ID, err)
				stats.Rejected++
				return
			}
			threshold := source.Tier(it.SourceTier).ScoreThreshold()
			avg := (a + b) / 2
			if avg >= threshold && a >= 4 && b >= 4 {
				scored = append(scored, scoredItem{it: it, a: a, b: b})
			} else {
				stats.Rejected++
				_ = d.Items.UpdateSelection(ctx, it.ID, item.Selection{
					Stage: item.StageRejected, Pass: false, ScoreA: a, ScoreB: b,
					Reason: fmt.Sprintf("均分 %.1f 低于门槛 %.1f", avg, threshold),
				})
			}
		}(it)
	}
	wg.Wait()
	stats.Scored = len(scored)

	// ---------- 阶段 3：中文写作（逐条，限流） ----------
	for i, sc := range scored {
		if i >= writeLimit {
			break
		}
		w, err := writeChinese(ctx, d, sc.it)
		if err != nil {
			stats.LLMErrors++
			log.Printf("[select] 写作失败 %s: %v", sc.it.ID, err)
			continue
		}
		sel := item.Selection{
			Stage: item.StageWritten, Pass: true,
			ScoreA: sc.a, ScoreB: sc.b,
			TitleZh: w.TitleZh, SummaryZh: w.SummaryZh, ReasonZh: w.ReasonZh, Tags: w.Tags,
		}
		if uerr := d.Items.UpdateSelection(ctx, sc.it.ID, sel); uerr != nil {
			log.Printf("[select] 写回失败 %s: %v", sc.it.ID, uerr)
			continue
		}
		stats.Written++
	}
	return stats, nil
}

type prefilterResult struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

func prefilterBatchCall(ctx context.Context, d Deps, batch []item.Item) (map[string]prefilterResult, error) {
	var lines strings.Builder
	for _, it := range batch {
		fmt.Fprintf(&lines, "[%s] %s\n", it.ID, it.TextForLLM(600))
	}
	user := prompts.RenderPrompt(prompts.Prefilter, "ITEMS", lines.String())
	raw, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.2)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			ID     string `json:"id"`
			Pass   bool   `json:"pass"`
			Reason string `json:"reason"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("预筛 JSON 解析: %w", err)
	}
	out := make(map[string]prefilterResult, len(parsed.Results))
	for _, r := range parsed.Results {
		out[r.ID] = prefilterResult{Pass: r.Pass, Reason: r.Reason}
	}
	return out, nil
}

func doubleScore(ctx context.Context, d Deps, it item.Item) (a, b float64, err error) {
	user := prompts.RenderPrompt(prompts.Score, "ITEM", it.TextForLLM(maxContentChars))
	type scoreOut struct {
		Score float64 `json:"score"`
	}
	parse := func(raw string) (float64, error) {
		var s scoreOut
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return 0, err
		}
		if s.Score < 0 {
			s.Score = 0
		}
		if s.Score > 10 {
			s.Score = 10
		}
		return s.Score, nil
	}
	rawA, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.2)
	if err != nil {
		return 0, 0, fmt.Errorf("模型A: %w", err)
	}
	a, err = parse(rawA)
	if err != nil {
		return 0, 0, fmt.Errorf("模型A解析: %w", err)
	}
	rawB, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelB(), 0.5)
	if err != nil {
		return 0, 0, fmt.Errorf("模型B: %w", err)
	}
	b, err = parse(rawB)
	if err != nil {
		return 0, 0, fmt.Errorf("模型B解析: %w", err)
	}
	return a, b, nil
}

type writeOut struct {
	TitleZh   string   `json:"titleZh"`
	SummaryZh string   `json:"summaryZh"`
	ReasonZh  string   `json:"reasonZh"`
	Tags      []string `json:"tags"`
}

func writeChinese(ctx context.Context, d Deps, it item.Item) (writeOut, error) {
	var w writeOut
	user := prompts.RenderPrompt(prompts.Write, "ITEM", it.TextForLLM(maxContentChars))
	raw, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.3)
	if err != nil {
		return w, err
	}
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return w, fmt.Errorf("写作 JSON 解析: %w", err)
	}
	if strings.TrimSpace(w.TitleZh) == "" {
		return w, fmt.Errorf("写作返回空标题")
	}
	return w, nil
}

func markDropped(ctx context.Context, d Deps, it item.Item, reason string) {
	_ = d.Items.UpdateSelection(ctx, it.ID, item.Selection{
		Stage: item.StageDropped, Pass: false, Reason: reason,
	})
}
