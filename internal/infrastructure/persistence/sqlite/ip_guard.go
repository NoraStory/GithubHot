package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ---------- IP 治理存储：事件 / 指纹 / 封禁 ----------

// IPEventRow 违规事件行。
type IPEventRow struct {
	ID     int64
	IP     string
	Kind   string
	Detail string
	Score  int
	At     time.Time
}

// AddIPEvent 记录一条违规事件。
func (db *DB) AddIPEvent(ctx context.Context, ip, kind, detail string, score int) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO ip_events (ip, kind, detail, score, created_at) VALUES (?, ?, ?, ?, ?)",
		ip, kind, detail, score, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("写入违规事件: %w", err)
	}
	// 顺手清理 7 天前旧事件，防表膨胀
	_, _ = db.ExecContext(ctx, "DELETE FROM ip_events WHERE created_at < ?",
		time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339))
	return nil
}

// RecentIPEventsScore 该 IP 最近 duration 秒内的事件积分合计。
func (db *DB) RecentIPEventsScore(ctx context.Context, ip string, seconds int) (int, error) {
	since := time.Now().Add(-time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339)
	var total sql.NullInt64
	err := db.QueryRowContext(ctx,
		"SELECT SUM(score) FROM ip_events WHERE ip = ? AND created_at >= ?", ip, since).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("统计违规积分: %w", err)
	}
	return int(total.Int64), nil
}

// ListIPEvents 最近 limit 条违规事件（新的在前）。
func (db *DB) ListIPEvents(ctx context.Context, limit int) ([]IPEventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx,
		"SELECT id, ip, kind, detail, score, created_at FROM ip_events ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("查询违规事件: %w", err)
	}
	defer rows.Close()
	return scanIPEvents(rows)
}

func scanIPEvents(rows *sql.Rows) ([]IPEventRow, error) {
	out := []IPEventRow{}
	for rows.Next() {
		var e IPEventRow
		var at string
		if err := rows.Scan(&e.ID, &e.IP, &e.Kind, &e.Detail, &e.Score, &at); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return nil, err
		}
		e.At = t
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------- 指纹 ----------

// FingerprintRow 指纹行。
type FingerprintRow struct {
	Fingerprint string
	IPs         []string
	Webrtc      []string // WebRTC 探测到的真实 IP（host/srflx 候选）
	Components  map[string]string
	Flags       []string // 第三层环境核验命中项（如 headless-ua）
	UA          string
	FirstSeen   time.Time
	LastSeen    time.Time
	Hits        int
}

// UpsertFingerprint 登记一次指纹上报；返回该指纹历史上出现过的所有 IP。
// meta：WebRTC IP、分量明细（components）、环境核验命中（flags，做并集累计）。
func (db *DB) UpsertFingerprint(ctx context.Context, fp, ip, ua string, webrtc []string, components map[string]string, flags []string) ([]string, error) {
	row := db.QueryRowContext(ctx,
		"SELECT ips, ua, hits, webrtc, flags FROM ip_fingerprints WHERE fp = ?", fp)
	var ipsJSON, oldUA, rtcJSON, flagsJSON string
	var hits int
	err := row.Scan(&ipsJSON, &oldUA, &hits, &rtcJSON, &flagsJSON)
	now := time.Now().UTC().Format(time.RFC3339)
	ips := []string{}
	rtc := []string{}
	knownFlags := []string{}
	_ = json.Unmarshal([]byte(rtcJSON), &rtc)
	_ = json.Unmarshal([]byte(flagsJSON), &knownFlags)
	for _, w := range webrtc {
		known := false
		for _, x := range rtc {
			if x == w {
				known = true
				break
			}
		}
		if !known {
			rtc = append(rtc, w)
		}
	}
	for _, f := range flags {
		known := false
		for _, x := range knownFlags {
			if x == f {
				known = true
				break
			}
		}
		if !known {
			knownFlags = append(knownFlags, f)
		}
	}
	rb, _ := json.Marshal(rtc)
	fb, _ := json.Marshal(knownFlags)
	// components 渐进合并：新分量非空才覆盖，空值保留旧值（APP WebView 分多次补齐四维）
	oldComponents := map[string]string{}
	if err == nil {
		var oldCompJSON string
		_ = db.QueryRowContext(ctx,
			"SELECT components FROM ip_fingerprints WHERE fp = ?", fp).Scan(&oldCompJSON)
		_ = json.Unmarshal([]byte(oldCompJSON), &oldComponents)
	}
	for k, v := range components {
		if v != "" {
			oldComponents[k] = v
		}
	}
	cb, _ := json.Marshal(oldComponents)
	if err == sql.ErrNoRows {
		ips = []string{ip}
		b, _ := json.Marshal(ips)
		_, err = db.ExecContext(ctx,
			"INSERT INTO ip_fingerprints (fp, ips, ua, first_seen, last_seen, hits, webrtc, components, flags) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)",
			fp, string(b), ua, now, now, string(rb), string(cb), string(fb))
		if err != nil {
			return nil, fmt.Errorf("写入指纹: %w", err)
		}
		return ips, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询指纹: %w", err)
	}
	_ = json.Unmarshal([]byte(ipsJSON), &ips)
	known := false
	for _, x := range ips {
		if x == ip {
			known = true
			break
		}
	}
	if !known {
		ips = append(ips, ip)
	}
	b, _ := json.Marshal(ips)
	if ua == "" {
		ua = oldUA
	}
	_, err = db.ExecContext(ctx,
		"UPDATE ip_fingerprints SET ips = ?, ua = ?, last_seen = ?, hits = hits + 1, webrtc = ?, components = ?, flags = ? WHERE fp = ?",
		string(b), ua, now, string(rb), string(cb), string(fb), fp)
	if err != nil {
		return nil, fmt.Errorf("更新指纹: %w", err)
	}
	return ips, nil
}

// scanFingerprintRows 统一扫描指纹查询结果（含 webrtc/components/flags 列）。
func scanFingerprintRows(rows *sql.Rows) ([]FingerprintRow, error) {
	out := []FingerprintRow{}
	for rows.Next() {
		var f FingerprintRow
		var ipsJSON, rtcJSON, compJSON, flagsJSON, first, last string
		if err := rows.Scan(&f.Fingerprint, &ipsJSON, &rtcJSON, &compJSON, &flagsJSON, &f.UA, &first, &last, &f.Hits); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(ipsJSON), &f.IPs)
		_ = json.Unmarshal([]byte(rtcJSON), &f.Webrtc)
		f.Components = map[string]string{}
		_ = json.Unmarshal([]byte(compJSON), &f.Components)
		_ = json.Unmarshal([]byte(flagsJSON), &f.Flags)
		f.FirstSeen, _ = time.Parse(time.RFC3339, first)
		f.LastSeen, _ = time.Parse(time.RFC3339, last)
		out = append(out, f)
	}
	return out, rows.Err()
}

// fingerprintCols 指纹查询的统一列清单。
const fingerprintCols = "fp, ips, webrtc, components, flags, ua, first_seen, last_seen, hits"

// ListFingerprints 最近 limit 个活跃指纹。
func (db *DB) ListFingerprints(ctx context.Context, limit int) ([]FingerprintRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx,
		"SELECT "+fingerprintCols+" FROM ip_fingerprints ORDER BY last_seen DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("查询指纹: %w", err)
	}
	defer rows.Close()
	return scanFingerprintRows(rows)
}

// ListFingerprintsSince 时间窗内的活跃指纹（flag 命中统计用）。
// last_seen 按 RFC3339 文本存储，同格式比较即字典序比较。
func (db *DB) ListFingerprintsSince(ctx context.Context, since time.Time, limit int) ([]FingerprintRow, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := db.QueryContext(ctx,
		"SELECT "+fingerprintCols+" FROM ip_fingerprints WHERE last_seen >= ? ORDER BY last_seen DESC LIMIT ?",
		since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("查询窗口内指纹: %w", err)
	}
	defer rows.Close()
	return scanFingerprintRows(rows)
}

// ListFingerprintsByIP 反查：IPS JSON 中包含该 IP 的指纹（IP 下钻用）。
func (db *DB) ListFingerprintsByIP(ctx context.Context, ip string) ([]FingerprintRow, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT "+fingerprintCols+" FROM ip_fingerprints WHERE ips LIKE ? ORDER BY last_seen DESC LIMIT 50",
		"%\""+ip+"\"%")
	if err != nil {
		return nil, fmt.Errorf("反查指纹: %w", err)
	}
	defer rows.Close()
	return scanFingerprintRows(rows)
}

// ListIPEventsByIP 该 IP 的最近违规事件（IP 下钻用）。
func (db *DB) ListIPEventsByIP(ctx context.Context, ip string, limit int) ([]IPEventRow, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := db.QueryContext(ctx,
		"SELECT id, ip, kind, detail, score, created_at FROM ip_events WHERE ip = ? ORDER BY id DESC LIMIT ?", ip, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 IP 事件: %w", err)
	}
	defer rows.Close()
	return scanIPEvents(rows)
}

// ListIPEventsSince 最近 since 以来的违规事件（时间筛选用；since 为零值则不限制）。
func (db *DB) ListIPEventsSince(ctx context.Context, limit int, since time.Time) ([]IPEventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if since.IsZero() {
		rows, err = db.QueryContext(ctx,
			"SELECT id, ip, kind, detail, score, created_at FROM ip_events ORDER BY id DESC LIMIT ?", limit)
	} else {
		rows, err = db.QueryContext(ctx,
			"SELECT id, ip, kind, detail, score, created_at FROM ip_events WHERE created_at >= ? ORDER BY id DESC LIMIT ?",
			since.UTC().Format(time.RFC3339), limit)
	}
	if err != nil {
		return nil, fmt.Errorf("查询违规事件: %w", err)
	}
	defer rows.Close()
	return scanIPEvents(rows)
}

// ---------- 封禁 ----------

// BanRow 封禁行。
type BanRow struct {
	IP        string
	Strikes   int
	Level     int
	Reason    string
	BannedAt  time.Time
	ExpiresAt time.Time
}

// FindBan 查单个 IP 封禁；无记录返回 (nil, nil)。
func (db *DB) FindBan(ctx context.Context, ip string) (*BanRow, error) {
	var b BanRow
	var bannedAt, expiresAt string
	err := db.QueryRowContext(ctx,
		"SELECT ip, strikes, level, reason, banned_at, expires_at FROM ip_bans WHERE ip = ?", ip).
		Scan(&b.IP, &b.Strikes, &b.Level, &b.Reason, &bannedAt, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询封禁: %w", err)
	}
	b.BannedAt, _ = time.Parse(time.RFC3339, bannedAt)
	b.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	return &b, nil
}

// BannedAmong 给定 IP 列表，返回其中当前仍在封禁期内的 IP。
func (db *DB) BannedAmong(ctx context.Context, ips []string) ([]string, error) {
	if len(ips) == 0 {
		return nil, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := []string{}
	for _, ip := range ips {
		var n int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM ip_bans WHERE ip = ? AND expires_at > ?", ip, now).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			out = append(out, ip)
		}
	}
	return out, nil
}

// UpsertBan 写入/升级封禁（按已有 strike 递增由调用方算好传入）。
func (db *DB) UpsertBan(ctx context.Context, ip string, strikes, level int, reason string, duration time.Duration) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx,
		`INSERT INTO ip_bans (ip, strikes, level, reason, banned_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET strikes = excluded.strikes, level = excluded.level,
			reason = excluded.reason, banned_at = excluded.banned_at, expires_at = excluded.expires_at`,
		ip, strikes, level, reason, now.Format(time.RFC3339), now.Add(duration).UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("写入封禁: %w", err)
	}
	return nil
}

// ListBans 全部未过期封禁（按过期时间升序）。
func (db *DB) ListBans(ctx context.Context) ([]BanRow, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT ip, strikes, level, reason, banned_at, expires_at FROM ip_bans WHERE expires_at > ? ORDER BY expires_at ASC",
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("查询封禁列表: %w", err)
	}
	defer rows.Close()
	out := []BanRow{}
	for rows.Next() {
		var b BanRow
		var bannedAt, expiresAt string
		if err := rows.Scan(&b.IP, &b.Strikes, &b.Level, &b.Reason, &bannedAt, &expiresAt); err != nil {
			return nil, err
		}
		b.BannedAt, _ = time.Parse(time.RFC3339, bannedAt)
		b.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteBan 解封。
func (db *DB) DeleteBan(ctx context.Context, ip string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM ip_bans WHERE ip = ?", ip)
	if err != nil {
		return fmt.Errorf("解除封禁: %w", err)
	}
	return nil
}
