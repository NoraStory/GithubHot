package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
)

// ItemRepo 原始资料仓储实现。
type ItemRepo struct{ db *DB }

// NewItemRepo 构造。
func NewItemRepo(db *DB) *ItemRepo { return &ItemRepo{db: db} }

// Upsert 按 ID 判重插入；已存在返回 inserted=false（不覆盖首见内容）。
func (r *ItemRepo) Upsert(ctx context.Context, it item.Item) (bool, error) {
	tags, _ := json.Marshal(it.Selection.Tags)
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO items (id, source_id, source_tier, url, title, summary, content, author, published_at, fetched_at, state, accepted, reason, score_a, score_b, title_zh, summary_zh, reason_zh, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING",
		it.ID, it.SourceID, it.SourceTier, it.URL, it.Title, it.Summary, it.Content, it.Author,
		rfc(it.PublishedAt), rfc(it.FetchedAt),
		string(it.Selection.Stage), boolInt(it.Selection.Pass), it.Selection.Reason,
		it.Selection.ScoreA, it.Selection.ScoreB,
		it.Selection.TitleZh, it.Selection.SummaryZh, it.Selection.ReasonZh, string(tags),
	)
	if err != nil {
		return false, fmt.Errorf("写入条目 %s: %w", it.ID, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountSince 统计时间之后的条数。
func (r *ItemRepo) CountSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE fetched_at >= ?", rfc(since)).Scan(&n)
	return n, err
}

// ByStage 按阶段取条目（发布时间倒序）。占位符数量固定，阶段列表转为 IN 参数。
func (r *ItemRepo) ByStage(ctx context.Context, stages []item.Stage, limit int) ([]item.Item, error) {
	if len(stages) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(stages)), ",")
	query := "SELECT " + itemCols + " FROM items WHERE state IN (" + ph + ") ORDER BY published_at DESC LIMIT ?"
	args := make([]any, 0, len(stages)+1)
	for _, s := range stages {
		args = append(args, string(s))
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("按阶段查询: %w", err)
	}
	defer rows.Close()
	return scanItems(rows)
}

// Recent 最近条目。
func (r *ItemRepo) Recent(ctx context.Context, limit int) ([]item.Item, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+itemCols+" FROM items ORDER BY fetched_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

// UpdateSelection 写回精选结果（覆盖阶段与写作产物）。
func (r *ItemRepo) UpdateSelection(ctx context.Context, id string, sel item.Selection) error {
	tags, _ := json.Marshal(sel.Tags)
	_, err := r.db.ExecContext(ctx,
		"UPDATE items SET state = ?, accepted = ?, reason = ?, score_a = ?, score_b = ?, title_zh = ?, summary_zh = ?, reason_zh = ?, tags = ? WHERE id = ?",
		string(sel.Stage), boolInt(sel.Pass), sel.Reason, sel.ScoreA, sel.ScoreB,
		sel.TitleZh, sel.SummaryZh, sel.ReasonZh, string(tags), id,
	)
	return err
}

// FindByIDs 批量取条目。
func (r *ItemRepo) FindByIDs(ctx context.Context, ids []string) ([]item.Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	query := "SELECT " + itemCols + " FROM items WHERE id IN (" + ph + ")"
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

// Search 在已写作条目中按关键词检索（中文标题/摘要/原标题 LIKE 匹配）。
// % _ 通配符转义：用户输入的 % 不应匹配一切。
func (r *ItemRepo) Search(ctx context.Context, q string, limit int) ([]item.Item, error) {
	pattern := "%" + escapeLike(q) + "%"
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+itemCols+" FROM items WHERE state = 'written' AND (title_zh LIKE ? ESCAPE '\\' OR summary_zh LIKE ? ESCAPE '\\' OR title LIKE ? ESCAPE '\\') ORDER BY published_at DESC LIMIT ?",
		pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

// escapeLike 转义 LIKE 模式中的通配符与转义符本身。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// PendingContentZh 已精选（written 或已聚簇 clustered）、有原文（正文优先，否则摘要）
// 但还没有中文译文的条目。聚簇会把 written 推进成 clustered，两者都算已精选成品。
func (r *ItemRepo) PendingContentZh(ctx context.Context, limit int) ([]item.Item, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+itemCols+" FROM items WHERE state IN ('written', 'clustered') AND content_zh = '' AND (content != '' OR summary != '') ORDER BY published_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

// SaveContentZh 写入原文的中文译文（本地存档）。
func (r *ItemRepo) SaveContentZh(ctx context.Context, id string, zh string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE items SET content_zh = ? WHERE id = ?", zh, id)
	return err
}

const itemCols = "id, source_id, source_tier, url, title, summary, content, content_zh, author, published_at, fetched_at, state, accepted, reason, score_a, score_b, title_zh, summary_zh, reason_zh, tags"

func scanItems(rows *sql.Rows) ([]item.Item, error) {
	var out []item.Item
	for rows.Next() {
		var it item.Item
		var kindred struct{ stage, tags string }
		var published, fetched string
		var accepted int
		if err := rows.Scan(&it.ID, &it.SourceID, &it.SourceTier, &it.URL, &it.Title, &it.Summary, &it.Content, &it.ContentZh, &it.Author,
			&published, &fetched, &kindred.stage, &accepted, &it.Selection.Reason,
			&it.Selection.ScoreA, &it.Selection.ScoreB,
			&it.Selection.TitleZh, &it.Selection.SummaryZh, &it.Selection.ReasonZh, &kindred.tags); err != nil {
			return nil, err
		}
		it.Selection.Stage = item.Stage(kindred.stage)
		it.Selection.Pass = accepted == 1
		_ = json.Unmarshal([]byte(kindred.tags), &it.Selection.Tags)
		it.PublishedAt = parseTime(published)
		it.FetchedAt = parseTime(fetched)
		out = append(out, it)
	}
	return out, rows.Err()
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// 接口满足性检查。
var _ item.Repository = (*ItemRepo)(nil)
