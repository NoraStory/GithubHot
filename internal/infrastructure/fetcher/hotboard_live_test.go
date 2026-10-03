package fetcher

import (
	"context"
	"os"
	"testing"
)

// 实况测试：默认跳过（打真实 API），需要时 LIVE_HOT_TESTS=1 go test -run Hot -v 手动执行。
func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LIVE_HOT_TESTS") != "1" {
		t.Skip("实况测试，设 LIVE_HOT_TESTS=1 才运行")
	}
}

// TestWeiboHotFetch 实测微博热搜解析（兼容 is_ad 数字/布尔漂移）。
func TestWeiboHotFetch(t *testing.T) {
	requireLive(t)
	items, err := fetchWeiboHot(context.Background(), 15)
	if err != nil {
		t.Fatalf("fetchWeiboHot: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("无条目")
	}
	for _, it := range items[:3] {
		t.Logf("rank=%s title=%s url=%s", it.Meta["rank"], it.Title, it.URL)
	}
}

// TestBilibiliHotFetch 实测 B 站热门解析（-352 风控时容忍失败，仅记录）。
func TestBilibiliHotFetch(t *testing.T) {
	requireLive(t)
	items, err := fetchBilibiliHot(context.Background(), 10)
	if err != nil {
		t.Logf("bilibili 当前不可用（风控/结构变更）: %v", err)
		return
	}
	for _, it := range items[:3] {
		t.Logf("rank=%s title=%s url=%s", it.Meta["rank"], it.Title, it.URL)
	}
}
