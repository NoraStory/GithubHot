package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// TestPipelineEndToEnd 是本项目的验收测试：
// 采集 → GitHub 双轨发现 → 精选写作 → 聚簇（两条同事件报道合并）→
// 融合链接 → 热度 → 日报。全部依赖为假实现，可重复运行。
func TestPipelineEndToEnd(t *testing.T) {
	now := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	d := Deps{
		Sources:        newMemSources(),
		Items:          newMemItems(),
		Projects:       newMemProjects(),
		Stories:        newMemStories(),
		Digests:        newMemDigests(),
		LLM:            &fakeLLM{},
		GitHub:         fakeGitHub{},
		Fetchers:       fakeRegistry{f: fakeFetcher{now: now}},
		DigestRenderer: fakeDigestRenderer{},
		SiteRenderer:   fakeSiteRenderer{},
		Clock:          fixedClock{t: now},
	}
	_ = d.Sources.Save(context.Background(), source.Source{
		ID: "rss-test", Name: "测试源", Kind: source.KindRSS, Tier: source.TierFirstParty,
		Config: map[string]string{"url": "https://example.com/feed.xml"}, Enabled: true, CreatedAt: now,
	})

	// 预置"昨天"的项目快照，模拟多日运行：agent-kit 昨日 500★ → 今日双轨观测 505★
	projects := d.Projects.(*memProjects)
	_ = projects.Upsert(context.Background(), githubProject(t, "openai/agent-kit", 500, now.Add(-20*time.Hour)))
	_ = projects.AddSnapshot(context.Background(), snapshotOf("openai/agent-kit", 500, now.Add(-20*time.Hour)))

	res, err := RunPipeline(context.Background(), d)
	if err != nil {
		t.Fatalf("流水线失败: %v", err)
	}

	// 1. 采集：4 条进入，其中 1 条 URL 与另一条不同但内容不同——全部入库
	if res.Collect.Inserted != 4 {
		t.Fatalf("应采集 4 条，得到 %d", res.Collect.Inserted)
	}

	// 2. 精选：广告被预筛淘汰；其余 3 条通过双评分并完成中文写作
	if res.Select.Dropped != 1 {
		t.Fatalf("应淘汰 1 条广告，得到 %d", res.Select.Dropped)
	}
	if res.Select.Written != 3 {
		t.Fatalf("应完成 3 条写作，得到 %d", res.Select.Written)
	}

	// 3. 聚簇：两条 Agent框架 报道合并为同一事件 → 2 个新事件 + 1 次合并
	if res.Cluster.NewStories != 2 {
		t.Fatalf("应产生 2 个事件（Agent框架 + 向量数据库），得到 %d", res.Cluster.NewStories)
	}
	if res.Cluster.Merged != 1 {
		t.Fatalf("同事件报道应合并 1 次，得到 %d", res.Cluster.Merged)
	}

	// 4. GitHub 双轨：search 1 + trending 2；agent-kit 已预置（走合并），tiny-db 为新项目
	if res.Discover.NewProjects != 1 || res.Discover.Merged != 1 || res.Discover.SearchRepos != 1 || res.Discover.TrendingRepos != 2 {
		t.Fatalf("发现统计不符: %+v", res.Discover)
	}

	// 5. 融合：第一条资讯事件应链接到 openai/agent-kit
	if res.Fusion.Links != 1 {
		t.Fatalf("应建立 1 条融合链接，得到 %d", res.Fusion.Links)
	}

	// 6. 日报包含榜单与融合行
	if !strings.Contains(res.DigestMD, "openai/agent-kit") {
		t.Fatal("日报应包含 GitHub 项目榜")
	}
	if !strings.Contains(res.DigestMD, "Agent框架2.0发布") {
		t.Fatal("日报应包含中文资讯标题")
	}
	if !strings.Contains(res.DigestMD, "×") {
		t.Fatal("日报应包含融合观察行")
	}

	// 7. 榜单：项目榜 2 项，agent-kit 因日增 5 星 + trending #1 排第一
	if len(res.View.GitHub) != 2 {
		t.Fatalf("项目榜应有 2 项，得到 %d", len(res.View.GitHub))
	}
	if res.View.GitHub[0].FullName != "openai/agent-kit" {
		t.Fatalf("agent-kit 应排第一（trending+增长），得到 %s", res.View.GitHub[0].FullName)
	}
	if res.View.GitHub[0].StarsGained != 5 {
		t.Fatalf("agent-kit 24h 增量应为 5（跨日快照差分），得到 %d", res.View.GitHub[0].StarsGained)
	}
	// agent-kit: 10*log2(6)=25.85 + trending#1(6) + novelty(5) ≈ 36.85；tiny-db: 无增长 0 + rank2(3) = 3
	if len(res.View.Fusion) != 1 {
		t.Fatalf("融合观察应 1 对，得到 %d", len(res.View.Fusion))
	}

	// 8. 资讯榜：融合加成 ×1.25 生效——agent-kit 事件热度应高于无融合的数据库事件
	var agentHot, dbHot float64
	for _, s := range res.View.News {
		switch {
		case strings.Contains(s.TitleZh, "Agent框架"):
			agentHot = s.Hotness
		case strings.Contains(s.TitleZh, "数据库"):
			dbHot = s.Hotness
		}
	}
	if agentHot == 0 || dbHot == 0 {
		t.Fatalf("两个事件都应上榜: agent=%.1f db=%.1f", agentHot, dbHot)
	}
	// agent 事件: 2个T1源 (1.0*0.917 + 1.0*0.905)*10*1.25(融合) ≈ 22.8
	// db 事件: 1个T1源 0.905*10 = 9.05
	if agentHot <= dbHot {
		t.Fatalf("融合事件热度应更高: agent=%.1f db=%.1f", agentHot, dbHot)
	}
}

// TestPipelineRejectsWhenLLMMissing 验证 LLM 必选语义：无网关时精选阶段明确报错，
// 且采集与发现的结果已入库（不浪费）。
func TestPipelineRejectsWhenLLMMissing(t *testing.T) {
	now := time.Now().UTC()
	d := Deps{
		Sources:  newMemSources(),
		Items:    newMemItems(),
		Projects: newMemProjects(),
		Stories:  newMemStories(),
		Digests:  newMemDigests(),
		LLM:      nil,
		GitHub:   fakeGitHub{},
		Fetchers: fakeRegistry{f: fakeFetcher{now: now}},
		Clock:    fixedClock{t: now},
	}
	_ = d.Sources.Save(context.Background(), source.Source{
		ID: "rss-test", Name: "测试源", Kind: source.KindRSS, Tier: source.TierFirstParty,
		Config: map[string]string{"url": "https://example.com/feed.xml"}, Enabled: true, CreatedAt: now,
	})
	_, err := RunPipeline(context.Background(), d)
	if err == nil || !strings.Contains(err.Error(), "LLM 必选") {
		t.Fatalf("应报 LLM 必选错误，得到: %v", err)
	}
	stages, _ := d.Items.ByStage(context.Background(), []item.Stage{item.StageNew}, 10)
	if len(stages) == 0 {
		t.Fatal("采集结果应已入库供下次续跑")
	}
}

// TestCollectDedupe 验证 URL 归一化判重：同 URL 不同追踪参数只入库一次。
func TestCollectDedupe(t *testing.T) {
	now := time.Now().UTC()
	items := newMemItems()
	d := Deps{Items: items, Sources: newMemSources(), Clock: fixedClock{t: now}}

	src := source.Source{ID: "s1", Name: "源", Kind: source.KindRSS, Tier: source.TierFirstParty,
		Config: map[string]string{"url": "https://x.com/feed"}, Enabled: true, CreatedAt: now}
	_ = d.Sources.Save(context.Background(), src)
	d.Fetchers = fakeRegistry{f: dupeVariantFetcher{}}

	inserted, dupes, err := collectOne(context.Background(), d, src, now)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || dupes != 1 {
		t.Fatalf("两次同文异参的产出应入库 1 条、判重 1 条，得到 inserted=%d dupes=%d", inserted, dupes)
	}
}
