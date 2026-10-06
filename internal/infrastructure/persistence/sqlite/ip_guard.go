package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/fpcluster"
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
	// 参数验证：防止异常输入
	if seconds <= 0 || seconds > 86400*7 {
		return 0, fmt.Errorf("invalid seconds: %d (must be 0 < seconds <= 604800)", seconds)
	}
	
	since := time.Now().Add(-time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339)
	var total sql.NullInt64
	err := db.QueryRowContext(ctx,
		"SELECT SUM(score) FROM ip_events WHERE ip = ? AND created_at >= ?", ip, since).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("统计违规积分: %w", err)
	}
	
	// 防止整数溢出（32位系统）
	if total.Int64 > 2147483647 {
		return 2147483647
	}
	if total.Int64 < -2147483648 {
		return 0
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
	CanvasPHash string             // P2-1 感知哈希（hex16）
	MinHashSig  string             // P2-2 MinHash 签名（hex）
	EntropyBits float64            // P2-3 分量熵权（0=未计算）
	Stability   float64            // P2-4 整体稳定度（0-1）
	CompStabilityJSON string         // P2-4 各分量稳定度（原始 JSON）
	CompStability     map[string]float64
	JA4               string // P3-2 TLS 客户端指纹（TLS 模式下捕获；纯 HTTP 为空）
	AttestationJSON   string // P4-1 平台证明结果（原始 JSON，'{}'=未验证）
	GNNScore          *float64 // P6-3 GNN 推理 bot 概率（NULL=未打分）
	BehaviorJSON      string // P4-5 行为生物特征（滑窗统计量 JSON，'{}'=未采集）
	ClockSkewPPM      *float64 // P4-6 时钟偏移（ppm；NULL=未采集）
	ClusterID         *int64  // P4-4 图聚类簇归属（NULL=未聚类）
	AnomalyScore      *float64 // P5-2 iForest 异常分（NULL=未计算）
}

// UpsertFingerprint 登记一次指纹上报；返回该指纹历史上出现过的所有 IP。
// webrtc/components/flags 同前；canvasPhash/minhashSig 为 P2-1/P2-2 数学指纹；
// stability >= 0 时落库整体稳定度（P2-4），compStabilityJSON 非空时落库各分量稳定度；
// ja4 为 P3-2 TLS 客户端指纹（TLS 模式才有，非空覆盖）；
// behaviorJSON / clockSkewPPM 为 P4-5/P4-6 行为与时钟信号（非空/非 nil 覆盖）。
func (db *DB) UpsertFingerprint(ctx context.Context, fp, ip, ua string, webrtc []string, components map[string]string, flags []string, canvasPhash, minhashSig, ja4 string, stability float64, compStabilityJSON, behaviorJSON string, clockSkewPPM *float64) ([]string, error) {
	// 输入验证：防止资源耗尽攻击
	const (
		maxWebRTCCandidates  = 20
		maxFlags             = 50
		maxComponentKeys     = 100
		maxComponentValueLen = 1024
		maxUALen             = 512
	)
	
	if len(webrtc) > maxWebRTCCandidates {
		webrtc = webrtc[:maxWebRTCCandidates]
	}
	if len(flags) > maxFlags {
		flags = flags[:maxFlags]
	}
	if len(components) > maxComponentKeys {
		return nil, fmt.Errorf("components数量超限: %d > %d", len(components), maxComponentKeys)
	}
	for k, v := range components {
		if len(k) > 64 {
			return nil, fmt.Errorf("component key过长: %d", len(k))
		}
		if len(v) > maxComponentValueLen {
			return nil, fmt.Errorf("component value过长: %d", len(v))
		}
	}
	if len(ua) > maxUALen {
		ua = ua[:maxUALen]
	}
	
	// 验证 clockSkewPPM 范围
	if clockSkewPPM != nil && (math.IsNaN(*clockSkewPPM) || math.Abs(*clockSkewPPM) > 1e6) {
		clockSkewPPM = nil // 非法值视为未采集
	}
	
	// 合并查询：一次获取所有需要的字段，避免 N+1
	row := db.QueryRowContext(ctx,
		"SELECT ips, ua, hits, webrtc, flags, components FROM ip_fingerprints WHERE fp = ?", fp)
	var ipsJSON, oldUA, rtcJSON, flagsJSON, oldCompJSON string
	var hits int
	err := row.Scan(&ipsJSON, &oldUA, &hits, &rtcJSON, &flagsJSON, &oldCompJSON)
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
	// 限制 rtc 累积大小
	if len(rtc) > maxWebRTCCandidates {
		rtc = rtc[len(rtc)-maxWebRTCCandidates:]
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
	// 限制 flags 累积大小
	if len(knownFlags) > maxFlags {
		knownFlags = knownFlags[len(knownFlags)-maxFlags:]
	}
	
	rb, _ := json.Marshal(rtc)
	fb, _ := json.Marshal(knownFlags)
	// components 渐进合并：新分量非空才覆盖，空值保留旧值（APP WebView 分多次补齐四维）
	oldComponents := map[string]string{}
	if err == nil {
		_ = json.Unmarshal([]byte(oldCompJSON), &oldComponents)
	}
	for k, v := range components {
		if v != "" {
			oldComponents[k] = v
		}
	}
	// 限制 oldComponents 大小
	if len(oldComponents) > maxComponentKeys {
		// 保留最新的 maxComponentKeys 个（简单截断）
		keys := make([]string, 0, len(oldComponents))
		for k := range oldComponents {
			keys = append(keys, k)
		}
		if len(keys) > maxComponentKeys {
			for i := 0; i < len(keys)-maxComponentKeys; i++ {
				delete(oldComponents, keys[i])
			}
		}
	}
	
	cb, _ := json.Marshal(oldComponents)
	if err == sql.ErrNoRows {
		ips = []string{ip}
		b, _ := json.Marshal(ips)
		_, err = db.ExecContext(ctx,
			"INSERT INTO ip_fingerprints (fp, ips, ua, first_seen, last_seen, hits, webrtc, components, flags, canvas_phash, minhash_sig, stability, comp_stability, ja4, behavior, clock_skew_ppm) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			fp, string(b), ua, now, now, string(rb), string(cb), string(fb), canvasPhash, minhashSig, stability, compStabilityJSON, ja4, behaviorJSON, clockSkewPPM)
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
		"UPDATE ip_fingerprints SET ips = ?, ua = ?, last_seen = ?, hits = hits + 1, webrtc = ?, components = ?, flags = ?, canvas_phash = CASE WHEN ? != '' THEN ? ELSE canvas_phash END, minhash_sig = CASE WHEN ? != '' THEN ? ELSE minhash_sig END, stability = CASE WHEN ? >= 0 THEN ? ELSE stability END, comp_stability = CASE WHEN ? != '' THEN ? ELSE comp_stability END, ja4 = CASE WHEN ? != '' THEN ? ELSE ja4 END, behavior = CASE WHEN ? != '' THEN ? ELSE behavior END, clock_skew_ppm = COALESCE(?, clock_skew_ppm) WHERE fp = ?",
		string(b), ua, now, string(rb), string(cb), string(fb), canvasPhash, canvasPhash, minhashSig, minhashSig, stability, stability, compStabilityJSON, compStabilityJSON, ja4, ja4, behaviorJSON, behaviorJSON, clockSkewPPM, fp)
	if err != nil {
		return nil, fmt.Errorf("更新指纹: %w", err)
	}
	return ips, nil
}

// scanFingerprintRows 统一扫描指纹查询结果（含 webrtc/components/flags 与 P2 数学指纹列）。
func scanFingerprintRows(rows *sql.Rows) ([]FingerprintRow, error) {
	out := []FingerprintRow{}
	for rows.Next() {
		var f FingerprintRow
		var ipsJSON, rtcJSON, compJSON, flagsJSON, first, last string
		if err := rows.Scan(&f.Fingerprint, &ipsJSON, &rtcJSON, &compJSON, &flagsJSON, &f.UA, &first, &last, &f.Hits,
			&f.CanvasPHash, &f.MinHashSig, &f.EntropyBits, &f.Stability, &f.CompStabilityJSON, &f.JA4, &f.AttestationJSON, &f.GNNScore,
			&f.BehaviorJSON, &f.ClockSkewPPM, &f.ClusterID, &f.AnomalyScore); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(ipsJSON), &f.IPs)
		_ = json.Unmarshal([]byte(rtcJSON), &f.Webrtc)
		f.Components = map[string]string{}
		_ = json.Unmarshal([]byte(compJSON), &f.Components)
		_ = json.Unmarshal([]byte(flagsJSON), &f.Flags)
		f.CompStability = map[string]float64{}
		_ = json.Unmarshal([]byte(f.CompStabilityJSON), &f.CompStability)
		f.FirstSeen, _ = time.Parse(time.RFC3339, first)
		f.LastSeen, _ = time.Parse(time.RFC3339, last)
		out = append(out, f)
	}
	return out, rows.Err()
}

// fingerprintCols 指纹查询的统一列清单（P2 起含数学指纹与稳定度/熵权列；P3 起含 ja4）。
const fingerprintCols = "fp, ips, webrtc, components, flags, ua, first_seen, last_seen, hits, canvas_phash, minhash_sig, entropy_bits, stability, comp_stability, ja4, attestation, behavior, clock_skew_ppm, cluster_id, gnn_score"

// ListFingerprints 最近 limit 个活跃指纹。
func (db *DB) ListFingerprints(ctx context.Context, limit int) ([]FingerprintRow, error) {
	const maxFingerprintQueryLimit = 5000
	if limit <= 0 {
		limit = 20
	}
	if limit > maxFingerprintQueryLimit {
		limit = maxFingerprintQueryLimit
	}
	rows, err := db.QueryContext(ctx,
		"SELECT "+fingerprintCols+" FROM ip_fingerprints ORDER BY last_seen DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("查询指纹: %w", err)
	}
	defer rows.Close()
	return scanFingerprintRows(rows)
}

// PHashRow 感知哈希候选（P2-1 同源关联扫描用）。
type PHashRow struct {
	Fingerprint string
	PHash       string
	LastSeen    time.Time
}

// ListPHashCandidates 时间窗内带感知哈希的指纹（新指纹入库时扫描同源）。
func (db *DB) ListPHashCandidates(ctx context.Context, since time.Time, limit int) ([]PHashRow, error) {
	if limit <= 0 {
		limit = 2000
	}
	rows, err := db.QueryContext(ctx,
		"SELECT fp, canvas_phash, last_seen FROM ip_fingerprints WHERE canvas_phash != '' AND last_seen >= ? ORDER BY last_seen DESC LIMIT ?",
		since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("查询 pHash 候选: %w", err)
	}
	defer rows.Close()
	out := []PHashRow{}
	for rows.Next() {
		var r PHashRow
		var last string
		if err := rows.Scan(&r.Fingerprint, &r.PHash, &last); err != nil {
			return nil, err
		}
		r.LastSeen, _ = time.Parse(time.RFC3339, last)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- P2-2 LSH 桶 / P2-3 熵权 / P2-5 IP 地理 ----------

// LSHBandRow LSH 桶定位行（band 号 + 桶键，见 fpmath.Bands 的输出键）。
type LSHBandRow struct {
	Band int
	Hash string
}

// UpsertLSHBands 重写某指纹的 LSH 桶记录（先删后插，幂等；签名变化时旧桶自动失效）。
// 优化版：限制 bands 数量，防止DoS攻击。
func (db *DB) UpsertLSHBands(ctx context.Context, fp string, bands []LSHBandRow) error {
	if fp == "" || len(bands) == 0 {
		return nil
	}
	
	// 限制：最多50个band（正常为16）
	const maxLSHBands = 50
	if len(bands) > maxLSHBands {
		return fmt.Errorf("LSH bands数量超限: %d > %d", len(bands), maxLSHBands)
	}
	
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "DELETE FROM fp_lsh_buckets WHERE fp = ?", fp); err != nil {
		return fmt.Errorf("清空 LSH 桶: %w", err)
	}
	for _, b := range bands {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO fp_lsh_buckets (band, bucket_hash, fp, created_at) VALUES (?, ?, ?, ?)",
			b.Band, b.Hash, fp, now); err != nil {
			return fmt.Errorf("写 LSH 桶: %w", err)
		}
	}
	return nil
}

// ListLSHCandidates 同带召回：与目标指纹共享任一 LSH 桶的其他指纹（不含自身）。
// 优化版：限制 bands 数量，防止查询过大。
func (db *DB) ListLSHCandidates(ctx context.Context, fp string, bands []LSHBandRow, limit int) ([]string, error) {
	if fp == "" || len(bands) == 0 {
		return nil, nil
	}
	
	// 限制：最多50个band（正常为16）
	const maxLSHBands = 50
	if len(bands) > maxLSHBands {
		return nil, fmt.Errorf("LSH bands数量超限: %d > %d", len(bands), maxLSHBands)
	}
	
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	
	// 使用 strings.Builder 提升性能
	var query strings.Builder
	query.WriteString("SELECT DISTINCT fp FROM fp_lsh_buckets WHERE fp != ? AND (")
	args := []any{fp}
	for i, b := range bands {
		if i > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(band = ? AND bucket_hash = ?)")
		args = append(args, b.Band, b.Hash)
	}
	query.WriteString(") LIMIT ?")
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("召回 LSH 候选: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MinHashSigRow 候选指纹的 MinHash 签名（LSH 召回后精确 Jaccard 复核用）。
type MinHashSigRow struct {
	FP  string
	Sig string
}

// ListMinHashSigs 批量取候选指纹的 MinHash 签名（精确 Jaccard 复核用）。
// 优化版：限制输入大小，批量处理防止查询崩溃。
func (db *DB) ListMinHashSigs(ctx context.Context, fps []string) ([]MinHashSigRow, error) {
	out := []MinHashSigRow{}
	if len(fps) == 0 {
		return out, nil
	}
	
	// 限制：最多5000个指纹
	const maxBatchSize = 500
	if len(fps) > 5000 {
		return nil, fmt.Errorf("fps数组过大: %d > 5000", len(fps))
	}
	
	// 分批查询，每批最多500个占位符
	for i := 0; i < len(fps); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(fps) {
			end = len(fps)
		}
		batch := fps[i:end]
		
		// 构建查询（使用 strings.Builder 提升性能）
		var query strings.Builder
		query.WriteString("SELECT fp, minhash_sig FROM ip_fingerprints WHERE minhash_sig != '' AND fp IN (")
		args := make([]any, 0, len(batch))
		for j, f := range batch {
			if j > 0 {
				query.WriteString(",")
			}
			query.WriteString("?")
			args = append(args, f)
		}
		query.WriteString(")")
		
		rows, err := db.QueryContext(ctx, query.String(), args...)
		if err != nil {
			return nil, fmt.Errorf("查 MinHash 签名(批次%d): %w", i/maxBatchSize, err)
		}
		
		for rows.Next() {
			var fp, sig string
			if err := rows.Scan(&fp, &sig); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, MinHashSigRow{FP: fp, Sig: sig})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	
	return out, nil
}

// FindFingerprint 查单条指纹档案（稳定性/轮换检测/熵权读取用）；无记录返回 (nil, nil)。
func (db *DB) FindFingerprint(ctx context.Context, fp string) (*FingerprintRow, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT "+fingerprintCols+" FROM ip_fingerprints WHERE fp = ?", fp)
	if err != nil {
		return nil, fmt.Errorf("查指纹: %w", err)
	}
	defer rows.Close()
	list, err := scanFingerprintRows(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// UpdateEntropyBits 每日 cron 刷新分量熵权（P2-3）。
func (db *DB) UpdateEntropyBits(ctx context.Context, fp string, bits float64) error {
	_, err := db.ExecContext(ctx, "UPDATE ip_fingerprints SET entropy_bits = ? WHERE fp = ?", bits, fp)
	if err != nil {
		return fmt.Errorf("写熵权: %w", err)
	}
	return nil
}

// UpdateIPGeo 补全 IP 档案的地理/ASN 信息（P2-5）。upsert 语义：第一层档案行是
// 延迟刷库的，地理核验时刻档案行可能尚不存在——这里直接建档（reqs=0 占位，
// 后续 TouchIPProfile 的 SELECT-then-INSERT/UPDATE 路径都能正确接管）。
func (db *DB) UpdateIPGeo(ctx context.Context, ip string, asn uint, asnType, country, tz string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx,
		`INSERT INTO ip_profiles (ip, first_seen, last_seen, reqs, ua_set, ua_last, asn, asn_type, geo_country, geo_tz)
		 VALUES (?, ?, ?, 0, '[]', '', ?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET asn = excluded.asn, asn_type = excluded.asn_type,
		   geo_country = excluded.geo_country, geo_tz = excluded.geo_tz`,
		ip, now, now, int(asn), asnType, country, tz)
	if err != nil {
		return fmt.Errorf("写 IP 地理: %w", err)
	}
	return nil
}

// UpdateAttestation 写回平台证明结果（P4-1）。
func (db *DB) UpdateAttestation(ctx context.Context, fp string, attestationJSON string) error {
	_, err := db.ExecContext(ctx,
		"UPDATE ip_fingerprints SET attestation = CASE WHEN ? != '' THEN ? ELSE attestation END WHERE fp = ?",
		attestationJSON, attestationJSON, fp)
	if err != nil {
		return fmt.Errorf("写平台证明: %w", err)
	}
	return nil
}

// ---------- P4-4 图聚类存储 ----------

// ReplaceClusters 整表重写簇（每日 cron 全量重算）：先清 cluster_id 与旧簇行，
// 再插入新簇并回填成员 cluster_id。簇数与成员规模小（<1e5），事务内完成。
func (db *DB) ReplaceClusters(ctx context.Context, clusters []fpcluster.Cluster) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开事务: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE ip_fingerprints SET cluster_id = NULL WHERE cluster_id IS NOT NULL"); err != nil {
		return fmt.Errorf("清空簇归属: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM fp_clusters"); err != nil {
		return fmt.Errorf("清空簇表: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, cl := range clusters {
		members, _ := json.Marshal(cl.Members)
		res, err := tx.ExecContext(ctx,
			"INSERT INTO fp_clusters (member_fps, size, first_seen, reason) VALUES (?, ?, ?, ?)",
			string(members), cl.Size, now, cl.Reason)
		if err != nil {
			return fmt.Errorf("写簇: %w", err)
		}
		id, _ := res.LastInsertId()
		for _, fp := range cl.Members {
			if _, err := tx.ExecContext(ctx, "UPDATE ip_fingerprints SET cluster_id = ? WHERE fp = ?", id, fp); err != nil {
				return fmt.Errorf("回填簇归属: %w", err)
			}
		}
	}
	return tx.Commit()
}

// ListAllFPLinks 全部关联边（时间窗内，聚类构图用；与 ListFPLinks 的单指纹查询相对）。
func (db *DB) ListAllFPLinks(ctx context.Context, since time.Time, limit int) ([]FPLinkRow, error) {
	if limit <= 0 {
		limit = 100000
	}
	rows, err := db.QueryContext(ctx,
		"SELECT src, dst, kind, weight, first_seen, last_seen FROM fp_links WHERE last_seen >= ? ORDER BY last_seen DESC LIMIT ?",
		since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("查询关联边: %w", err)
	}
	defer rows.Close()
	out := []FPLinkRow{}
	for rows.Next() {
		var l FPLinkRow
		var fs, ls string
		if err := rows.Scan(&l.Src, &l.Dst, &l.Kind, &l.Weight, &fs, &ls); err != nil {
			return nil, err
		}
		l.FirstSeen, _ = time.Parse(time.RFC3339, fs)
		l.LastSeen, _ = time.Parse(time.RFC3339, ls)
		out = append(out, l)
	}
	return out, rows.Err()
}

// ClusterRowDTO 簇档案（管理端集群视图用）。
type ClusterRowDTO struct {
	ID        int64
	Members   []string
	Size      int
	FirstSeen time.Time
	Reason    string
}

// ListClusters 簇列表（按 size 降序）。
func (db *DB) ListClusters(ctx context.Context, limit int) ([]ClusterRowDTO, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.QueryContext(ctx,
		"SELECT id, member_fps, size, first_seen, reason FROM fp_clusters ORDER BY size DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("查询簇: %w", err)
	}
	defer rows.Close()
	out := []ClusterRowDTO{}
	for rows.Next() {
		var c ClusterRowDTO
		var members, fs string
		if err := rows.Scan(&c.ID, &members, &c.Size, &fs, &c.Reason); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(members), &c.Members)
		c.FirstSeen, _ = time.Parse(time.RFC3339, fs)
		out = append(out, c)
	}
	return out, rows.Err()
}

// FPLinkRow 指纹关联边（pHash 同源 / MinHash 相似 / 物理特征 / 时间共现）。
type FPLinkRow struct {
	Src, Dst, Kind      string
	Weight              float64
	FirstSeen, LastSeen time.Time
}

// UpsertFPLink 记录一条指纹关联边（幂等：重复命中刷新权重与 last_seen）。
func (db *DB) UpsertFPLink(ctx context.Context, src, dst, kind string, weight float64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx,
		`INSERT INTO fp_links (src, dst, kind, weight, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(src, dst, kind) DO UPDATE SET weight = excluded.weight, last_seen = excluded.last_seen`,
		src, dst, kind, weight, now, now)
	if err != nil {
		return fmt.Errorf("写指纹关联: %w", err)
	}
	return nil
}

// ListFPLinks 某指纹的关联边（source 或 target 任一侧命中）。
func (db *DB) ListFPLinks(ctx context.Context, fp string, limit int) ([]FPLinkRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx,
		"SELECT src, dst, kind, weight, first_seen, last_seen FROM fp_links WHERE src = ? OR dst = ? ORDER BY last_seen DESC LIMIT ?",
		fp, fp, limit)
	if err != nil {
		return nil, fmt.Errorf("查询指纹关联: %w", err)
	}
	defer rows.Close()
	out := []FPLinkRow{}
	for rows.Next() {
		var r FPLinkRow
		var first, last string
		if err := rows.Scan(&r.Src, &r.Dst, &r.Kind, &r.Weight, &first, &last); err != nil {
			return nil, err
		}
		r.FirstSeen, _ = time.Parse(time.RFC3339, first)
		r.LastSeen, _ = time.Parse(time.RFC3339, last)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListFingerprintsSince 时间窗内的活跃指纹（flag 命中统计用）。// last_seen 按 RFC3339 文本存储，同格式比较即字典序比较。
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
	// 使用 JSON 函数避免 LIKE 注入风险
	rows, err := db.QueryContext(ctx,
		`SELECT `+fingerprintCols+` FROM ip_fingerprints 
		 WHERE EXISTS(
		   SELECT 1 FROM json_each(ips) WHERE value = ?
		 ) ORDER BY last_seen DESC LIMIT 50`,
		ip)
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
	expiresAt := now.Add(duration).UTC()
	// 优化：封禁期间幂等保护，只在未封禁或已过期时才更新
	_, err := db.ExecContext(ctx,
		`INSERT INTO ip_bans (ip, strikes, level, reason, banned_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET 
		   strikes = CASE 
		     WHEN datetime(expires_at) > datetime('now') THEN strikes  -- 封禁中不升级
		     ELSE excluded.strikes 
		   END,
		   level = CASE 
		     WHEN datetime(expires_at) > datetime('now') THEN level
		     ELSE excluded.level
		   END,
		   reason = CASE 
		     WHEN datetime(expires_at) > datetime('now') THEN reason
		     ELSE excluded.reason
		   END,
		   banned_at = CASE 
		     WHEN datetime(expires_at) > datetime('now') THEN banned_at
		     ELSE excluded.banned_at
		   END,
		   expires_at = CASE 
		     WHEN datetime(expires_at) > datetime('now') THEN expires_at
		     ELSE excluded.expires_at
		   END`,
		ip, strikes, level, reason, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
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
