package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- P2-1 pHash 同源关联 ----------

func (f *fakeGuardStore) ListPHashCandidates(_ context.Context, since time.Time, limit int) ([]PHashRowDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []PHashRowDTO{}
	for _, p := range f.phashes {
		if !p.LastSeen.Before(since) {
			out = append(out, p)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeGuardStore) UpsertFPLink(_ context.Context, src, dst, kind string, weight float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.links {
		if f.links[i].Src == src && f.links[i].Dst == dst && f.links[i].Kind == kind {
			f.links[i].Weight = weight
			f.links[i].LastSeen = time.Now()
			return nil
		}
	}
	f.links = append(f.links, FPLinkDTO{Src: src, Dst: dst, Kind: kind, Weight: weight, FirstSeen: time.Now(), LastSeen: time.Now()})
	return nil
}

func (f *fakeGuardStore) ListFPLinks(_ context.Context, fp string, limit int) ([]FPLinkDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []FPLinkDTO{}
	for _, l := range f.links {
		if l.Src == fp || l.Dst == fp {
			out = append(out, l)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeGuardStore) seedPHash(fp, phash string, ageHours float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.phashes = append(f.phashes, PHashRowDTO{
		Fingerprint: fp, PHash: phash,
		LastSeen: time.Now().Add(-time.Duration(ageHours * float64(time.Hour))),
	})
}

func (f *fakeGuardStore) linkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.links)
}

// reportWithPHash 上报一条带感知哈希的指纹，返回记录的事件 kind 集合。
func reportWithPHash(t *testing.T, store *fakeGuardStore, fp, phash string) map[string]int {
	t.Helper()
	s := &Server{Guard: NewIPGuard(store)}
	body := `{"fp":"` + fp + `","flags":[],"canvas_phash":"` + phash + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fp/report", strings.NewReader(body))
	req.RemoteAddr = "198.51.100.30:4444"
	rec := httptest.NewRecorder()
	s.fpReportAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("上报应 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	return store.kindsOf("198.51.100.30")
}

// TestPHashLinkSameDeviceNewHash 驱动更新/抗指纹微扰导致精确哈希变了，但 pHash 距离近
// → 应记同源关联边且不计分（相似本身不是违规）。
func TestPHashLinkSameDeviceNewHash(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.seedPHash("old-device-fp-0001", "0f0f0f0f0f0f0f0f", 1)
	kinds := reportWithPHash(t, store, "new-device-fp-0002", "0f0f0f0f0f0f0f0e") // 距离 1

	if store.linkCount() != 1 {
		t.Fatalf("应写入 1 条同源关联边，实际 %d", store.linkCount())
	}
	link := store.links[0]
	if link.Src != "new-device-fp-0002" || link.Dst != "old-device-fp-0001" || link.Kind != "phash" {
		t.Fatalf("关联边内容不符：%#v", link)
	}
	if link.Weight <= 0.9 {
		t.Fatalf("距离 1 的权重应接近 1，实际 %.3f", link.Weight)
	}
	if _, ok := kinds["fp-phash-link"]; !ok {
		t.Fatalf("应记 0 分观察事件，实际 %v", kinds)
	}
	// 关键：不计分 → 不封禁
	if ban := store.banOf("198.51.100.30"); ban != nil {
		t.Fatalf("同源关联不应封禁：%s", ban.Reason)
	}
	for _, e := range store.events {
		if e.Kind == "fp-phash-link" && e.Score != 0 {
			t.Fatalf("关联事件必须 0 分，实际 %d", e.Score)
		}
	}
}

// TestPHashLinkSkipsDifferentDevice 距离过大（不同设备）→ 不关联。
func TestPHashLinkSkipsDifferentDevice(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.seedPHash("other-device-fp-001", "ffffffffffffffff", 1)
	kinds := reportWithPHash(t, store, "my-device-fp-002", "0000000000000000") // 距离 64
	if store.linkCount() != 0 {
		t.Fatalf("不同设备不应关联，实际 %d", store.linkCount())
	}
	if kinds["fp-phash-link"] != 0 {
		t.Fatalf("不应记关联事件，实际 %v", kinds)
	}
}

// TestPHashLinkIgnoresSelfAndInvalid 自身上报不产生自环；非法/缺失哈希不关联。
func TestPHashLinkIgnoresSelfAndInvalid(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.seedPHash("same-fp-0000000001", "0f0f0f0f0f0f0f0f", 1)
	reportWithPHash(t, store, "same-fp-0000000001", "0f0f0f0f0f0f0f0f")
	if store.linkCount() != 0 {
		t.Fatalf("自身不应产生关联边，实际 %d", store.linkCount())
	}
	reportWithPHash(t, store, "bad-hash-fp-00002", "zzzz")
	if store.linkCount() != 0 {
		t.Fatalf("非法哈希不应关联，实际 %d", store.linkCount())
	}
	reportWithPHash(t, store, "no-hash-fp-000003", "")
	if store.linkCount() != 0 {
		t.Fatalf("缺失哈希不应关联，实际 %d", store.linkCount())
	}
}

// TestPHashLinkWindowAndIdempotence 窗口外候选不关联；重复上报同一对指纹只保留一条边（刷新权重）。
func TestPHashLinkWindowAndIdempotence(t *testing.T) {
	t.Setenv("IP_GUARD_ENABLED", "1")
	store := newFakeStore()
	store.seedPHash("stale-device-fp-01", "0f0f0f0f0f0f0f0e", 24*40) // 40 天前，超 30 天窗口
	reportWithPHash(t, store, "fresh-device-fp-02", "0f0f0f0f0f0f0f0f")
	if store.linkCount() != 0 {
		t.Fatalf("窗口外候选不应关联，实际 %d", store.linkCount())
	}

	store2 := newFakeStore()
	store2.seedPHash("alive-device-fp-01", "0f0f0f0f0f0f0f0e", 1)
	reportWithPHash(t, store2, "again-device-fp-02", "0f0f0f0f0f0f0f0f")
	reportWithPHash(t, store2, "again-device-fp-02", "0f0f0f0f0f0f0f0f")
	if store2.linkCount() != 1 {
		t.Fatalf("重复上报应只保留一条边，实际 %d", store2.linkCount())
	}
}

// TestSanitizePHash 上报字段清洗：非 hex16 一律丢弃。
func TestSanitizePHash(t *testing.T) {
	cases := map[string]string{
		"0f0f0f0f0f0f0f0f": "0f0f0f0f0f0f0f0f",
		"0F0F0F0F0F0F0F0F": "0f0f0f0f0f0f0f0f",
		" 0f0f0f0f0f0f0f0f ": "0f0f0f0f0f0f0f0f",
		"short":            "",
		"zzzzzzzzzzzzzzzz": "",
		"0f0f0f0f0f0f0f0f0": "",
		"":                 "",
	}
	for in, want := range cases {
		if got := sanitizePHash(in); got != want {
			t.Fatalf("sanitizePHash(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSanitizeMinHash 上报字段清洗：只允许 hex，长度受限。
func TestSanitizeMinHash(t *testing.T) {
	if got := sanitizeMinHash("AbCd12"); got != "abcd12" {
		t.Fatalf("应小写化，实际 %q", got)
	}
	if got := sanitizeMinHash("xyz"); got != "" {
		t.Fatalf("非 hex 应丢弃，实际 %q", got)
	}
	if got := sanitizeMinHash(strings.Repeat("a", 300)); got != "" {
		t.Fatalf("超长应丢弃，实际长度 %d", len(got))
	}
}
