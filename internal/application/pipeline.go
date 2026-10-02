package application

import (
	"context"
	"fmt"
)

// PipelineResult 一次完整流水线的产物。
type PipelineResult struct {
	DigestDate  string           `json:"digestDate"`
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

	if d.LLM == nil {
		return res, fmt.Errorf("LLM 必选：请配置 LLM_API_KEY / LLM_BASE_URL（OpenAI 兼容）后重试；本次采集与发现结果已入库，重跑不会浪费")
	}
	ss, err := SelectAndWrite(ctx, d, 120)
	if err != nil {
		return res, fmt.Errorf("精选写作: %w", err)
	}
	res.Select = ss

	cl, err := ClusterIntoStories(ctx, d)
	if err != nil {
		return res, fmt.Errorf("聚簇: %w", err)
	}
	res.Cluster = cl

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

	view, err := BuildHotView(ctx, d)
	if err != nil {
		return res, fmt.Errorf("榜单视图: %w", err)
	}
	res.View = view

	stats := DigestStats{
		Sources:   cs.DueSources,
		Collected: cs.Inserted,
		ModelA:    d.LLM.ModelA(),
		ModelB:    d.LLM.ModelB(),
	}
	dig, err := BuildDigest(ctx, d, d.Clock.Now(), stats)
	if err != nil {
		return res, fmt.Errorf("日报: %w", err)
	}
	res.DigestDate = dig.Date
	res.DigestMD = dig.Markdown
	res.ModelA = d.LLM.ModelA()
	res.ModelB = d.LLM.ModelB()

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
