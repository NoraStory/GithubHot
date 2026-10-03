package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// TouchIPProfile 第一层 IP 记忆：登记一次访问（累加请求数、补充 UA 集合）。
// 高频调用由上层限频（每 IP 最多每 60s 落库一次），这里只做幂等 upsert。
func (db *DB) TouchIPProfile(ctx context.Context, ip, ua string, reqs int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	row := db.QueryRowContext(ctx, "SELECT ua_set FROM ip_profiles WHERE ip = ?", ip)
	var uaJSON string
	err := row.Scan(&uaJSON)
	if err != nil {
		// 新 IP：建档
		set := []string{}
		if ua != "" {
			set = append(set, ua)
		}
		b, _ := json.Marshal(set)
		_, err = db.ExecContext(ctx,
			"INSERT INTO ip_profiles (ip, first_seen, last_seen, reqs, ua_set, ua_last) VALUES (?, ?, ?, ?, ?, ?)",
			ip, now, now, reqs, string(b), ua)
		if err != nil {
			return fmt.Errorf("建档 IP 档案: %w", err)
		}
		return nil
	}
	var set []string
	_ = json.Unmarshal([]byte(uaJSON), &set)
	known := false
	for _, x := range set {
		if x == ua {
			known = true
			break
		}
	}
	if ua != "" && !known && len(set) < 20 {
		set = append(set, ua)
	}
	b, _ := json.Marshal(set)
	if ua == "" {
		ua = "(empty)"
	}
	_, err = db.ExecContext(ctx,
		"UPDATE ip_profiles SET last_seen = ?, reqs = reqs + ?, ua_set = ?, ua_last = ? WHERE ip = ?",
		now, reqs, string(b), ua, ip)
	if err != nil {
		return fmt.Errorf("更新 IP 档案: %w", err)
	}
	return nil
}

// IPProfileRow IP 档案行（后台下钻查看）。
type IPProfileRow struct {
	IP        string
	FirstSeen time.Time
	LastSeen  time.Time
	Reqs      int
	UASet     []string
	UALast    string
}

// FindIPProfile 查单个 IP 档案；无记录返回 (nil, nil)。
func (db *DB) FindIPProfile(ctx context.Context, ip string) (*IPProfileRow, error) {
	var p IPProfileRow
	var first, last, uaJSON string
	err := db.QueryRowContext(ctx,
		"SELECT ip, first_seen, last_seen, reqs, ua_set, ua_last FROM ip_profiles WHERE ip = ?", ip).
		Scan(&p.IP, &first, &last, &p.Reqs, &uaJSON, &p.UALast)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询 IP 档案: %w", err)
	}
	_ = json.Unmarshal([]byte(uaJSON), &p.UASet)
	p.FirstSeen, _ = time.Parse(time.RFC3339, first)
	p.LastSeen, _ = time.Parse(time.RFC3339, last)
	return &p, nil
}
