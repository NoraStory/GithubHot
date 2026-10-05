package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NoraStory/GithubHot/internal/domain/fpstats"
)

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
	Coherent    bool              `json:"coherent"` // 旧客户端兼容：UA 与 platform 一致性
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

// fpReportAPI POST /api/v1/fp/report：浏览器上报设备指纹，服务端登记并做连坐判定。
// 第三层环境核验：客户端 flags + 服务端 Client Hints 比对，命中即计违规分。
func (s *Server) fpReportAPI(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeJSON(w, 200, map[string]any{"ok": true, "banned": false})
		return
	}
	var p fpPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || len(p.Fingerprint) < 16 || len(p.Fingerprint) > 128 {
		writeErr(w, 400, errorString("请求体需要 {\"fp\": \"...\"}"))
		return
	}
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()
	ip := clientIPFromRequest(r)

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

	meta := FingerprintMeta{Webrtc: p.WebRTC, Components: sanitizeComponents(p.Components), Flags: flags}
	out, _ := s.Guard.ReportFingerprint(ctx, ip, r.UserAgent(), p.Fingerprint, meta)
	// 每个命中项记违规事件（积分见 flagScore）。
	// kind 按 flag 细分（env-flag:<flag>）：使不同命中项成为**独立证据**参与互证，
	// 同时让 5 分钟去重按 flag 粒度生效——否则多条 flag 会被压成同一条事件。
	for _, f := range flags {
		fs, ok := flagScore[f]
		if !ok {
			fs = flagScore["env-incoherent"]
		}
		s.Guard.Event(ctx, ip, "env-flag:"+f, "环境核验命中 "+f, fs.score, fs.severe)
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
	})
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

// registerIPGuardRoutes 挂防护端点（仅公开上报口；管理端操作端点在 admin.go 的守卫组内）。
func (s *Server) registerIPGuardRoutes(r chi.Router) {
	r.Post("/fp/report", s.fpReportAPI)
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
