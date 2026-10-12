package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClassifyRate 分类器表驱动：三类别边界（视频流不能被 .mp4 扩展名降级）。
func TestClassifyRate(t *testing.T) {
	cases := []struct {
		path string
		want rateClass
	}{
		{"/", rateClassMain},
		{"/album", rateClassMain},
		{"/story/abc", rateClassMain},
		{"/api/v1/hot", rateClassMain},
		{"/api/v1/fp/report", rateClassMain},
		{"/video/x/1.mp4", rateClassMain},         // 代理流：前缀判定先于扩展名
		{"/video/y/y12.mp4", rateClassMain},       //
		{"/api/v1/admin/summary", rateClassAdmin}, //
		{"/api/v1/admin/ipguard/summary", rateClassAdmin},
		{"/assets/index-CE8qa5l-.js", rateClassStatic},
		{"/anzhiyu/css/index.css", rateClassStatic},
		{"/anzhiyu/fonts/lxgw.woff2", rateClassStatic},
		{"/img/background/a.png", rateClassStatic},
		{"/favicon.ico", rateClassStatic},
		{"/llms.txt", rateClassStatic},
		{"/feed/rss.xml", rateClassStatic},
		{"/anzhiyu/js/index_media.js", rateClassStatic},
	}
	for _, c := range cases {
		if got := classifyRate(c.path); got != c.want {
			t.Errorf("classifyRate(%q) = %d, want %d", c.path, got, c.want)
		}
	}
}

// newRateTestGuard 中间件级测试工装：固定公网客户端 IP 的 200 兜底 handler。
func newRateTestGuard(t *testing.T) (*IPGuard, *fakeGuardStore, func(path string) int) {
	t.Helper()
	store := newFakeStore()
	g := NewIPGuard(store)
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	const clientIP = "203.0.113.77"
	serve := func(p string) int {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.RemoteAddr = clientIP + ":44300"
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) Chrome/126.0.0.0")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	return g, store, serve
}

// TestRefreshBurstNotFlagged 模拟真实刷新风暴（本轮修复的核心回归）：
// 3 次连续刷新，每次 40 个静态资源（/anzhiyu 字体/CSS/JS + /img）+ 6 个价值请求
// （API + 导航）。修复前 138 req/min 直接记 rate +30 并逼近软封锁；修复后静态
// 资源分流，主窗口仅 18/min——零事件、不封、不软封锁。
func TestRefreshBurstNotFlagged(t *testing.T) {
	g, store, serve := newRateTestGuard(t)
	staticPaths := []string{
		"/anzhiyu/css/index.css", "/anzhiyu/js/anzhiyu-shim.js",
		"/anzhiyu/fonts/lxgw.woff2", "/img/comment_bg.png", "/assets/index-CE8qa5l-.js",
	}
	for round := 0; round < 3; round++ {
		for i := 0; i < 40; i++ {
			serve(staticPaths[i%len(staticPaths)])
		}
		for _, p := range []string{"/api/v1/hot", "/api/v1/hot/news", "/api/v1/hot/domestic",
			"/api/v1/digests?pageSize=50", "/api/v1/site/config", "/"} {
			serve(p)
		}
	}
	if len(store.events) != 0 {
		t.Fatalf("刷新风暴不应产生任何违规事件，实际 %v", store.events)
	}
	if b, _ := store.FindBan(context.Background(), "203.0.113.77"); b != nil {
		t.Fatal("刷新风暴不应触发封禁")
	}
	if g.softBlocked("203.0.113.77", time.Now()) {
		t.Fatal("刷新风暴不应进入软封锁账本")
	}
}

// TestAssetFloodTripwire 静态资源洪水绊线：≥600/min 记 asset-flood +30 弱类
// （单类不封，面板可见可互证）。
func TestAssetFloodTripwire(t *testing.T) {
	_, store, serve := newRateTestGuard(t)
	for i := 0; i < 601; i++ {
		serve("/anzhiyu/css/index.css")
	}
	kinds := store.kindsOf("203.0.113.77")
	if kinds["asset-flood"] != 1 {
		t.Fatalf("应记 1 条 asset-flood，实际 %v", kinds)
	}
	if b, _ := store.FindBan(context.Background(), "203.0.113.77"); b != nil {
		t.Fatal("asset-flood 弱类单证据不应封禁")
	}
}

// TestAPIRateStillFires 三档速率线改口径后判定能力保留：151 次 API 记 rate；
// 601 次触发 traffic-attack 即时封。
func TestAPIRateStillFires(t *testing.T) {
	_, store, serve := newRateTestGuard(t)
	for i := 0; i < 151; i++ {
		serve("/api/v1/hot")
	}
	if store.kindsOf("203.0.113.77")["rate"] != 1 {
		t.Fatal("151 次 API/min 应记 rate 事件")
	}
	if b, _ := store.FindBan(context.Background(), "203.0.113.77"); b != nil {
		t.Fatal("151/min 不应封禁")
	}
	for i := 0; i < 450; i++ {
		serve("/api/v1/hot")
	}
	if b, _ := store.FindBan(context.Background(), "203.0.113.77"); b == nil {
		t.Fatal("601 次 API/min 应触发 traffic-attack 即时封")
	}
}

// TestAdminPollingSeparateWindow 管理端独立窗口：控制台正常轮询（30/min）
// 零事件；枚举级高频（201/min）才触发 admin-probe，且不污染主速率窗口。
func TestAdminPollingSeparateWindow(t *testing.T) {
	_, store, serve := newRateTestGuard(t)
	for i := 0; i < 40; i++ {
		serve("/api/v1/admin/system/stats")
	}
	if len(store.events) != 0 {
		t.Fatalf("40 次/min 的控制台轮询不应产生任何事件，实际 %v", store.events)
	}
	for i := 0; i < 161; i++ {
		serve("/api/v1/admin/system/stats")
	}
	if store.kindsOf("203.0.113.77")["admin-probe"] != 1 {
		t.Fatalf("201 次/min 管理端访问应记 admin-probe，实际 %v", store.kindsOf("203.0.113.77"))
	}
	if store.kindsOf("203.0.113.77")["rate"] != 0 {
		t.Fatal("管理端流量不应计入主速率窗口")
	}
}
