package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------- P0-4：检测 flag 命中统计（灰度观察面板的数据源） ----------

// ListFingerprintsSince 时间窗内活跃指纹（fakeGuardStore 的补充实现，见 ipguard_risk_test.go）。
func (f *fakeGuardStore) ListFingerprintsSince(_ context.Context, since time.Time, limit int) ([]FingerprintDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []FingerprintDTO{}
	for _, fp := range f.fps {
		if !fp.LastSeen.Before(since) {
			out = append(out, fp)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeGuardStore) addFingerprint(fp string, hits int, ageHours float64, flags ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fps = append(f.fps, FingerprintDTO{
		Fingerprint: fp, Flags: flags, Hits: hits,
		LastSeen: time.Now().Add(-time.Duration(ageHours * float64(time.Hour))),
	})
}

// summaryOf 调汇总接口并解出 flag_stats。
func summaryOf(t *testing.T, s *Server, url string) []map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ipGuardSummaryAPI(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("summary 应 200，实际 %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应不是 JSON: %v", err)
	}
	raw, ok := payload["flag_stats"]
	if !ok {
		t.Fatalf("summary 必须含 flag_stats 字段（P0-4 面板数据源）")
	}
	list, _ := raw.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// TestSummaryFlagStatsAggregates 汇总接口按近 7 天窗口聚合各 flag 命中排行。
func TestSummaryFlagStatsAggregates(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.addFingerprint("fp-headless-1", 3, 1, "env-flag:headless-ua", "env-flag:no-plugins")
	store.addFingerprint("fp-headless-2", 1, 2, "env-flag:headless-ua")
	store.addFingerprint("fp-botd", 9, 3, "botd_headless")
	store.addFingerprint("fp-stale", 50, 24*10, "env-flag:headless-ua") // 窗口外，应被忽略
	s := &Server{Guard: NewIPGuard(store)}

	stats := summaryOf(t, s, "/api/v1/admin/ipguard/summary")
	if len(stats) != 3 {
		t.Fatalf("应有 3 个 flag（窗口外的忽略），实际 %#v", stats)
	}
	byKey := map[string]map[string]any{}
	for _, st := range stats {
		byKey[st["key"].(string)] = st
	}
	if got := byKey["botd_headless"]; got == nil || got["hits"].(float64) != 9 {
		t.Fatalf("botd_headless 统计不符：%#v", got)
	}
	hd := byKey["env-flag:headless-ua"]
	if hd == nil || hd["hits"].(float64) != 4 || hd["affected_fps"].(float64) != 2 {
		t.Fatalf("headless-ua 应为 4 次命中 / 2 个指纹，实际 %#v", hd)
	}
	if stats[0]["key"].(string) != "botd_headless" {
		t.Fatalf("应按命中次数降序，实际首位 %v", stats[0]["key"])
	}
}

// TestSummaryFlagStatsEmpty 空库（全新部署）：字段存在且为空数组，前端据此走空态。
func TestSummaryFlagStatsEmpty(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	s := &Server{Guard: NewIPGuard(newFakeStore())}
	if stats := summaryOf(t, s, "/api/v1/admin/ipguard/summary"); len(stats) != 0 {
		t.Fatalf("空库应为空统计，实际 %#v", stats)
	}
}

// TestSummaryFlagStatsTop15 面板只展示 Top-15（避免长尾把区块撑爆）。
func TestSummaryFlagStatsTop15(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	for i := 0; i < 20; i++ {
		store.addFingerprint("fp-"+string(rune('a'+i)), i+1, 1, "flag-"+string(rune('a'+i)))
	}
	s := &Server{Guard: NewIPGuard(store)}
	if stats := summaryOf(t, s, "/api/v1/admin/ipguard/summary"); len(stats) != 15 {
		t.Fatalf("应截断为 15 条，实际 %d", len(stats))
	}
}

// TestSummaryFlagStatsDisabledGuard 防护未启用时 summary 仍返回 enabled=false（旧契约不变）。
func TestSummaryFlagStatsDisabledGuard(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.ipGuardSummaryAPI(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/ipguard/summary", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d", rec.Code)
	}
	var payload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload["enabled"] != false {
		t.Fatalf("应返回 enabled=false，实际 %#v", payload)
	}
}
