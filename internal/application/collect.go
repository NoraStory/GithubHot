package application

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// CollectStats 采集统计。
type CollectStats struct {
	DueSources int              `json:"dueSources"`
	Inserted   int              `json:"inserted"`
	Duplicates int              `json:"duplicates"`
	Errors     map[string]error `json:"-"`
	ErrorCount int              `json:"errors"`
}

// CollectSources 采集用例：找出到期的信源，并发抓取（worker pool 控制并发度，
// 保护目标站点与限流配额），经领域判重后入库。
func CollectSources(ctx context.Context, d Deps, workers int) (CollectStats, error) {
	if workers <= 0 {
		workers = 6
	}
	stats := CollectStats{Errors: map[string]error{}}
	all, err := d.Sources.All(ctx)
	if err != nil {
		return stats, fmt.Errorf("读取信源: %w", err)
	}
	now := d.Clock.Now()
	var due []source.Source
	for _, s := range all {
		// github_search / github_trending 由 DiscoverProjects 双轨发现处理，
		// script 由 push 接口直接写入——都不走通用抓取器
		if s.Kind == source.KindGitHubSearch || s.Kind == source.KindGitHubTrending || s.Kind == source.KindScript {
			continue
		}
		if s.DueForFetch(now) {
			due = append(due, s)
		}
	}
	stats.DueSources = len(due)
	if len(due) == 0 {
		return stats, nil
	}

	var mu sync.Mutex
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, s := range due {
		wg.Add(1)
		go func(s source.Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			inserted, dupes, ferr := collectOne(ctx, d, s, now)
			// 自适应抓取间隔（AIHOT 同款）：按产出推进，失败按 0 产出退避
			if ar, ok := d.Sources.(source.AdaptiveRepository); ok {
				insertedForAdaptive := inserted
				if ferr != nil {
					insertedForAdaptive = 0
				}
				if aerr := ar.UpdateFetchStats(ctx, s.ID, insertedForAdaptive, s.IntervalMinutes, now); aerr != nil {
					log.Printf("[collect] 自适应更新失败 %s: %v", s.ID, aerr)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			stats.Inserted += inserted
			stats.Duplicates += dupes
			if ferr != nil {
				stats.Errors[s.ID] = ferr
				stats.ErrorCount++
				log.Printf("[collect] 信源 %s 失败: %v", s.ID, ferr)
			}
		}(s)
	}
	wg.Wait()
	return stats, nil
}

// collectOne 抓取单个信源并入库。
func collectOne(ctx context.Context, d Deps, s source.Source, now time.Time) (inserted, dupes int, err error) {
	fetcher, ferr := d.Fetchers.Fetcher(s.Kind)
	if ferr != nil {
		if ferr == ErrAdapterNotInstalled {
			return 0, 0, fmt.Errorf("%w: %s", ErrAdapterNotInstalled, s.Kind)
		}
		return 0, 0, ferr
	}
	raws, ferr := fetcher.Fetch(ctx, s, now)
	if ferr != nil {
		// 抓取失败同样视为"空手一轮"：触发自适应退避（间隔翻倍），
		// 否则接口宕机时信源会按原间隔空转撞墙。
		if merr := d.Sources.MarkFetched(ctx, s.ID, now); merr != nil {
			log.Printf("[collect] 记录抓取时间失败 %s: %v", s.ID, merr)
		}
		return 0, 0, ferr
	}
	for _, r := range raws {
		key, kerr := shared.DedupeKey(r.URL)
		if kerr != nil {
			continue
		}
		it, ierr := item.New(key, s.ID, string(s.Tier), r.URL, r.Title, r.PublishedAt, now)
		if ierr != nil {
			continue
		}
		it.Summary = r.Summary
		it.Content = r.Content
		it.Author = r.Author
		it.Meta = r.Meta
		// 热榜条目（带 rank 元数据）走轻管道：标 hotboard 阶段，不进入 LLM 双评分。
		if r.Meta != nil && r.Meta["rank"] != "" {
			it.Selection.Stage = item.StageHotBoard
		}
		ok, uerr := d.Items.Upsert(ctx, *it)
		if uerr != nil {
			continue
		}
		if ok {
			inserted++
		} else {
			dupes++
		}
	}
	if merr := d.Sources.MarkFetched(ctx, s.ID, now); merr != nil {
		log.Printf("[collect] 记录抓取时间失败 %s: %v", s.ID, merr)
	}
	return inserted, dupes, nil
}
