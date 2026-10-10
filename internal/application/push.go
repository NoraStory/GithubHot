package application

import (
	"context"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/github"
	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// PushItem 脚本推送用例：外部脚本通过 CLI/API 把一条资料直接写入
// script 类信源（AIHOT 的"你自己脚本推送进来的内容"）。URL 判重照常生效。
func PushItem(ctx context.Context, d Deps, sourceID, rawURL, title, summary string) (item.Item, error) {
	var zero item.Item
	src, err := d.Sources.FindByID(ctx, sourceID)
	if err != nil {
		return zero, fmt.Errorf("信源 %s 不存在（kind 需为 script）: %w", sourceID, err)
	}
	if src.Kind != source.KindScript {
		return zero, fmt.Errorf("推送目标信源 %s 的 kind 必须是 script（当前 %s）", sourceID, src.Kind)
	}
	now := d.Clock.Now()
	key, err := shared.DedupeKey(rawURL)
	if err != nil {
		return zero, err
	}
	it, err := item.New(key, src.ID, string(src.Tier), rawURL, title, now, now)
	if err != nil {
		return zero, err
	}
	it.Summary = summary
	inserted, err := d.Items.Upsert(ctx, *it)
	if err != nil {
		return zero, err
	}
	if !inserted {
		return zero, fmt.Errorf("重复条目（已存在）: %s", rawURL)
	}
	return *it, nil
}

// StoryDetail 事件详情视图（事件页 + API 共用）。
type StoryDetailView struct {
	Story    StoryRow             `json:"story"`
	Members  []DiagRow            `json:"members"`
	Projects []ProjectRow         `json:"projects"`
	History  []story.HotnessPoint `json:"history"`
}

// BuildStoryDetail 事件详情：综述 + 成员资料 + 关联项目 + 热度历史。
func BuildStoryDetail(ctx context.Context, d Deps, id string) (*StoryDetailView, error) {
	s, err := d.Stories.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("读取事件: %w", err)
	}
	v := &StoryDetailView{
		Story:    buildStoryRow(ctx, d, s, 0, mustSourceNames(ctx, d), d.Clock.Now()),
		Members:  []DiagRow{},
		Projects: []ProjectRow{},
		History:  []story.HotnessPoint{},
	}
	var ids []string
	for _, m := range s.Members {
		if m.ItemID != "" {
			ids = append(ids, m.ItemID)
		}
	}
	if len(ids) > 0 {
		items, err := d.Items.FindByIDs(ctx, ids)
		if err == nil {
			names, _ := loadSourceNames(ctx, d)
			for _, it := range items {
				v.Members = append(v.Members, DiagRow{
					ID: it.ID, Stage: string(it.Selection.Stage),
					SourceName: names[it.SourceID], Title: it.Title, TitleZh: it.Selection.TitleZh,
					URL: it.URL, Reason: it.Selection.Reason,
					ScoreA: it.Selection.ScoreA, ScoreB: it.Selection.ScoreB,
					Published: it.PublishedAt.Format("2006-01-02 15:04"),
					ContentZh: it.ContentZh,
				})
			}
		}
	}
	now := d.Clock.Now()
	// 详情页项目行热度保持纯快照口径（不接多源共振）：本事件自身就引用了这些
	// 项目，共振恒 ≥1 无区分度；共振加权只作用于榜单排序（BuildHotView）。
	snapAll, err := d.Projects.AllSnapshotsSince(ctx, now.Add(-7*24*time.Hour))
	if err == nil {
		for _, fn := range s.Projects {
			if snaps := snapAll[fn]; len(snaps) > 0 {
				v.Projects = append(v.Projects, ProjectRow{
					FullName: fn, URL: "https://github.com/" + fn,
					Hotness:     projectHotnessOf(snaps, now),
					StarsGained: starsGained(snaps, now),
				})
			}
		}
	}
	v.History, _ = d.Stories.HotnessHistory(ctx, id, 100)
	if v.History == nil {
		v.History = []story.HotnessPoint{}
	}
	return v, nil
}

func mustSourceNames(ctx context.Context, d Deps) map[string]string {
	names, _ := loadSourceNames(ctx, d)
	return names
}

func projectHotnessOf(snaps []github.Snapshot, now time.Time) float64 {
	return github.Hotness(github.HotnessInput{Snapshots: snaps, Now: now, Window: 24 * time.Hour})
}

func starsGained(snaps []github.Snapshot, now time.Time) int {
	return github.StarsGainedIn(snaps, now, 24*time.Hour)
}

// SelectBenchSample 校准样本（JSONL 一行一条）。
type SelectBenchSample struct {
	Text  string `json:"text"`
	Label string `json:"label"` // pass / drop
}

// SelectBenchReport 校准报告。
type SelectBenchReport struct {
	Total     int      `json:"total"`
	TP        int      `json:"tp"`
	FP        int      `json:"fp"`
	FN        int      `json:"fn"`
	TN        int      `json:"tn"`
	Precision float64  `json:"precision"`
	Recall    float64  `json:"recall"`
	F1        float64  `json:"f1"`
	Mismatch  []string `json:"mismatches"`
}

// RunSelectBench 精选校准（AIHOT SelectBench 同思路）：用标注样本回测
// 当前预筛提示词与门槛，输出精确率/召回率/F1 与误判清单。
// 样本格式：每行一个 JSON {"text":"...","label":"pass|drop"}。
func RunSelectBench(ctx context.Context, d Deps, samples []SelectBenchSample) (*SelectBenchReport, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("样本为空")
	}
	now := d.Clock.Now()
	rep := &SelectBenchReport{Total: len(samples)}
	for i, s := range samples {
		// 直接构造条目（样本没有真实 URL，不走构造器校验）
		it := item.Item{
			ID: fmt.Sprintf("bench-%d", i), Title: s.Text,
			SourceTier: string(source.TierMedia), FetchedAt: now,
			Selection: item.Selection{Stage: item.StageNew},
		}
		results, err := prefilterBatchCall(ctx, d, []item.Item{it})
		if err != nil {
			return rep, fmt.Errorf("预筛调用失败（样本 %d）: %w", i, err)
		}
		modelPass := results[it.ID].Pass
		labelPass := s.Label == "pass"
		switch {
		case labelPass && modelPass:
			rep.TP++
		case labelPass && !modelPass:
			rep.FN++
			rep.Mismatch = append(rep.Mismatch, fmt.Sprintf("漏收（应为 pass）: %s", truncRunes(s.Text, 40)))
		case !labelPass && modelPass:
			rep.FP++
			rep.Mismatch = append(rep.Mismatch, fmt.Sprintf("误收（应为 drop）: %s", truncRunes(s.Text, 40)))
		default:
			rep.TN++
		}
	}
	if rep.TP+rep.FP > 0 {
		rep.Precision = float64(rep.TP) / float64(rep.TP+rep.FP)
	}
	if rep.TP+rep.FN > 0 {
		rep.Recall = float64(rep.TP) / float64(rep.TP+rep.FN)
	}
	if rep.Precision+rep.Recall > 0 {
		rep.F1 = 2 * rep.Precision * rep.Recall / (rep.Precision + rep.Recall)
	}
	return rep, nil
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
