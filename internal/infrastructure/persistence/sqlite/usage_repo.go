package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/NoraStory/GithubHot/internal/application"
)

// UsageRepo LLM 用量仓储实现。
type UsageRepo struct{ db *DB }

// NewUsageRepo 构造。
func NewUsageRepo(db *DB) *UsageRepo { return &UsageRepo{db: db} }

// Add 记录一次 LLM 调用用量。
func (r *UsageRepo) Add(ctx context.Context, u application.UsageRow) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO llm_usage (phase, kind, model, prompt_tokens, completion_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		u.Phase, u.Kind, u.Model, u.PromptTokens, u.CompletionTokens, rfc(u.CreatedAt),
	)
	return err
}

// TotalsSince 统计某时刻以来的总用量。
func (r *UsageRepo) TotalsSince(ctx context.Context, since string) (prompt, completion int, err error) {
	err = r.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0) FROM llm_usage WHERE created_at >= ?",
		since).Scan(&prompt, &completion)
	return prompt, completion, err
}

// PhaseSince 按阶段统计某时刻以来的用量。
func (r *UsageRepo) PhaseSince(ctx context.Context, since string) ([]application.PhaseUsage, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT phase, COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COUNT(*) FROM llm_usage WHERE created_at >= ? GROUP BY phase ORDER BY 2 DESC",
		since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.PhaseUsage
	for rows.Next() {
		var p application.PhaseUsage
		if err := rows.Scan(&p.Phase, &p.PromptTokens, &p.CompletionTokens, &p.Calls); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Daily 最近 N 天逐日用量（UTC 日界）。
func (r *UsageRepo) Daily(ctx context.Context, days int) ([]application.DayUsage, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT substr(created_at, 1, 10) AS day, COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0) FROM llm_usage GROUP BY day ORDER BY day DESC LIMIT ?", days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.DayUsage
	for rows.Next() {
		var d application.DayUsage
		if err := rows.Scan(&d.Day, &d.PromptTokens, &d.CompletionTokens); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

var _ application.UsageRepo = (*UsageRepo)(nil)

var _ = sql.ErrNoRows
var _ = fmt.Sprintf
