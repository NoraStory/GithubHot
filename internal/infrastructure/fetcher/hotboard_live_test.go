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

// TestNeteaseHotFetch 实测网易新闻排行榜解析（GBK HTML）。
func TestNeteaseHotFetch(t *testing.T) {
	requireLive(t)
	items, err := fetchNeteaseHot(context.Background(), 10)
	if err != nil {
		t.Fatalf("fetchNeteaseHot: %v", err)
	}
	for _, it := range items[:3] {
		t.Logf("rank=%s title=%s heat=%s url=%s", it.Meta["rank"], it.Title, it.Meta["heat"], it.URL)
	}
}

// TestTencentHotFetch 实测腾讯新闻热点榜解析（JSON 接口，过滤 TIP 运营位）。
func TestTencentHotFetch(t *testing.T) {
	requireLive(t)
	items, err := fetchTencentHot(context.Background(), 10)
	if err != nil {
		t.Fatalf("fetchTencentHot: %v", err)
	}
	for _, it := range items[:3] {
		t.Logf("rank=%s title=%s url=%s", it.Meta["rank"], it.Title, it.URL)
	}
}
