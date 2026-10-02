package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/shared"
)

// UsageRow 一次 LLM 调用的用量记录。
type UsageRow struct {
	Phase            string // prefilter / score / write / cluster / fusion / overview / embed
	Kind             string // chat / embed
	Model            string
	PromptTokens     int
	CompletionTokens int
	CreatedAt        time.Time
}

// PhaseUsage 按阶段汇总。
type PhaseUsage struct {
	Phase            string `json:"phase"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	Calls            int    `json:"calls"`
}

// DayUsage 逐日汇总。
type DayUsage struct {
	Day              string `json:"day"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
}

// UsageSummary 用量总览（Token 计量 + 成本估算）。
type UsageSummary struct {
	BudgetTokens   int          `json:"budgetTokensPerDay"`
	UsedPrompt     int          `json:"todayPromptTokens"`
	UsedCompletion int          `json:"todayCompletionTokens"`
	UsedTotal      int          `json:"todayTotalTokens"`
	Exceeded       bool         `json:"budgetExceeded"`
	PriceInPerM    float64      `json:"priceInPerM"`
	PriceOutPerM   float64      `json:"priceOutPerM"`
	EstimatedCost  float64      `json:"estimatedCostToday"`
	ByPhase        []PhaseUsage `json:"byPhase"`
	Days           []DayUsage   `json:"days"`
}

// UsageRepo 用量仓储端口。
type UsageRepo interface {
	Add(ctx context.Context, u UsageRow) error
	TotalsSince(ctx context.Context, since string) (prompt, completion int, err error)
	PhaseSince(ctx context.Context, since string) ([]PhaseUsage, error)
	Daily(ctx context.Context, days int) ([]DayUsage, error)
}

// BudgetConfig 预算配置：每日 Token 上限 + 单价（每百万 token，用于成本估算展示）。
type BudgetConfig struct {
	DailyTokens  int
	PriceInPerM  float64
	PriceOutPerM float64
}

// ErrBudgetExceeded 预算熔断：当日 Token 配额已用尽。
var ErrBudgetExceeded = errors.New("预算熔断：当日 LLM Token 配额已用尽（LLM_BUDGET_TOKENS_PER_DAY）")

// PhaseSetter 流水线在阶段间设置当前阶段名（用量按阶段归账）。
type PhaseSetter interface{ SetPhase(phase string) }

// BudgetChecker 查询预算是否已熔断。
type BudgetChecker interface{ BudgetExceeded() bool }

// BudgetGuard LLM 网关的预算装饰器：每次调用前检查当日配额，
// 调用后（经网关的用量回调）按阶段记账。实现 LLMGateway，可透明替换。
type BudgetGuard struct {
	Next   LLMGateway
	Usage  UsageRepo
	Budget BudgetConfig
	Clock  shared.Clock

	phase string
}

// NewBudgetGuard 构造并把自己登记为网关的用量接收器。
func NewBudgetGuard(next LLMGateway, usage UsageRepo, budget BudgetConfig, clock shared.Clock) *BudgetGuard {
	g := &BudgetGuard{Next: next, Usage: usage, Budget: budget, Clock: clock}
	return g
}

// SetPhase 设置当前流水线阶段。
func (g *BudgetGuard) SetPhase(phase string) { g.phase = phase }

// BudgetExceeded 报告是否已熔断。
func (g *BudgetGuard) BudgetExceeded() bool {
	p, _ := g.todayTokens()
	return g.Budget.DailyTokens > 0 && p >= g.Budget.DailyTokens
}

// RecordUsage 网关用量回调（在 llm 内部解析出 usage 后调用）。
func (g *BudgetGuard) RecordUsage(kind, model string, prompt, completion int) {
	if g.Usage == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = g.Usage.Add(ctx, UsageRow{
		Phase: g.phase, Kind: kind, Model: model,
		PromptTokens: prompt, CompletionTokens: completion, CreatedAt: g.Clock.Now(),
	})
}

func (g *BudgetGuard) todayTokens() (int, error) {
	if g.Usage == nil {
		return 0, nil
	}
	since := g.Clock.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:00")
	p, c, err := g.Usage.TotalsSince(context.Background(), since)
	return p + c, err
}

// checkBudget 调用前熔断检查。
func (g *BudgetGuard) checkBudget() error {
	used, err := g.todayTokens()
	if err != nil {
		return nil // 统计失败不阻断业务
	}
	if g.Budget.DailyTokens > 0 && used >= g.Budget.DailyTokens {
		return fmt.Errorf("%w: 已用 %d tokens", ErrBudgetExceeded, used)
	}
	return nil
}

// ChatJSON 预算检查 → 委托下层网关（用量经回调记账）。
func (g *BudgetGuard) ChatJSON(ctx context.Context, system, user, model string, temperature float64) (string, error) {
	if err := g.checkBudget(); err != nil {
		return "", err
	}
	return g.Next.ChatJSON(ctx, system, user, model, temperature)
}

// Embed 预算检查 → 委托下层网关。
func (g *BudgetGuard) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := g.checkBudget(); err != nil {
		return nil, err
	}
	return g.Next.Embed(ctx, texts)
}

// ModelA / ModelB 透传。
func (g *BudgetGuard) ModelA() string { return g.Next.ModelA() }
func (g *BudgetGuard) ModelB() string { return g.Next.ModelB() }

// UsageSummary 汇总今日 + 近 7 日用量与成本估算（控制台展示）。
func (g *BudgetGuard) UsageSummary(ctx context.Context) (UsageSummary, error) {
	return UsageOverview(ctx, g.Usage, g.Budget, g.Clock)
}

// UsageOverview 从仓储直接汇总用量（供只读的接口层使用，不依赖 guard 实例）。
func UsageOverview(ctx context.Context, usage UsageRepo, budget BudgetConfig, clock shared.Clock) (UsageSummary, error) {
	var s UsageSummary
	s.BudgetTokens = budget.DailyTokens
	s.PriceInPerM = budget.PriceInPerM
	s.PriceOutPerM = budget.PriceOutPerM
	if usage == nil {
		return s, nil
	}
	since := clock.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:00")
	p, c, err := usage.TotalsSince(ctx, since)
	if err != nil {
		return s, err
	}
	s.UsedPrompt, s.UsedCompletion = p, c
	s.UsedTotal = p + c
	s.Exceeded = budget.DailyTokens > 0 && s.UsedTotal >= budget.DailyTokens
	s.EstimatedCost = float64(p)/1e6*budget.PriceInPerM + float64(c)/1e6*budget.PriceOutPerM
	s.ByPhase, _ = usage.PhaseSince(ctx, since)
	s.Days, _ = usage.Daily(ctx, 7)
	return s, nil
}

// 编译期接口满足性检查。
var _ LLMGateway = (*BudgetGuard)(nil)
var _ PhaseSetter = (*BudgetGuard)(nil)
var _ BudgetChecker = (*BudgetGuard)(nil)
