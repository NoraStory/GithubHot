package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- P2-2 MinHash+LSH 组件集合关联 ----------

// seedFingerprint 注入一条带分量与稳定度历史的旧档案（轮换检测/熵权用例铺底）。
func (f *fakeGuardStore) seedFingerprint(fp string, ageHours float64, components map[string]string, compStability map[string]float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := time.Now().Add(-time.Duration(ageHours * float64(time.Hour)))
	f.fps = append(f.fps, FingerprintDTO{
		Fingerprint: fp, Components: components, CompStability: compStability,
		FirstSeen: t, LastSeen: t,
	})
}

// reportSets 上报一条带 sets 清单的指纹，返回该 IP 记录到的事件 kind 集合。
func reportSets(t *testing.T, store *fakeGuardStore, fp, ip, setsJSON string) map[string]int {
	t.Helper()
	s := &Server{Guard: NewIPGuard(store)}
	body := `{"fp":"` + fp + `","flags":[],"sets":` + setsJSON + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body))
	req.RemoteAddr = ip + ":4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("上报应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	return store.kindsOf(ip)
}

// TestMinHashLinkSameComponentSet 组件轮换（换掉 canvas 等身份分量）但字体/扩展/插件
// 清单高度重合 → LSH 同带召回 + 精确 Jaccard > 0.8 → 记 minhash 关联边，0 分不计违规。
func TestMinHashLinkSameComponentSet(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	kinds := reportSets(t, store, "fp-set-a-000000000001", "198.51.100.41",
		`{"fonts":["Arial","Consolas","Segoe UI"],"webgl_exts":["OES_texture_float"],"plugins":["Chrome PDF Viewer"]}`)
	if _, ok := kinds["fp-minhash-link"]; ok {
		t.Fatalf("首报不应有关联事件（无候选）：%v", kinds)
	}
	// 第二指纹：组件集合同质（模拟同源设备轮换身份分量）
	kinds2 := reportSets(t, store, "fp-set-b-000000000002", "198.51.100.42",
		`{"fonts":["Arial","Consolas","Segoe UI"],"webgl_exts":["OES_texture_float"],"plugins":["Chrome PDF Viewer"]}`)
	_ = kinds2
	links := store.links
	found := false
	for _, l := range links {
		if l.Kind == "minhash" {
			found = true
			if l.Weight <= minhashJaccardMin {
				t.Fatalf("关联权重应 > %.2f，实际 %.3f", minhashJaccardMin, l.Weight)
			}
		}
	}
	if !found {
		t.Fatalf("同集合指纹应产生 minhash 关联边，实际 %v", links)
	}
	for _, e := range store.events {
		if e.Kind == "fp-minhash-link" && e.Score != 0 {
			t.Fatalf("关联事件必须 0 分，实际 %d", e.Score)
		}
	}
}

// TestMinHashSkipsDifferentSets 完全不同的清单 → 不同桶或 Jaccard=0 → 不关联。
func TestMinHashSkipsDifferentSets(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	reportSets(t, store, "fp-diff-a-0000000001", "198.51.100.43",
		`{"fonts":["Arial"],"webgl_exts":["EXT_A"],"plugins":["P_A"]}`)
	reportSets(t, store, "fp-diff-b-0000000002", "198.51.100.44",
		`{"fonts":["SimSun"],"webgl_exts":["EXT_B"],"plugins":["P_B"]}`)
	for _, l := range store.links {
		if l.Kind == "minhash" {
			t.Fatalf("不同集合不应产生 minhash 关联：%#v", l)
		}
	}
}

// ---------- P2-4 换脸轮换检测 ----------

// TestRotationDetected 稳定分量（字体）在旧指纹上一直稳定，却在被 pHash 关联的
// 新指纹上突变 → 换脸实锤。灰度期（FP_SCORE_SHADOW 默认开）0 分只记录；
// 关掉灰度后 25 分。
func TestRotationDetected(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	// 旧指纹：字体清单历史稳定（稳定度 0.9），2 小时前活跃
	store.seedFingerprint("fp-old-face-00000001", 2,
		map[string]string{"fonts": "F_LIST_1", "webgl": "W1", "renderer": "R1"},
		map[string]float64{"fonts": 0.9, "webgl": 0.9, "renderer": 0.9})
	store.seedPHash("fp-old-face-00000001", "0f0f0f0f0f0f0f0f", 2)

	// 新指纹：canvas pHash 与旧指纹距离 1（同设备），但字体清单变了
	s := &Server{Guard: NewIPGuard(store)}
	body := `{"fp":"fp-new-face-00000002","flags":[],"canvas_phash":"0f0f0f0f0f0f0f0e",` +
		`"components":{"fonts":"F_LIST_2","webgl":"W1","renderer":"R1"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body))
	req.RemoteAddr = "198.51.100.45:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("上报应 200，实际 %d", rec.Code)
	}
	hit := false
	for _, e := range store.events {
		if e.Kind == "env-flag:fpb_rotation_detected" {
			hit = true
			if e.Score != 0 {
				t.Fatalf("灰度期轮换检测必须 0 分，实际 %d", e.Score)
			}
			if !strings.Contains(e.Detail, "fonts") {
				t.Fatalf("事件明细应含突变分量名，实际 %q", e.Detail)
			}
		}
	}
	if !hit {
		t.Fatalf("应命中换脸轮换检测，实际事件 %v", store.events)
	}

	// 关掉灰度 → 25 分
	store2 := newFakeStore()
	store2.seedFingerprint("fp-old-face-00000001", 2,
		map[string]string{"fonts": "F_LIST_1"},
		map[string]float64{"fonts": 0.9})
	store2.seedPHash("fp-old-face-00000001", "0f0f0f0f0f0f0f0f", 2)
	t.Setenv("FP_SCORE_SHADOW", "0")
	s2 := &Server{Guard: NewIPGuard(store2)}
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(
		`{"fp":"fp-new-face-00000002","flags":[],"canvas_phash":"0f0f0f0f0f0f0f0e",`+
			`"components":{"fonts":"F_LIST_2"}}`))
	req2.RemoteAddr = "198.51.100.46:4444"
	rec2 := httptest.NewRecorder()
	s2.fpReportAPI(rec2, req2)
	scored := false
	for _, e := range store2.events {
		if e.Kind == "env-flag:fpb_rotation_detected" {
			scored = true
			if e.Score != rotationDetectedScore {
				t.Fatalf("非灰度期应 %d 分，实际 %d", rotationDetectedScore, e.Score)
			}
		}
	}
	if !scored {
		t.Fatalf("关闭灰度后应计分")
	}
}

// TestRotationSkipsUnstableHistory 旧指纹的字体稳定度不足（无历史/从未稳定）→
// 即使取值不同也不判换脸（防新档案误伤）。
func TestRotationSkipsUnstableHistory(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.seedFingerprint("fp-unstable-0000001", 2,
		map[string]string{"fonts": "F_LIST_1"},
		map[string]float64{"fonts": 0.2}) // 稳定度低于 0.7
	store.seedPHash("fp-unstable-0000001", "0f0f0f0f0f0f0f0f", 2)
	s := &Server{Guard: NewIPGuard(store)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(
		`{"fp":"fp-rot-new-0000002","flags":[],"canvas_phash":"0f0f0f0f0f0f0f0e",`+
			`"components":{"fonts":"F_LIST_2"}}`))
	req.RemoteAddr = "198.51.100.47:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	for _, e := range store.events {
		if e.Kind == "env-flag:fpb_rotation_detected" {
			t.Fatalf("稳定度不足不应判换脸：%s", e.Detail)
		}
	}
}

// ---------- P2-3 熵值加权 ----------

// TestEntropyFactorDamping 低熵指纹（大众配置）的三层违规分按 min(1, bits/40) 衰减：
// entropy_bits=20 → 系数 0.5 → headless-ua 的 50 分记 25 分。无熵权档案的指纹不变。
func TestEntropyFactorDamping(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.seedFingerprint("fp-lowentropy-0001", 1, map[string]string{"canvas": "c1"}, nil)
	if err := store.UpdateEntropyBits(context.Background(), "fp-lowentropy-0001", 20); err != nil {
		t.Fatal(err)
	}
	s := &Server{Guard: NewIPGuard(store)}
	body := `{"fp":"fp-lowentropy-0001","flags":["headless-ua"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body))
	req.RemoteAddr = "198.51.100.48:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	damped, undamped := false, false
	for _, e := range store.events {
		if e.Kind == "env-flag:headless-ua" {
			if e.Score == 25 {
				damped = true
			}
		}
	}
	if !damped {
		t.Fatalf("低熵指纹应衰减到 25 分，实际事件 %v", store.events)
	}
	// 对照：无熵权的指纹足额计分
	body2 := `{"fp":"fp-noentropy-000002","flags":["headless-ua"]}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body2))
	req2.RemoteAddr = "198.51.100.49:4444"
	rec2 := httptest.NewRecorder()
	s.fpReportAPI(rec2, req2)
	for _, e := range store.events {
		if e.Kind == "env-flag:headless-ua" && e.IP == "198.51.100.49" && e.Score == 50 {
			undamped = true
		}
	}
	if !undamped {
		t.Fatalf("无熵权档案应保持 50 分足额")
	}
}

// TestRefreshEntropyBits 每日刷新：全同值的分量权重为 0（无区分度），少数派的值权重高。
func TestRefreshEntropyBits(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	// canvas 三条同值 → w=0；fonts 两条 rare-x（各 -log2(2/3)≈0.58）、一条 unique-y（-log2(1/3)≈1.58）
	store.seedFingerprint("fp-ent-common-001", 1, map[string]string{"canvas": "same", "fonts": "rare-x"}, nil)
	store.seedFingerprint("fp-ent-common-002", 1, map[string]string{"canvas": "same", "fonts": "rare-x"}, nil)
	store.seedFingerprint("fp-ent-rare-0003", 1, map[string]string{"canvas": "same", "fonts": "unique-y"}, nil)
	g := NewIPGuard(store)
	if err := g.RefreshEntropyBits(context.Background()); err != nil {
		t.Fatal(err)
	}
	r1, _ := store.FindFingerprint(context.Background(), "fp-ent-common-001")
	r3, _ := store.FindFingerprint(context.Background(), "fp-ent-rare-0003")
	if r1 == nil || r3 == nil {
		t.Fatal("档案缺失")
	}
	if r1.EntropyBits < 0.57 || r1.EntropyBits > 0.59 {
		t.Fatalf("少数派值熵权应为 -log2(2/3)≈0.58，实际 %.2f", r1.EntropyBits)
	}
	if r3.EntropyBits < 1.57 || r3.EntropyBits > 1.59 {
		t.Fatalf("唯一值熵权应为 -log2(1/3)≈1.58，实际 %.2f", r3.EntropyBits)
	}
}

// ---------- 两笔小账：/healthz 存活探针不随封禁 404 ----------

// TestHealthzAliveWhenBanned 封禁 IP 的 /healthz 必须仍 200（外部监控的存活判断
// 不能把"已封禁"误报成"站点宕机"）；其余路径维持全站 404。
func TestHealthzAliveWhenBanned(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	g := NewIPGuard(store)
	ip := "198.51.100.50"
	if err := store.UpsertBan(context.Background(), ip, 1, 1, "测试封禁", time.Hour); err != nil {
		t.Fatal(err)
	}
	// healthz：放行
	nextHit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextHit = true })
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = ip + ":1000"
	rec := httptest.NewRecorder()
	g.Middleware(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !nextHit {
		t.Fatalf("封禁 IP 的 /healthz 应放行 200，实际 %d nextHit=%v", rec.Code, nextHit)
	}
	// 其他路径：仍 404
	nextHit = false
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/hot", nil)
	req2.RemoteAddr = ip + ":1000"
	rec2 := httptest.NewRecorder()
	g.Middleware(next).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound || nextHit {
		t.Fatalf("封禁 IP 的普通路径应 404 且不透传，实际 %d nextHit=%v", rec2.Code, nextHit)
	}
}
