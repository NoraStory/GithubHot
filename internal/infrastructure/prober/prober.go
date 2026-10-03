// Package prober 健康探针：定期探测各信源可用性与关键端点连通性。
// 结果落库 probe_results，管理端 /admin/probes 展示。
package prober

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// Result 一次探测结果。
type Result struct {
	Target    string
	Kind      string // source | endpoint
	OK        bool
	Latency   time.Duration
	Detail    string
	CheckedAt time.Time
}

// Store 探测结果存储端口。
type Store interface {
	RecordProbe(target, kind string, ok bool, latency time.Duration, detail string, at time.Time) error
	LatestProbes(ctx context.Context) ([]sqlite.ProbeRecord, error)
	ProbeHistory(ctx context.Context, target string, limit int) ([]sqlite.ProbeRecord, error)
}

// Service 探针服务。
type Service struct {
	Deps    application.Deps
	Store   Store
	Targets EndpointTargets
	// PingDB 本地数据库探针；nil 时跳过该项。
	PingDB func(ctx context.Context) error

	running atomic.Bool // 防重入：定时轮询与手动触发不会重叠
}

// EndpointTargets 端点探测目标。
type EndpointTargets struct {
	LLMBaseURL  string
	LLMAPIKey   string
	GitHubToken string
	// StorageBase 背景视频对象存储公开前缀；留空跳过该项。
	StorageBase string
	// MusicID 音乐馆歌单 id（探测 meting 上游用）；留空用内置默认。
	MusicID string
}

// probeWorkers 并发探测的协程数。
const probeWorkers = 4

// probeTimeout 单个探针的超时。
const probeTimeout = 25 * time.Second

// Run 执行一轮全量探测并落库，返回本轮结果。
// 已有探测进行中时直接返回 nil（手动触发与定时轮询不会重叠）。
func (s *Service) Run(ctx context.Context) []Result {
	if !s.running.CompareAndSwap(false, true) {
		log.Printf("[prober] 上一轮探测尚未结束，本轮跳过")
		return nil
	}
	defer s.running.Store(false)

	type job struct {
		name string
		run  func(ctx context.Context) Result
	}
	jobs := []job{}
	// 信源探测
	sources, err := s.Deps.Sources.All(ctx)
	if err != nil {
		log.Printf("[prober] 读取信源失败: %v", err)
	}
	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		// 预留/内建种类（github_search/github_trending/script）的适配器不随仓库分发，
		// 由 GitHub 双轨与推送通道直管——跳过，避免把"设计如此"报成故障。
		if _, err := s.Deps.Fetchers.Fetcher(src.Kind); err != nil {
			continue
		}
		src := src
		jobs = append(jobs, job{name: "source:" + src.ID, run: func(ctx context.Context) Result {
			return s.probeSource(ctx, src)
		}})
	}
	// 端点探测
	endpoints := []struct {
		name string
		run  func(ctx context.Context) Result
	}{
		{"LLM 网关", s.probeLLM},
		{"GitHub API", s.probeGitHub},
		{"音乐代理上游", s.probeMusic},
		{"背景对象存储", s.probeStorage},
	}
	if s.PingDB != nil {
		endpoints = append(endpoints, struct {
			name string
			run  func(ctx context.Context) Result
		}{"本地数据库", s.probeDB})
	}
	for _, e := range endpoints {
		e := e
		jobs = append(jobs, job{name: "endpoint:" + e.name, run: e.run})
	}

	results := make([]Result, 0, len(jobs))
	var mu sync.Mutex
	sem := make(chan struct{}, probeWorkers)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			res := j.run(pctx)
			if res.CheckedAt.IsZero() {
				res.CheckedAt = time.Now()
			}
			if err := s.Store.RecordProbe(res.Target, res.Kind, res.OK, res.Latency, res.Detail, res.CheckedAt); err != nil {
				log.Printf("[prober] 记录失败 %s: %v", res.Target, err)
			}
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	return results
}

// Start 启动探测调度：先等初始延迟再跑首轮，之后每 interval 一轮，直到 ctx 取消。
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	go func() {
		initial := 15 * time.Second
		log.Printf("[prober] 探针已启动：首轮 %v 后进行，之后每 %v 一轮", initial, interval)
		select {
		case <-ctx.Done():
			return
		case <-time.After(initial):
		}
		for {
			results := s.Run(ctx)
			ok, total := 0, 0
			for _, r := range results {
				total++
				if r.OK {
					ok++
				}
			}
			log.Printf("[prober] 本轮探测完成：%d/%d 正常", ok, total)
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

func result(target, kind string, start time.Time, err error, detail string) Result {
	r := Result{Target: target, Kind: kind, Latency: time.Since(start), Detail: detail}
	if err != nil {
		r.Detail = trunc(err.Error(), 160)
	} else {
		r.OK = true
	}
	return r
}

// probeSource 用信源适配器真实抓取一次（与信源管理"试抓"同一通道，只抓不精选）。
func (s *Service) probeSource(ctx context.Context, src source.Source) Result {
	start := time.Now()
	f, err := s.Deps.Fetchers.Fetcher(src.Kind)
	if err != nil {
		return result(src.Name, "source", start, err, "")
	}
	raws, err := f.Fetch(ctx, src, time.Now())
	if err != nil {
		return result(src.Name, "source", start, err, "")
	}
	return result(src.Name, "source", start, nil, fmt.Sprintf("抓到 %d 条", len(raws)))
}

// probeLLM GET {base}/models 校验网关连通性与 Key 有效性。
func (s *Service) probeLLM(ctx context.Context) Result {
	const target = "LLM 网关"
	start := time.Now()
	if s.Targets.LLMBaseURL == "" {
		return result(target, "endpoint", start, fmt.Errorf("未配置 LLM_BASE_URL"), "")
	}
	body, status, err := safehttp.Do(ctx, "GET", strings.TrimSuffix(s.Targets.LLMBaseURL, "/")+"/models",
		map[string]string{"Authorization": "Bearer " + s.Targets.LLMAPIKey}, nil)
	if err != nil {
		return result(target, "endpoint", start, err, "")
	}
	if status/100 != 2 {
		return result(target, "endpoint", start, fmt.Errorf("HTTP %d", status), "")
	}
	detail := fmt.Sprintf("HTTP %d", status)
	var m struct {
		Data []any `json:"data"`
	}
	if json.Unmarshal(body, &m) == nil {
		detail = fmt.Sprintf("HTTP %d · %d 个模型可用", status, len(m.Data))
	}
	return result(target, "endpoint", start, nil, detail)
}

// probeGitHub GET /rate_limit 校验 token 有效性并报告剩余配额。
func (s *Service) probeGitHub(ctx context.Context) Result {
	const target = "GitHub API"
	start := time.Now()
	headers := map[string]string{"Accept": "application/vnd.github+json"}
	if s.Targets.GitHubToken != "" {
		headers["Authorization"] = "Bearer " + s.Targets.GitHubToken
	}
	body, status, err := safehttp.Do(ctx, "GET", "https://api.github.com/rate_limit", headers, nil)
	if err != nil {
		return result(target, "endpoint", start, err, "")
	}
	if status/100 != 2 {
		return result(target, "endpoint", start, fmt.Errorf("HTTP %d", status), "")
	}
	detail := fmt.Sprintf("HTTP %d", status)
	var m struct {
		Resources struct {
			Core struct {
				Remaining int `json:"remaining"`
				Limit     int `json:"limit"`
			} `json:"core"`
		} `json:"resources"`
	}
	if json.Unmarshal(body, &m) == nil && m.Resources.Core.Limit > 0 {
		detail = fmt.Sprintf("HTTP %d · 配额 %d/%d", status, m.Resources.Core.Remaining, m.Resources.Core.Limit)
	}
	return result(target, "endpoint", start, nil, detail)
}

// probeMusic 探测 meting 上游（音乐馆歌单代理的源头）：用真实歌单请求
// （type=playlist，与服务端代理同一通道），2xx 即健康。
func (s *Service) probeMusic(ctx context.Context) Result {
	const target = "音乐代理上游"
	start := time.Now()
	id := strings.TrimSpace(s.Targets.MusicID)
	if id == "" {
		id = "652135520"
	}
	_, status, err := safehttp.Do(ctx, "GET",
		"https://api.injahow.cn/meting/?server=netease&type=playlist&id="+id, nil, nil)
	if err != nil {
		return result(target, "endpoint", start, err, "")
	}
	if status/100 != 2 {
		return result(target, "endpoint", start, fmt.Errorf("HTTP %d", status), "")
	}
	return result(target, "endpoint", start, nil, fmt.Sprintf("HTTP %d · 歌单可拉取", status))
}

// probeStorage 探测背景视频对象存储（R2 公开域名），Range 请求只取 1 字节。
func (s *Service) probeStorage(ctx context.Context) Result {
	const target = "背景对象存储"
	start := time.Now()
	if s.Targets.StorageBase == "" {
		return result(target, "endpoint", start, fmt.Errorf("未配置对象存储（R2_PUBLIC_BASE 为空）"), "")
	}
	_, status, err := safehttp.Do(ctx, "GET", strings.TrimSuffix(s.Targets.StorageBase, "/")+"/x/x7.mp4",
		map[string]string{"Range": "bytes=0-0"}, nil)
	if err != nil {
		return result(target, "endpoint", start, err, "")
	}
	if status/100 == 5 || status == 404 {
		return result(target, "endpoint", start, fmt.Errorf("HTTP %d", status), "")
	}
	return result(target, "endpoint", start, nil, fmt.Sprintf("HTTP %d", status))
}

func (s *Service) probeDB(ctx context.Context) Result {
	const target = "本地数据库"
	start := time.Now()
	if err := s.PingDB(ctx); err != nil {
		return result(target, "endpoint", start, err, "")
	}
	return result(target, "endpoint", start, nil, "SELECT 1 通过")
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
