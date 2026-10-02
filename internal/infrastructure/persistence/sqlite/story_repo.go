package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// StoryRepo 事件仓储实现。成员与项目链接以 JSON 列存储（聚合内整体存取）。
type StoryRepo struct{ db *DB }

// NewStoryRepo 构造。
func NewStoryRepo(db *DB) *StoryRepo { return &StoryRepo{db: db} }

// Save 全量保存事件（UPSERT）。
func (r *StoryRepo) Save(ctx context.Context, s *story.Story) error {
	members, err := json.Marshal(s.Members)
	if err != nil {
		return fmt.Errorf("序列化成员: %w", err)
	}
	projects, _ := json.Marshal(s.Projects)
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO stories (id, kind, title_zh, summary_zh, url, overview, manual, members, projects, hotness, first_seen_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET kind = excluded.kind, title_zh = excluded.title_zh, summary_zh = excluded.summary_zh, url = excluded.url, overview = excluded.overview, manual = excluded.manual, members = excluded.members, projects = excluded.projects, hotness = excluded.hotness, updated_at = excluded.updated_at",
		s.ID, string(s.Kind), s.TitleZh, s.SummaryZh, s.URL, s.Overview, boolInt(s.Manual), string(members), string(projects), s.Hotness, rfc(s.FirstSeenAt), rfc(s.UpdatedAt),
	)
	return err
}

// SetManual 设置/取消人工锁定。
func (r *StoryRepo) SetManual(ctx context.Context, storyID string, manual bool) error {
	_, err := r.db.ExecContext(ctx, "UPDATE stories SET manual = ? WHERE id = ?", boolInt(manual), storyID)
	return err
}

// HotnessHistory 事件热度历史（升序）。
func (r *StoryRepo) HotnessHistory(ctx context.Context, storyID string, limit int) ([]story.HotnessPoint, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT at, hotness FROM story_history WHERE story_id = ? ORDER BY at DESC LIMIT ?", storyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []story.HotnessPoint
	for rows.Next() {
		var at string
		p := story.HotnessPoint{}
		if err := rows.Scan(&at, &p.Hotness); err != nil {
			return nil, err
		}
		p.At = parseTime(at)
		out = append(out, p)
	}
	// 反转为升序
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// SaveOverview 写入事件综述。
func (r *StoryRepo) SaveOverview(ctx context.Context, storyID string, overview string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE stories SET overview = ? WHERE id = ?", overview, storyID)
	return err
}

// FindByID 按ID查事件。
func (r *StoryRepo) FindByID(ctx context.Context, id string) (*story.Story, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, kind, title_zh, summary_zh, url, COALESCE(overview, ''), COALESCE(manual, 0), members, projects, hotness, first_seen_at, updated_at FROM stories WHERE id = ?", id)
	return scanStory(row)
}

// Active 指定时间之后仍有活跃成员的事件（粗过滤，精确窗口在领域服务里算）。
func (r *StoryRepo) Active(ctx context.Context, since time.Time) ([]*story.Story, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, kind, title_zh, summary_zh, url, COALESCE(overview, ''), COALESCE(manual, 0), members, projects, hotness, first_seen_at, updated_at FROM stories WHERE updated_at >= ? OR first_seen_at >= ? ORDER BY hotness DESC",
		rfc(since), rfc(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*story.Story
	for rows.Next() {
		s, err := scanStory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Delete 删除事件（被吸收方）。
func (r *StoryRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM stories WHERE id = ?", id)
	return err
}

// AddHistory 记录热度历史。
func (r *StoryRepo) AddHistory(ctx context.Context, storyID string, at time.Time, hotness float64) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO story_history (story_id, at, hotness) VALUES (?, ?, ?)",
		storyID, rfc(at), hotness,
	)
	return err
}

// HistoryNear 取事件在 [at-lookBack, at] 区间内最接近 at 的热度。
func (r *StoryRepo) HistoryNear(ctx context.Context, storyID string, at time.Time, lookBack time.Duration) (float64, bool, error) {
	var h float64
	from := rfc(at.Add(-lookBack))
	to := rfc(at)
	err := r.db.QueryRowContext(ctx,
		"SELECT hotness FROM story_history WHERE story_id = ? AND at >= ? AND at <= ? ORDER BY at DESC LIMIT 1",
		storyID, from, to).Scan(&h)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return h, true, nil
}

// LinkProjects 建立事件与项目的融合链接（并集追加）。
func (r *StoryRepo) LinkProjects(ctx context.Context, storyID string, fullNames []string) error {
	s, err := r.FindByID(ctx, storyID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range s.Projects {
		seen[p] = true
	}
	for _, p := range fullNames {
		if !seen[p] {
			s.Projects = append(s.Projects, p)
			seen[p] = true
		}
	}
	return r.Save(ctx, s)
}

func scanStory(rs rowScanner) (*story.Story, error) {
	var s story.Story
	var kind, members, projects string
	var manual int
	var first, updated string
	if err := rs.Scan(&s.ID, &kind, &s.TitleZh, &s.SummaryZh, &s.URL, &s.Overview, &manual, &members, &projects, &s.Hotness, &first, &updated); err != nil {
		return nil, err
	}
	s.Manual = manual == 1
	s.Kind = story.Kind(kind)
	_ = json.Unmarshal([]byte(members), &s.Members)
	_ = json.Unmarshal([]byte(projects), &s.Projects)
	s.FirstSeenAt = parseTime(first)
	s.UpdatedAt = parseTime(updated)
	return &s, nil
}

// 接口满足性检查。
var _ story.Repository = (*StoryRepo)(nil)
