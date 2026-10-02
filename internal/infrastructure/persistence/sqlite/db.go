// Package sqlite 是全部领域仓储的持久化实现（modernc.org/sqlite，纯 Go 无 CGO，
// 单二进制跨平台部署）。DDL 为内联字面量，幂等可重复执行；
// 空串默认值统一由仓储层代码保证，不依赖 SQL DEFAULT。
package sqlite

import (
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
