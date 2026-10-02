package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/github"
)

// ProjectRepo GitHub 项目仓储实现。
type ProjectRepo struct{ db *DB }

// NewProjectRepo 构造。
func NewProjectRepo(db *DB) *ProjectRepo { return &ProjectRepo{db: db} }

// Upsert 新增或全量更新项目。
func (r *ProjectRepo) Upsert(ctx context.Context, p github.Project) error {
	topics, _ := json.Marshal(p.Topics)
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO projects (full_name, html_url, description, language, topics, stars, forks, trending_rank, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(full_name) DO UPDATE SET html_url = excluded.html_url, description = excluded.description, language = excluded.language, topics = excluded.topics, stars = excluded.stars, forks = excluded.forks, trending_rank = excluded.trending_rank, last_seen_at = excluded.last_seen_at",
		p.FullName, p.HTMLURL, p.Description, p.Language, string(topics), p.Stars, p.Forks, p.TrendingRank, rfc(p.FirstSeenAt), rfc(p.LastSeenAt),
	)
	return err
}

// Touch 更新派生字段（保留首次发现时间）。
func (r *ProjectRepo) Touch(ctx context.Context, p github.Project) error {
	topics, _ := json.Marshal(p.Topics)
	_, err := r.db.ExecContext(ctx,
		"UPDATE projects SET html_url = ?, description = ?, language = ?, topics = ?, stars = ?, forks = ?, trending_rank = ?, last_seen_at = ? WHERE full_name = ?",
		p.HTMLURL, p.Description, p.Language, string(topics), p.Stars, p.Forks, p.TrendingRank, rfc(p.LastSeenAt), p.FullName,
	)
	return err
}

// FindByFullName 按全名查项目。
func (r *ProjectRepo) FindByFullName(ctx context.Context, fullName string) (*github.Project, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT full_name, html_url, description, language, topics, stars, forks, trending_rank, first_seen_at, last_seen_at FROM projects WHERE full_name = ?", fullName)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// All 全部项目。
func (r *ProjectRepo) All(ctx context.Context) ([]github.Project, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT full_name, html_url, description, language, topics, stars, forks, trending_rank, first_seen_at, last_seen_at FROM projects ORDER BY full_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []github.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// AddSnapshot 追加项目观测快照。
func (r *ProjectRepo) AddSnapshot(ctx context.Context, s github.Snapshot) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO project_snapshots (full_name, at, stars, trending_rank, search_rank) VALUES (?, ?, ?, ?, ?)",
		s.FullName, rfc(s.At), s.Stars, s.TrendingRank, s.SearchRank,
	)
	return err
}

// SnapshotsSince 某项目自某时刻起的快照（升序）。
func (r *ProjectRepo) SnapshotsSince(ctx context.Context, fullName string, since time.Time) ([]github.Snapshot, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT full_name, at, stars, trending_rank, search_rank FROM project_snapshots WHERE full_name = ? AND at >= ? ORDER BY at ASC",
		fullName, rfc(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshots(rows, fullName)
}

// AllSnapshotsSince 全部项目自某时刻起的快照，按项目分组。
func (r *ProjectRepo) AllSnapshotsSince(ctx context.Context, since time.Time) (map[string][]github.Snapshot, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT full_name, at, stars, trending_rank, search_rank FROM project_snapshots WHERE at >= ? ORDER BY at ASC", rfc(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]github.Snapshot{}
	for rows.Next() {
		var name string
		var s github.Snapshot
		var at string
		if err := rows.Scan(&name, &at, &s.Stars, &s.TrendingRank, &s.SearchRank); err != nil {
			return nil, err
		}
		s.FullName = name
		s.At = parseTime(at)
		out[name] = append(out[name], s)
	}
	return out, rows.Err()
}

const projectCols = "full_name, html_url, description, language, topics, stars, forks, trending_rank, first_seen_at, last_seen_at"

func scanProject(rs rowScanner) (*github.Project, error) {
	var p github.Project
	var topics string
	var first, last string
	if err := rs.Scan(&p.FullName, &p.HTMLURL, &p.Description, &p.Language, &topics, &p.Stars, &p.Forks, &p.TrendingRank, &first, &last); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(topics), &p.Topics)
	p.FirstSeenAt = parseTime(first)
	p.LastSeenAt = parseTime(last)
	return &p, nil
}

func scanSnapshots(rows *sql.Rows, fallbackName string) ([]github.Snapshot, error) {
	var out []github.Snapshot
	for rows.Next() {
		var s github.Snapshot
		var at string
		if err := rows.Scan(&s.FullName, &at, &s.Stars, &s.TrendingRank, &s.SearchRank); err != nil {
			return nil, err
		}
		s.At = parseTime(at)
		out = append(out, s)
	}
	_ = fallbackName
	return out, rows.Err()
}

// 接口满足性检查。
var _ github.Repository = (*ProjectRepo)(nil)
