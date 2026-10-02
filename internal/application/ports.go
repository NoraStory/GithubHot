// Package application 编排领域对象完成用例。它只依赖领域层与下方声明的
// 端口（接口），对具体适配器一无所知——依赖全部指向内层（DDD 依赖倒置）。
package application

import (
	"context"
	"errors"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/domain/github"
	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// FetchedItem 信源适配器产出的一条原始资料。
type FetchedItem struct {
	URL         string
	Title       string
	Summary     string
	Content     string
	Author      string
	PublishedAt time.Time
}

// SourceFetcher 信源抓取端口。一种 Kind 对应一个实现。
type SourceFetcher interface {
	Kind() source.Kind
	Fetch(ctx context.Context, s source.Source, now time.Time) ([]FetchedItem, error)
}

// FetcherRegistry 按 Kind 分发抓取器。
type FetcherRegistry interface {
	Fetcher(kind source.Kind) (SourceFetcher, error)
}

// ErrAdapterNotInstalled 预留信源种类（X 账号、微信公众号）没有适配器。
var ErrAdapterNotInstalled = errors.New("该信源种类的适配器未随仓库分发（需要付费 API），请自行实现 SourceFetcher 并注册")

// GitHubRepo GitHub 双轨发现产出的项目快照。
type GitHubRepo struct {
	FullName     string
	HTMLURL      string
	Description  string
	Language     string
	Topics       []string
	Stars        int
	Forks        int
	TrendingRank int
	SearchRank   int
}

// GitHubGateway GitHub 数据端口：官方 Search API 与 trending 页双轨。
type GitHubGateway interface {
	// SearchNewRising 搜索近 N 天创建、star 超过阈值、按 star 排序的新项目。
	SearchNewRising(ctx context.Context, sinceDays, minStars, perPage int) ([]GitHubRepo, error)
	// FetchTrending 抓取 trending 每日榜。失败返回错误，由调用方决定降级。
	FetchTrending(ctx context.Context) ([]GitHubRepo, error)
}

// LLMGateway 大模型端口（OpenAI 兼容）。LLM 必选：没有可用网关时精选流程拒绝运行。
type LLMGateway interface {
	// ChatJSON 发起一次要求 JSON 输出的对话，返回原始 JSON 文本。
	ChatJSON(ctx context.Context, system, user string, model string, temperature float64) (string, error)
	// ModelA / ModelB 双评分用的两个模型标识。
	ModelA() string
	ModelB() string
	// Embed 语义向量。返回 ErrEmbeddingsUnsupported 表示未配置向量模型。
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// ErrEmbeddingsUnsupported 未配置向量模型，聚簇候选退化为词面相似度。
var ErrEmbeddingsUnsupported = errors.New("未配置向量模型（LLM_EMBED_MODEL）")

// SiteFiles 生成的静态站点产物（写盘由接口层决定）。
type SiteFiles struct {
	IndexHTML string
	DigestMD  string
}

// DigestRenderer 日报渲染端口：把榜单视图模型渲染成 Markdown。
type DigestRenderer interface {
	Render(ctx context.Context, v DigestView) (string, error)
}

// SiteRenderer 双榜网页渲染端口。
type SiteRenderer interface {
	RenderIndex(ctx context.Context, v HotView) (string, error)
	// RenderConsole 控制台页（用量/诊断/运行/信源）。
	RenderConsole(ctx context.Context, v ConsoleView) (string, error)
	// RenderSearch 搜索结果页。
	RenderSearch(ctx context.Context, v SearchView) (string, error)
}

// RunRow 一次流水线运行记录（展示行）。
type RunRow struct {
	StartedAt string  `json:"startedAt"`
	Status    string  `json:"status"`
	Duration  float64 `json:"durationSeconds"`
	Collected int     `json:"collected"`
	Written   int     `json:"written"`
	Stories   int     `json:"stories"`
}

// DiagRow 内容诊断行：一条资料走完精选流水线的全程痕迹。
type DiagRow struct {
	ID         string  `json:"id"`
	Stage      string  `json:"stage"`
	SourceName string  `json:"sourceName"`
	Title      string  `json:"title"`
	TitleZh    string  `json:"titleZh"`
	URL        string  `json:"url"`
	Reason     string  `json:"reason"`
	ScoreA     float64 `json:"scoreA"`
	ScoreB     float64 `json:"scoreB"`
	Published  string  `json:"publishedAt"`
}

// SourceInfo 信源信息（控制台展示行）。
type SourceInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Tier    string   `json:"tier"`
	Tags    []string `json:"tags"`
	Enabled bool     `json:"enabled"`
	Adapter string   `json:"adapter"`
}

// ConsoleView 控制台视图。
type ConsoleView struct {
	Usage        UsageSummary
	Runs         []RunRow
	Diagnostics  []DiagRow
	Stage        string
	Stages       []string
	Sources      []SourceInfo
	SourcesCount int
	Digests      []DigestMeta
}

// DigestMeta 期号元信息。
type DigestMeta struct {
	Date string `json:"date"`
	Kind string `json:"kind"`
}

// SearchView 搜索结果页视图。
type SearchView struct {
	Query   string
	Results []SearchResult
	Took    string
}

// DigestView 日报渲染的视图模型（接口层数据契约，含未来 APP 复用的 JSON 形状）。
type DigestView struct {
	Kind        string // daily / weekly / monthly
	Date        string // 期号
	PeriodLabel string // 展示用周期描述
	GitHub      []ProjectRow
	News        []StoryRow
	Fusion      []FusionRow
	Stats       DigestStats
	Generated   time.Time
}

// DigestStats 日报统计。
type DigestStats struct {
	Sources   int     `json:"sources"`
	Collected int     `json:"collected"`
	ModelA    string  `json:"modelA"`
	ModelB    string  `json:"modelB"`
	Duration  float64 `json:"durationSeconds"`
}

// ProjectRow GitHub 项目榜单行。
type ProjectRow struct {
	Rank          int      `json:"rank"`
	FullName      string   `json:"fullName"`
	URL           string   `json:"url"`
	Description   string   `json:"description"`
	DescriptionZh string   `json:"descriptionZh"`
	Language      string   `json:"language"`
	Topics        []string `json:"topics"`
	Stars         int      `json:"stars"`
	StarsGained   int      `json:"starsGained"`
	TrendingRank  int      `json:"trendingRank"`
	Hotness       float64  `json:"hotness"`
	Badges        []string `json:"badges"`
	StoryID       string   `json:"storyId"`
}

// StoryRow AI 资讯榜单行。
type StoryRow struct {
	Rank        int      `json:"rank"`
	StoryID     string   `json:"storyId"`
	TitleZh     string   `json:"titleZh"`
	SummaryZh   string   `json:"summaryZh"`
	Overview    string   `json:"overview,omitempty"`
	ReasonZh    string   `json:"reasonZh"`
	URL         string   `json:"url"`
	Tags        []string `json:"tags"`
	SourceCount int      `json:"sourceCount"`
	SourceNames []string `json:"sourceNames"`
	Score       float64  `json:"score"`
	Hotness     float64  `json:"hotness"`
	Badges      []string `json:"badges"`
	Projects    []string `json:"projects"`
}

// FusionRow 融合观察行：一条资讯 × 一个互相印证的项目。
type FusionRow struct {
	News    StoryRow   `json:"news"`
	Project ProjectRow `json:"project"`
}

// HotView 双榜页视图模型（API 与静态站共用）。
type HotView struct {
	Generated time.Time    `json:"generatedAt"`
	GitHub    []ProjectRow `json:"github"`
	News      []StoryRow   `json:"news"`
	Fusion    []FusionRow  `json:"fusion"`
	Digests   []DigestMeta `json:"digests"`
}

// Deps 应用层依赖的最小端口集合（全部在领域层或上方声明，基础设施层实现）。
type Deps struct {
	Sources        source.Repository
	Items          item.Repository
	Projects       github.Repository
	Stories        story.Repository
	Digests        digest.Repository
	Usage          UsageRepo
	Budget         BudgetConfig
	LLM            LLMGateway
	GitHub         GitHubGateway
	Fetchers       FetcherRegistry
	DigestRenderer DigestRenderer
	SiteRenderer   SiteRenderer
	Clock          shared.Clock
}
