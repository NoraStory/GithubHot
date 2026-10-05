package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------- P1-5：检测 flag 计分接入 + 灰度开关 ----------

// reportFlags 调 fp/report 上报一组 flag，返回记录下来的事件（kind → score）。
func reportFlags(t *testing.T, store *fakeGuardStore, flags []string) map[string]int {
	t.Helper()
	s := &Server{Guard: NewIPGuard(store)}
	body, _ := json.Marshal(map[string]any{
		"fp":    "fp00000000000000000000000000p1",
		"flags": flags,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", bytes.NewReader(body))
	req.RemoteAddr = "198.51.100.20:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("上报应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	out := map[string]int{}
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, e := range store.events {
		if e.IP == "198.51.100.20" {
			out[e.Kind] = e.Score
		}
	}
	return out
}

// TestP1ShadowScoringDefault P1 检测项默认（FP_SCORE_SHADOW=1）只记录不计分。
func TestP1ShadowScoringDefault(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("FP_SCORE_SHADOW", "")
	store := newFakeStore()
	got := reportFlags(t, store, []string{
		"fpb_canvas_diverge", "fpb_iframe_diverge", "fpb_audio_diverge",
		"fpb_gpu_claim_mismatch", "fpb_native_fn_tamper",
	})
	for _, k := range []string{
		"env-flag:fpb_canvas_diverge", "env-flag:fpb_iframe_diverge", "env-flag:fpb_audio_diverge",
		"env-flag:fpb_gpu_claim_mismatch", "env-flag:fpb_native_fn_tamper",
	} {
		if _, ok := got[k]; !ok {
			t.Fatalf("应记录事件 %s，实际 %v", k, got)
		}
		if got[k] != 0 {
			t.Fatalf("灰度期 %s 应为 0 分，实际 %d", k, got[k])
		}
	}
	if len(store.bans) != 0 {
		t.Fatalf("灰度期不应产生封禁")
	}
}

// TestP1ScoringWhenShadowOff 关掉灰度开关后 P1 计分项按 +15 计分。
func TestP1ScoringWhenShadowOff(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("FP_SCORE_SHADOW", "0")
	store := newFakeStore()
	got := reportFlags(t, store, []string{"fpb_canvas_diverge", "fpb_native_fn_tamper"})
	if got["env-flag:fpb_canvas_diverge"] != 15 || got["env-flag:fpb_native_fn_tamper"] != 15 {
		t.Fatalf("应为 15 分，实际 %v", got)
	}
	// 两条弱证据各封顶 40，共 80 < 100 → 单靠两条新检测不应封禁
	if len(store.bans) != 0 {
		t.Fatalf("两条 15 分新检测不应封禁，实际 %v", store.bans)
	}
}

// TestP1RecordOnlyFlagsNeverScore 规格列为"仅记录"的 P1 项即使关掉灰度也不计分。
func TestP1RecordOnlyFlagsNeverScore(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("FP_SCORE_SHADOW", "0")
	store := newFakeStore()
	got := reportFlags(t, store, []string{
		"fpb_getter_timing:navigator.webdriver", "fpb_getter_timing:screen.colorDepth",
		"fpb_stack_version_mismatch", "fpb_cores_claim_mismatch", "fpb_font_claim_mismatch",
		"fpb_gpu_software", "fpb_canvas_check_unsupported", "fpb_font_check_unsupported",
		"botd_headless", "botd_web_driver_1",
	})
	for k, v := range got {
		if v != 0 {
			t.Fatalf("%s 应只记录不计分，实际 %d", k, v)
		}
	}
	if len(got) < 10 {
		t.Fatalf("这些 flag 都应被记录，实际 %v", got)
	}
	if len(store.bans) != 0 {
		t.Fatalf("只记录项不应封禁")
	}
}

// TestUnknownFlagNoScoreRegression 未知 flag 不再兜底 30 分（旧实现会把客户端自报的新键变成积分）。
func TestUnknownFlagNoScoreRegression(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("FP_SCORE_SHADOW", "0") // 即使灰度关闭，未知键也不该计分
	store := newFakeStore()
	got := reportFlags(t, store, []string{"attacker-chosen-key", "u_choose_this_2025"})
	for k, v := range got {
		if v != 0 {
			t.Fatalf("未知 flag %s 不应计分，实际 %d", k, v)
		}
	}
}

// TestExistingEnvFlagsStillScore 既有环境核验项的计分不受 P1 改动影响。
func TestExistingEnvFlagsStillScore(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("FP_SCORE_SHADOW", "0")
	store := newFakeStore()
	got := reportFlags(t, store, []string{"headless-ua", "navigator-webdriver", "lang-tz-mismatch"})
	if got["env-flag:headless-ua"] != 50 {
		t.Fatalf("headless-ua 应 50 分，实际 %v", got)
	}
	if got["env-flag:navigator-webdriver"] != 60 {
		t.Fatalf("navigator-webdriver 应 60 分，实际 %v", got)
	}
	if got["env-flag:lang-tz-mismatch"] != 15 {
		t.Fatalf("lang-tz-mismatch 应 15 分，实际 %v", got)
	}
}

// TestShadowSwitchParsing 灰度开关解析：默认开，显式 0/false 关。
func TestShadowSwitchParsing(t *testing.T) {
	cases := map[string]bool{"": true, "1": true, "true": true, "yes": true, "0": false, "false": false, "FALSE": false}
	for v, wantShadow := range cases {
		t.Setenv("FP_SCORE_SHADOW", v)
		if got := shadowScoring(); got != wantShadow {
			t.Fatalf("FP_SCORE_SHADOW=%q → shadow=%v，期望 %v", v, got, wantShadow)
		}
	}
}

// TestGetterTimingPerPropertyKinds getter 时序按属性细分 kind（每条独立证据）。
func TestGetterTimingPerPropertyKinds(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	got := reportFlags(t, store, []string{
		"fpb_getter_timing:navigator.webdriver", "fpb_getter_timing:screen.colorDepth",
	})
	if _, ok := got["env-flag:fpb_getter_timing:navigator.webdriver"]; !ok {
		t.Fatalf("应按属性细分 kind，实际 %v", got)
	}
	if _, ok := got["env-flag:fpb_getter_timing:screen.colorDepth"]; !ok {
		t.Fatalf("不同属性应是独立事件，实际 %v", got)
	}
}
