package shared

import (
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://Example.com/path/?utm_source=x&utm_campaign=y&id=3", "https://example.com/path?id=3"},
		{"https://example.com/path/#section", "https://example.com/path"},
		{"http://example.com:80/a", "http://example.com/a"},
		{"https://example.com:443/a/", "https://example.com/a"},
	}
	for _, c := range cases {
		got, err := NormalizeURL(c.raw)
		if err != nil {
			t.Fatalf("NormalizeURL(%q) 报错: %v", c.raw, err)
		}
		if got != c.want {
			t.Fatalf("NormalizeURL(%q) = %q，期望 %q", c.raw, got, c.want)
		}
	}
	if _, err := NormalizeURL("not a url"); err == nil {
		t.Fatal("非法 URL 应报错")
	}
}

func TestDedupeKeyStable(t *testing.T) {
	a, _ := DedupeKey("https://example.com/post?utm_source=rss")
	b, _ := DedupeKey("https://EXAMPLE.com/post/")
	if a != b {
		t.Fatalf("追踪参数与大小写不应影响判重: %s vs %s", a, b)
	}
}

func TestFixedClock(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := FixedClock{T: at}
	if c.Now() != at {
		t.Fatal("FixedClock 应返回固定时间")
	}
}
