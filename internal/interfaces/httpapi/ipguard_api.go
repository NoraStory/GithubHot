package httpapi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NoraStory/GithubHot/internal/domain/fpmath"
	"github.com/NoraStory/GithubHot/internal/domain/fpstats"
)

// realIP 从请求中提取真实IP地址
// 注意：无条件信任 X-Real-IP/X-Forwarded-For，与 clientip.go 的 TRUSTED_PROXY
// 受信代理体系不一致（评审 P2 遗留：公开端点可被栽赃任意 IP），待统一收口。
func realIP(r *http.Request) string {
	// 优先使用X-Real-IP
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// 其次使用X-Forwarded-For的第一个IP
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// 最后使用RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// ---------- 指纹上报（公开端点，第二层入口） ----------

type fpPayload struct {
	Fingerprint string            `json:"fp"`
	WebRTC      []string          `json:"webrtc"`
	Canvas      string            `json:"canvas"`
	WebGL       string            `json:"webgl"`
	Audio       string            `json:"audio"`
	Fonts       string            `json:"fonts"`
	Components  map[string]string `json:"components"` // 各技术分量指纹明细（canvas/webgl/audio/fonts/screen/renderer）
	Flags       []string          `json:"flags"`      // 第三层环境核验命中项
	Renderer    string            `json:"renderer"`
	Screen      string            `json:"screen"`
	Coherent    bool              `json:"coherent"`     // 旧客户端兼容：UA 与 platform 一致性
	CanvasPHash string            `json:"canvas_phash"` // P2-1 64bit 感知哈希（hex16，可选）
	MinHashSig  string            `json:"minhash_sig"`  // 已废弃：签名改由服务端计算（客户端值不采信，防 LSH 桶投毒）
	Sets        map[string][]string `json:"sets"`         // P2-2 原始清单（fonts/webgl_exts/plugins，可选）
	TZ          string              `json:"tz"`           // P2-5 客户端 IANA 时区（可选）
	TZOffsetMin int                 `json:"tz_offset_min"` // P2-5 时区偏移分钟数（可选）
	Altcha      *altchaSolution     `json:"altcha"`        // P4-3 PoW 解（强制开启时必需；可选字段，旧服务端忽略）
	Behavior    json.RawMessage     `json:"behavior"`       // P4-5 行为滑窗统计量 JSON（可选）
	ClockSkewPPM *float64           `json:"clock_skew_ppm"` // P4-6 时钟偏移 ppm（可选）
}

// sanitizePHash 校验客户端上报的感知哈希（P2-1）：必须 hex16，否则丢弃（不关联）。
func sanitizePHash(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != fpmath.PHashHexLen {
		return ""
	}
	if _, err := fpmath.ParsePHash(s); err != nil {
		return ""
	}
	return s
}

// sanitizeSets 清洗原始清单（P2-2）：仅认 fonts/webgl_exts/plugins 三个键，
// 每键最多 64 项、单项 ≤ 64 字节，防伪造超大清单撑爆签名计算与存储。
func sanitizeSets(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, key := range []string{"fonts", "webgl_exts", "plugins"} {
		for _, v := range in[key] {
			v = strings.TrimSpace(v)
			if v == "" || len(v) > 64 {
				continue
			}
			if len(out[key]) >= 64 {
				break
			}
			out[key] = append(out[key], v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeBehavior 清洗行为特征 JSON（P4-5）：长度受限；结构合法性由引擎 behavior.Parse 校验。
func sanitizeBehavior(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 8192 {
		return ""
	}
	return s
}

// sanitizeTZ 清洗客户端 IANA 时区名（P2-5）：长度受限、仅允许时区名的合法字符。
func sanitizeTZ(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 {
		return ""
	}
	for _, r := range s {
		ok := r == '/' || r == '_' || r == '-' || r == '+' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return ""
		}
	}
	return s
}

// sanitizeComponents 清洗上报的分量明细：键值长度上限 + 键数上限，
// 防止伪造超大/超多的 components 撑爆存储（键名 32 字节、值 128 字节、最多 16 项）。
func sanitizeComponents(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if len(out) >= 16 {
			break
		}
		if k == "" || len(k) > 32 || len(v) > 128 {
			continue
		}
		out[k] = v
	}
	return out
}

// flagScore 第三层各命中项的违规积分与严重级别。
// 注意：环境核验 flags 来自客户端 JS 自报，可被伪造、也会被共享出口分摊，
// 因此**全部不设 severe**（不再即时封禁）——由 iprisk 多证据算法决定是否封禁。
var flagScore = map[string]struct {
	score  int
	severe bool
}{
	"navigator-webdriver":  {60, false},
	"automation-global":    {60, false},
	"headless-ua":          {50, false},
	"ua-platform-mismatch": {30, false},
	"ua-ch-mismatch":       {35, false},
	"lang-tz-mismatch":     {15, false},
	"no-plugins":           {10, false},
	"env-incoherent":       {30, false},
}

// p1ScoredFlags P1 检测族的计分表（灰度上线：FP_SCORE_SHADOW 默认开着时全部只记录）。
// 这些项的共同点是"两条独立证据不一致"或"实测能力与声明矛盾"，比单一路径的自报更硬。
// 其余 P1 检测项（getter 时序 / 栈版本 / 核数基准 / 字体矛盾 / 软件光栅 / *_unsupported /
// BotD 各 signal）按规格一律只记录不计分：噪声大，先靠 P0-4 面板看假阳性率。
var p1ScoredFlags = map[string]int{
	"fpb_canvas_diverge":     15,
	"fpb_iframe_diverge":     15,
	"fpb_audio_diverge":      15,
	"fpb_gpu_claim_mismatch": 15,
	"fpb_native_fn_tamper":   15,
}

// shadowScoring 灰度开关：FP_SCORE_SHADOW 默认开（新增检测只记录不计分）。
// 规格 §0.7：累计 7 天且假阳性率 < 0.5% 后才允许置 0 接入违规积分。
func shadowScoring() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FP_SCORE_SHADOW")))
	return v != "0" && v != "false"
}

// detectFlagScore 单条 flag 的违规计分（score, severe）。
//   - 既有环境核验项：走 flagScore 表；
//   - P1 检测项：走 p1ScoredFlags，灰度期一律 0 分只记录；
//   - 未登记的 flag（*_unsupported / botd_* / 未来新增键）：0 分只记录。
//
// 注意：旧实现对未知 flag 兜底 env-incoherent 的 30 分——这会让"客户端自报的任意新键"
// 直接变成积分，故收掉（客户端可控的键名不该有计分兜底）。
func detectFlagScore(f string) (int, bool) {
	if fs, ok := flagScore[f]; ok {
		return fs.score, fs.severe
	}
	score, ok := p1ScoredFlags[f]
	if !ok || shadowScoring() {
		return 0, false
	}
	return score, false
}

// fpReportAPI POST /api/v1/fp/report：浏览器上报设备指纹，服务端登记并做连坐判定。
// 第三层环境核验：客户端 flags + 服务端 Client Hints 比对，命中即计违规分。
func (s *Server) fpReportAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeJSON(w, 200, map[string]any{"ok": true, "banned": false})
		return
	}
	var p fpPayload
	// 免登录入口必须限 body：否则攻击者可用流式大 JSON 持续占内存（Decode 完成前
	// 字段长度校验不生效，实测 20MB body 会被全量读入）
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&p); err != nil || len(p.Fingerprint) < 16 || len(p.Fingerprint) > 128 {
		writeErr(w, 400, errorString("请求体需要 {\"fp\": \"...\"}"))
		return
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	ip := clientIPFromRequest(r)
	// P4-3 ALTCHA PoW：强制开启（ALTCHA_SECRET 在位且 ALTCHA_DIFFICULTY>0）时，
	// 无有效解的裸上报 401 + 弱证据计分；未启用时行为与 PoW 之前完全一致。
	if !s.checkAltchaForReport(w, r, ip, p.Fingerprint, p.Altcha) {
		return
	}

	// ---- 第三层：服务端环境核验（不依赖客户端自觉上报）----
	flags := p.Flags
	hasFlag := func(k string) bool {
		for _, f := range flags {
			if f == k {
				return true
			}
		}
		return false
	}
	// 旧客户端（无 flags 字段，nil 切片）只发 coherent 布尔：不一致时补等价命中项。
	// 注意区分：新客户端会显式发 "flags": []，反序列化为非 nil 空切片。
	if p.Flags == nil && !p.Coherent && !hasFlag("ua-platform-mismatch") {
		flags = append(flags, "ua-platform-mismatch")
	}
	// Client Hints 比对：Sec-CH-UA-Platform 与 UA 声明的系统矛盾 = 伪造 header 的爬虫
	if ch := r.Header.Get("Sec-CH-UA-Platform"); ch != "" {
		ua := strings.ToLower(r.UserAgent())
		chl := strings.ToLower(strings.Trim(ch, `"`))
		saysWin, saysMac := strings.Contains(ua, "windows"), strings.Contains(ua, "mac os")
		saysLinux := strings.Contains(ua, "linux") && !strings.Contains(ua, "android")
		saysAndroid := strings.Contains(ua, "android")
		switch {
		case strings.Contains(chl, "windows") && !saysWin,
			strings.Contains(chl, "mac") && !saysMac,
			strings.Contains(chl, "linux") && !saysLinux && !saysAndroid,
			strings.Contains(chl, "android") && !saysAndroid:
			if !hasFlag("ua-ch-mismatch") {
				flags = append(flags, "ua-ch-mismatch")
			}
		}
	}
	// UA 直查无头标记（客户端可能跑在旧 JS 里没检出来）
	if strings.Contains(strings.ToLower(r.UserAgent()), "headless") && !hasFlag("headless-ua") {
		flags = append(flags, "headless-ua")
	}

	meta := FingerprintMeta{
		Webrtc:      p.WebRTC,
		Components:  sanitizeComponents(p.Components),
		Flags:       flags,
		CanvasPHash: sanitizePHash(p.CanvasPHash),
		Sets:        sanitizeSets(p.Sets),
		TZ:          sanitizeTZ(p.TZ),
		TZOffsetMin: p.TZOffsetMin,
		JA4:         JA4FromContext(ctx), // P3-2：TLS 模式下由连接上下文注入（纯 HTTP 为空）
		Behavior:    sanitizeBehavior(string(p.Behavior)),
		ClockSkewPPM: p.ClockSkewPPM,
	}
	out, _ := s.Guard.ReportFingerprint(ctx, ip, r.UserAgent(), p.Fingerprint, meta)
	// P6-3b GNN sidecar 异步打分（fire-and-forget，不阻塞上报路径）
	if sidecarEnabled() {
		s.gnnSidecarScore(ctx, ip, p.Fingerprint)
	}
	// 每个命中项记违规事件（积分见 flagScore）。
	// kind 按 flag 细分（env-flag:<flag>）：使不同命中项成为**独立证据**参与互证，
	// 同时让 5 分钟去重按 flag 粒度生效——否则多条 flag 会被压成同一条事件。
	// P2-3 熵值加权：违规分 × min(1, entropy_bits/40)——大众配置（低熵）只计分
	// 不硬封，罕见组合（高熵）足额计分；熵权未算（0）时系数为 1，行为不变。
	factor := s.Guard.EntropyFactor(ctx, p.Fingerprint)
	for _, f := range flags {
		score, severe := detectFlagScore(f)
		if factor < 1 && score > 0 {
			score = int(math.Round(float64(score) * factor))
		}
		s.Guard.Event(ctx, ip, "env-flag:"+f, "环境核验命中 "+f, score, severe)
	}
	writeJSON(w, 200, out)
}

// ---------- 管理端：IP 防护总览与操作 ----------

// ipGuardSummaryAPI GET /api/v1/admin/ipguard/summary。
func (s *Server) ipGuardSummaryAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeJSON(w, 200, map[string]any{"enabled": false})
		return
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	// ?hours= 只取近 N 小时的事件（0/缺省 = 不限制）
	var since time.Time
	if h := parseHours(r.URL.Query().Get("hours")); h > 0 {
		since = time.Now().Add(-time.Duration(h) * time.Hour)
	}
	events, _ := s.Guard.Store().ListIPEventsSince(ctx, 50, since)
	fps, _ := s.Guard.Store().ListFingerprints(ctx, 20)
	bans, _ := s.Guard.Store().ListBans(ctx)
	writeJSON(w, 200, map[string]any{
		"enabled":      true,
		"talkers":      s.Guard.Talkers(),
		"events":       events,
		"fingerprints": fps,
		"bans":         bans,
		"flag_stats":   s.flagStats(ctx),
		"clusters":     s.clusterOverview(ctx),
	})
}

// clusterOverview 集群概览（P4-4）：按 size 降序 Top-10（additive 字段，零影响旧前端）。
func (s *Server) clusterOverview(ctx context.Context) []ClusterDTO {
	clusters, err := s.Guard.Store().ListClusters(ctx, 10)
	if err != nil {
		return []ClusterDTO{}
	}
	return clusters
}

// flagStatsWindow flag 命中统计的观察窗口（灰度判断假阳性率的基准窗）。
const flagStatsWindow = 7 * 24 * time.Hour

// flagStats 近 7 天各检测 flag 的命中排行（Top flagStatsTop），供管理端灰度观察：
// P1/P2 新增检测先只记录不计分，靠这里的命中数判断假阳性率是否达标。
func (s *Server) flagStats(ctx context.Context) []fpstats.FlagStat {
	since := time.Now().Add(-flagStatsWindow)
	fps, err := s.Guard.Store().ListFingerprintsSince(ctx, since, 5000)
	if err != nil {
		return []fpstats.FlagStat{}
	}
	samples := make([]fpstats.FlagSample, 0, len(fps))
	for _, f := range fps {
		samples = append(samples, fpstats.FlagSample{
			FP: f.Fingerprint, Flags: f.Flags, Visits: f.Hits, LastSeen: f.LastSeen,
		})
	}
	return fpstats.AggregateFlags(samples, since, flagStatsTop)
}

// flagStatsTop 面板展示条数（Top-N 横向条形）。
const flagStatsTop = 15

// ipGuardBanAPI POST /api/v1/admin/ipguard/ban {ip, hours, reason} 手动封禁。
func (s *Server) ipGuardBanAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeErr(w, 400, errorString("IP 防护未启用"))
		return
	}
	var p struct {
		IP     string `json:"ip"`
		Hours  int    `json:"hours"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.IP == "" {
		writeErr(w, 400, errorString("请求体需要 {\"ip\": \"...\"}"))
		return
	}
	if p.Hours <= 0 {
		p.Hours = 24
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	if err := s.Guard.BanIP(ctx, p.IP, p.Reason, p.Hours); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ipGuardUnbanAPI POST /api/v1/admin/ipguard/unban {ip}。
func (s *Server) ipGuardUnbanAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeErr(w, 400, errorString("IP 防护未启用"))
		return
	}
	var p struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.IP == "" {
		writeErr(w, 400, errorString("请求体需要 {\"ip\": \"...\"}"))
		return
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	if err := s.Guard.Store().DeleteBan(ctx, p.IP); err != nil {
		writeErr(w, 500, err)
		return
	}
	s.Guard.ResetBanCache(p.IP)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ipGuardIPDetailAPI GET /api/v1/admin/ipguard/ip?ip=xxx：单 IP 下钻详情（档案/封禁/关联指纹/违规事件）。
func (s *Server) ipGuardIPDetailAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeJSON(w, 200, map[string]any{"enabled": false})
		return
	}
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	if ip == "" {
		writeErr(w, 400, errorString("缺少 ?ip= 参数"))
		return
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	store := s.Guard.Store()
	profile, _ := store.FindIPProfile(ctx, ip)
	ban, _ := store.FindBan(ctx, ip)
	fps, _ := store.ListFingerprintsByIP(ctx, ip, 20)
	events, _ := store.ListIPEventsByIP(ctx, ip, 30)
	// ?hours= 只保留近 N 小时事件
	if h := parseHours(r.URL.Query().Get("hours")); h > 0 {
		cut := time.Now().Add(-time.Duration(h) * time.Hour)
		kept := events[:0]
		for _, e := range events {
			if e.At.After(cut) {
				kept = append(kept, e)
			}
		}
		events = kept
	}
	writeJSON(w, 200, map[string]any{
		"ip":           ip,
		"profile":      profile,
		"ban":          ban,
		"fingerprints": fps,
		"events":       events,
	})
}

// parseHours 解析 ?hours= 参数为整数小时（非法/超界返回 0 = 不限制）。
func parseHours(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var h int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		h = h*10 + int(c-'0')
		if h > 24*30 {
			return 0
		}
	}
	return h
}

// registerIPGuardRoutes 挂防护端点（仅公开上报口;管理端操作端点在 admin.go 的守卫组内)。
func (s *Server) registerIPGuardRoutes(r chi.Router) {
	r.Post("/fp/report", s.fpReportAPI)
	// P4-3 ALTCHA PoW：挑战签发（前端/APP 刷成本用）与独立校验通道。
	r.Get("/altcha/challenge", s.altchaChallengeAPI)
	r.Post("/altcha/verify", s.altchaVerifyAPI)
	// P4-1 平台证明（Play Integrity / 签名降级）
	r.Get("/app/attest/challenge", s.appAttestChallengeAPI)
	r.Post("/app/attest/verify", s.appAttestVerifyAPI)
	// DevTools 检测上报（用于行为分析，不立即封禁）
	r.Post("/security/devtools-detected", s.devtoolsDetectedAPI)
}

// Store 暴露存储（管理端 handler 用）。
func (g *IPGuard) Store() GuardStore { return g.store }

// Event 暴露事件入口（登录爆破等模块上报用）。
func (g *IPGuard) Event(ctx context.Context, ip, kind, detail string, score int, severe bool) {
	if !g.enabled {
		return
	}
	g.event(ctx, ip, kind, detail, score, severe)
}

// devtoolsDetectedAPI 接收前端上报的 DevTools 检测事件（仅记录，不封禁）。
func (s *Server) devtoolsDetectedAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}

	var req struct {
		UA        string `json:"ua"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}

	ip := realIP(r)
	// 记录事件，积分为 0（仅作为行为特征，不触发封禁）
	s.Guard.Event(r.Context(), ip, "devtools-detected", "前端检测到开发者工具打开", 0, false)

	writeJSON(w, 200, map[string]bool{"ok": true})
}
