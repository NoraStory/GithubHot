package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---------- 指纹上报（公开端点，第二层入口） ----------

type fpPayload struct {
	Fingerprint string   `json:"fp"`
	WebRTC      []string `json:"webrtc"`
	Canvas      string   `json:"canvas"`
	WebGL       string   `json:"webgl"`
	Renderer    string   `json:"renderer"`
	Screen      string   `json:"screen"`
	Coherent    bool     `json:"coherent"` // 客户端自算的 UA/platform 一致性
}

// fpReportAPI POST /api/v1/fp/report：浏览器上报设备指纹，服务端登记并做连坐判定。
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
	out, _ := s.Guard.ReportFingerprint(ctx, ip, r.UserAgent(), p.Fingerprint, p.WebRTC)
	// 环境自洽性差的指纹：作弊/改机工具常留此类矛盾（第三层·环境核验）
	if !p.Coherent {
		s.Guard.Event(ctx, ip, "env-incoherent", "UA 与 platform 不一致", 30, false)
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
	})
}

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
