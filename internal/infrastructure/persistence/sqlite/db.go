// Package sqlite 是全部领域仓储的持久化实现（modernc.org/sqlite，纯 Go 无 CGO，
// 单二进制跨平台部署）。DDL 为内联字面量，幂等可重复执行；
// 空串默认值统一由仓储层代码保证，不依赖 SQL DEFAULT。
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB 持有连接。
type DB struct {
	*sql.DB
}

// Open 打开（必要时创建）数据库并初始化 schema。
// 单连接模式：SQLite 单写者，本应用写入均为短事务，串行化最稳。
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s: %w", dataDir, err)
	}
	dbPath := filepath.Join(dataDir, "githubhot.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 busy_timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 foreign_keys: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db}, nil
}

// migrate 执行内建 DDL（全部为单行内联字面量，无任何输入参与）。
func migrate(db *sql.DB) error {
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS sources (id TEXT PRIMARY KEY, name TEXT NOT NULL, kind TEXT NOT NULL, config TEXT NOT NULL, tier TEXT NOT NULL, tags TEXT NOT NULL, interval_minutes INTEGER NOT NULL, enabled INTEGER NOT NULL, created_at TEXT NOT NULL, last_fetched_at TEXT)"); err != nil {
		return fmt.Errorf("建表 sources: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS items (id TEXT PRIMARY KEY, source_id TEXT NOT NULL, source_tier TEXT NOT NULL, url TEXT NOT NULL UNIQUE, title TEXT NOT NULL, summary TEXT NOT NULL, content TEXT NOT NULL, author TEXT NOT NULL, published_at TEXT NOT NULL, fetched_at TEXT NOT NULL, state TEXT NOT NULL, accepted INTEGER NOT NULL, reason TEXT NOT NULL, score_a REAL NOT NULL, score_b REAL NOT NULL, title_zh TEXT NOT NULL, summary_zh TEXT NOT NULL, reason_zh TEXT NOT NULL, tags TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 items: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_items_state ON items(state)"); err != nil {
		return fmt.Errorf("建索引 items: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_items_fetched ON items(fetched_at)"); err != nil {
		return fmt.Errorf("建索引 items: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS projects (full_name TEXT PRIMARY KEY, html_url TEXT NOT NULL, description TEXT NOT NULL, language TEXT NOT NULL, topics TEXT NOT NULL, stars INTEGER NOT NULL, forks INTEGER NOT NULL, trending_rank INTEGER NOT NULL, first_seen_at TEXT NOT NULL, last_seen_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 projects: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS project_snapshots (full_name TEXT NOT NULL, at TEXT NOT NULL, stars INTEGER NOT NULL, trending_rank INTEGER NOT NULL, search_rank INTEGER NOT NULL, PRIMARY KEY (full_name, at))"); err != nil {
		return fmt.Errorf("建表 project_snapshots: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_snapshots_name ON project_snapshots(full_name, at)"); err != nil {
		return fmt.Errorf("建索引 snapshots: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS stories (id TEXT PRIMARY KEY, kind TEXT NOT NULL, title_zh TEXT NOT NULL, summary_zh TEXT NOT NULL, url TEXT NOT NULL, members TEXT NOT NULL, projects TEXT NOT NULL, hotness REAL NOT NULL, first_seen_at TEXT NOT NULL, updated_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 stories: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_stories_kind ON stories(kind, hotness)"); err != nil {
		return fmt.Errorf("建索引 stories: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS story_history (story_id TEXT NOT NULL, at TEXT NOT NULL, hotness REAL NOT NULL, PRIMARY KEY (story_id, at))"); err != nil {
		return fmt.Errorf("建表 story_history: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS digests (date TEXT PRIMARY KEY, markdown TEXT NOT NULL, stats TEXT NOT NULL, created_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 digests: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS runs (id INTEGER PRIMARY KEY AUTOINCREMENT, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, status TEXT NOT NULL, stats TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 runs: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS llm_usage (id INTEGER PRIMARY KEY AUTOINCREMENT, phase TEXT NOT NULL, kind TEXT NOT NULL, model TEXT NOT NULL, prompt_tokens INTEGER NOT NULL, completion_tokens INTEGER NOT NULL, created_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 llm_usage: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS admin_sessions (id TEXT PRIMARY KEY, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, ip TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 admin_sessions: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS ip_events (id INTEGER PRIMARY KEY AUTOINCREMENT, ip TEXT NOT NULL, kind TEXT NOT NULL, detail TEXT NOT NULL, score INTEGER NOT NULL, created_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 ip_events: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_ip_events_time ON ip_events(created_at)"); err != nil {
		return fmt.Errorf("建索引 ip_events: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS ip_fingerprints (fp TEXT PRIMARY KEY, ips TEXT NOT NULL DEFAULT '[]', ua TEXT NOT NULL DEFAULT '', first_seen TEXT NOT NULL, last_seen TEXT NOT NULL, hits INTEGER NOT NULL DEFAULT 0)"); err != nil {
		return fmt.Errorf("建表 ip_fingerprints: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS ip_bans (ip TEXT PRIMARY KEY, strikes INTEGER NOT NULL DEFAULT 0, level INTEGER NOT NULL DEFAULT 1, reason TEXT NOT NULL DEFAULT '', banned_at TEXT NOT NULL, expires_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 ip_bans: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS ip_profiles (ip TEXT PRIMARY KEY, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL, reqs INTEGER NOT NULL DEFAULT 0, ua_set TEXT NOT NULL DEFAULT '[]', ua_last TEXT NOT NULL DEFAULT '')"); err != nil {
		return fmt.Errorf("建表 ip_profiles: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 settings: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS domestic_summaries (date TEXT PRIMARY KEY, summary TEXT NOT NULL, created_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 domestic_summaries: %w", err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS probe_results (id INTEGER PRIMARY KEY AUTOINCREMENT, target TEXT NOT NULL, kind TEXT NOT NULL, ok INTEGER NOT NULL, latency_ms INTEGER NOT NULL, detail TEXT NOT NULL, checked_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 probe_results: %w", err)
	}
	// 链接封面图缓存（og:image 懒抓取结果，APP 卡片封面用）
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS link_images (url TEXT PRIMARY KEY, image TEXT NOT NULL, fetched_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("建表 link_images: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_usage_created ON llm_usage(created_at)"); err != nil {
		return fmt.Errorf("建索引 usage: %w", err)
	}
	// 旧库增量迁移：列已存在时报错属预期，忽略
	_, _ = db.Exec("ALTER TABLE digests ADD COLUMN kind TEXT NOT NULL DEFAULT 'daily'")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN webrtc TEXT NOT NULL DEFAULT '[]'")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN components TEXT NOT NULL DEFAULT '{}'")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN flags TEXT NOT NULL DEFAULT '[]'")
	_, _ = db.Exec("ALTER TABLE stories ADD COLUMN overview TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE projects ADD COLUMN description_zh TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE sources ADD COLUMN current_interval_minutes INTEGER NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE sources ADD COLUMN empty_streak INTEGER NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE stories ADD COLUMN manual INTEGER NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE items ADD COLUMN content_zh TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE items ADD COLUMN meta TEXT NOT NULL DEFAULT '{}'")
	// P2 指纹数学列（感知哈希 / MinHash 签名 / 熵权 / 稳定度 / 分量稳定度）
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN canvas_phash TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN minhash_sig TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN entropy_bits REAL NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN stability REAL NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE ip_fingerprints ADD COLUMN comp_stability TEXT NOT NULL DEFAULT '{}'")
	// P2-5 地理与 ASN（ip-location-db mmdb，缺失时全部降级跳过）
	_, _ = db.Exec("ALTER TABLE ip_profiles ADD COLUMN asn INTEGER NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE ip_profiles ADD COLUMN asn_type TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE ip_profiles ADD COLUMN geo_country TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE ip_profiles ADD COLUMN geo_tz TEXT NOT NULL DEFAULT ''")
	// 指纹关联边（pHash 同源 / MinHash 相似 / 物理特征 / 时间共现）：P2 关联、P4-4 聚类、P6 图快照共用
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS fp_links (
		src TEXT NOT NULL, dst TEXT NOT NULL, kind TEXT NOT NULL,
		weight REAL NOT NULL DEFAULT 0, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL,
		PRIMARY KEY (src, dst, kind))`); err != nil {
		return fmt.Errorf("建表 fp_links: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_fp_links_src ON fp_links(src)"); err != nil {
		return fmt.Errorf("建索引 fp_links: %w", err)
	}
	// MinHash LSH 分桶（b=16 带 × r=8 行）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS fp_lsh_buckets (
		band INTEGER NOT NULL, bucket_hash TEXT NOT NULL, fp TEXT NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("建表 fp_lsh_buckets: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_lsh_bucket ON fp_lsh_buckets(band, bucket_hash)"); err != nil {
		return fmt.Errorf("建索引 fp_lsh_buckets: %w", err)
	}
	return nil
}

// RecordRun 记录一次流水线运行（参数化写入）。
func (db *DB) RecordRun(started, finished time.Time, status, statsJSON string) error {
	_, err := db.Exec(
		"INSERT INTO runs (started_at, finished_at, status, stats) VALUES (?, ?, ?, ?)",
		started.UTC().Format(time.RFC3339), finished.UTC().Format(time.RFC3339), status, statsJSON,
	)
	return err
}

// RunRecord 运行记录行。
type RunRecord struct {
	StartedAt string
	Status    string
	Duration  float64
	Stats     string
}

// ListRuns 最近 N 次运行记录（新在前）。
func (db *DB) ListRuns(ctx context.Context, limit int) ([]RunRecord, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT started_at, finished_at, status, stats FROM runs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunRecord
	for rows.Next() {
		var r RunRecord
		var finished string
		if err := rows.Scan(&r.StartedAt, &finished, &r.Status, &r.Stats); err != nil {
			return nil, err
		}
		st, err0 := time.Parse(time.RFC3339, r.StartedAt)
		ft, err1 := time.Parse(time.RFC3339, finished)
		if err0 == nil && err1 == nil {
			r.Duration = ft.Sub(st).Seconds()
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
