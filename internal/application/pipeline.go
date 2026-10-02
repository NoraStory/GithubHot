package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/digest"
)

// PipelineResult 一次完整流水线的产物。
type PipelineResult struct {
	DigestDate  string           `json:"digestDate"`
	WeeklyDate  string           `json:"weeklyDate,omitempty"`
	MonthlyDate string           `json:"monthlyDate,omitempty"`
	DigestMD    string           `json:"-"`
	SiteHTML    string           `json:"-"`
	View        HotView          `json:"-"`
	Collect     CollectStats     `json:"collect"`
	Discover    DiscoverStats    `json:"discover"`
	Select      SelectWriteStats `json:"select"`
	Cluster     ClusterStats     `json:"cluster"`
	Fusion      FusionStats      `json:"fusion"`
	Rank        RankStats        `json:"rank"`
	DurationSec float64          `json:"durationSeconds"`
	ModelA      string           `json:"modelA"`
	ModelB      string           `json:"modelB"`
}

// RunPipeline 完整流水线：种子信源 → 采集 → GitHub 双轨发现 → 精选写作 →
// 聚簇 → 融合链接 → 热度 → 日报与双榜视图。
//
// LLM 必选：LLM 网关缺失时在精选阶段报错终止（采集与发现已完成并入库，
// 下次运行自动续跑，不浪费已抓数据）。
func RunPipeline(ctx context.Context, d Deps) (*PipelineResult, error) {
	started := d.Clock.Now()
	res := &PipelineResult{}
	setPhase := func(phase string) {
		if ps, ok := d.LLM.(PhaseSetter); ok {
			ps.SetPhase(phase)
		}
	}
	budgetHit := func(stage string) bool {
		if bc, ok := d.LLM.(BudgetChecker); ok && bc.BudgetExceeded() {
			fmt.Printf("[pipeline] 预算熔断：跳过 %s 阶段（已有数据照常出榜出日报）\n", stage)
			return true
		}
		return false
	}

	imported, err := SeedSources(ctx, d)
	if err != nil {
		return res, fmt.Errorf("种子信源: %w", err)
	}
	if imported > 0 {
		fmt.Printf("[seed] 导入默认信源 %d 个\n", imported)
	}

	cs, err := CollectSources(ctx, d, 6)
	if err != nil {
		return res, fmt.Errorf("采集: %w", err)
	}
	res.Collect = cs

	ds, err := DiscoverProjects(ctx, d)
	if err != nil {
		return res, fmt.Errorf("GitHub 发现: %w", err)
	}
	res.Discover = ds

	if d.LLM != nil && !budgetHit("描述翻译") {
		setPhase("translate")
		if n, err := TranslateProjectDescriptions(ctx, d); err != nil && !errors.Is(err, ErrBudgetExceeded) {
			fmt.Printf("[pipeline] 描述翻译失败（跳过，展示英文原文）: %v\n", err)
		} else if n > 0 {
			fmt.Printf("[pipeline] 翻译项目描述 %d 条\n", n)
		}
	}

	if d.LLM == nil {
		return res, fmt.Errorf("LLM 必选：请配置 LLM_API_KEY / LLM_BASE_URL（OpenAI 兼容）后重试；本次采集与发现结果已入库，重跑不会浪费")
	}

	setPhase("prefilter")
	ss, err := SelectAndWrite(ctx, d, 120)
	if err != nil {
		if errors.Is(err, ErrBudgetExceeded) {
			fmt.Printf("[pipeline] 预算熔断：精选中止，已有数据照常出榜出日报\n")
		} else {
			return res, fmt.Errorf("精选写作: %w", err)
		}
	}
	res.Select = ss

	if budgetHit("聚簇") {
		return finishPipeline(ctx, d, res, started)
	}
	setPhase("cluster")
	cl, err := ClusterIntoStories(ctx, d)
	if err != nil {
		return res, fmt.Errorf("聚簇: %w", err)
	}
	res.Cluster = cl

	if budgetHit("融合") {
		return finishPipeline(ctx, d, res, started)
	}
	setPhase("fusion")
	fu, err := LinkFusion(ctx, d, 15, 15)
	if err != nil {
		// 融合是增强环节，失败不阻断日报
		fmt.Printf("[pipeline] 融合链接失败（跳过）: %v\n", err)
	}
	res.Fusion = fu

	rs, err := RankStories(ctx, d)
	if err != nil {
		return res, fmt.Errorf("热度: %w", err)
	}
	res.Rank = rs

	if !budgetHit("事件综述") {
		setPhase("overview")
		if n, err := SynthesizeOverviews(ctx, d, 10); err != nil {
			if !errors.Is(err, ErrBudgetExceeded) {
				fmt.Printf("[pipeline] 事件综述失败（跳过）: %v\n", err)
			}
		} else if n > 0 {
			fmt.Printf("[pipeline] 生成事件综述 %d 篇\n", n)
		}
	}
	return finishPipeline(ctx, d, res, started)
}

// finishPipeline 排名后的收尾：视图 → 日报 → 周报/月报 → 站点。
func finishPipeline(ctx context.Context, d Deps, res *PipelineResult, started time.Time) (*PipelineResult, error) {
	now := d.Clock.Now()
	view, err := BuildHotView(ctx, d, digest.KindDaily)
	if err != nil {
		return res, fmt.Errorf("榜单视图: %w", err)
	}
	res.View = view

	stats := DigestStats{
		Sources:   res.Collect.DueSources,
		Collected: res.Collect.Inserted,
		ModelA:    d.LLM.ModelA(),
		ModelB:    d.LLM.ModelB(),
	}
	dig, err := BuildDigest(ctx, d, digest.KindDaily, now, stats)
	if err != nil {
		return res, fmt.Errorf("日报: %w", err)
	}
	res.DigestDate = dig.Date
	res.DigestMD = dig.Markdown
	res.ModelA = d.LLM.ModelA()
	res.ModelB = d.LLM.ModelB()

	// 周报（周一）/ 月报（每月 1 日）：与日报同轮生成
	loc := shanghaiLoc()
	local := now.In(loc)
	if local.Weekday() == time.Monday {
		if wd, werr := BuildDigest(ctx, d, digest.KindWeekly, now, stats); werr == nil {
			res.WeeklyDate = wd.Date
			fmt.Printf("[pipeline] 周报已生成：%s\n", wd.Date)
		}
	}
	if local.Day() == 1 {
		if md, merr := BuildDigest(ctx, d, digest.KindMonthly, now, stats); merr == nil {
			res.MonthlyDate = md.Date
			fmt.Printf("[pipeline] 月报已生成：%s\n", md.Date)
		}
	}

	if d.SiteRenderer != nil {
		html, serr := d.SiteRenderer.RenderIndex(ctx, view)
		if serr != nil {
			fmt.Printf("[pipeline] 站点渲染失败（跳过）: %v\n", serr)
		}
		res.SiteHTML = html
	}

	res.DurationSec = d.Clock.Now().Sub(started).Seconds()
	return res, nil
}
