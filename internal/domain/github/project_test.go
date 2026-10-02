package github

import (
	"math"
	"testing"
	"time"
)

var base = time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)

func TestHotnessStarGrowth(t *testing.T) {
	// 24h 前 100 星 → 现在 160 星：gained=60 → 10×log2(61) ≈ 58.8
	snaps := []Snapshot{
		{FullName: "a/b", At: base.Add(-24 * time.Hour), Stars: 100},
		{FullName: "a/b", At: base, Stars: 160},
	}
	h := Hotness(HotnessInput{Snapshots: snaps, Now: base})
	want := 10 * math.Log2(61)
	if math.Abs(h-want) > 0.1 {
		t.Fatalf("热度应约 %.1f，得到 %.1f", want, h)
	}
}

func TestHotnessNoPriorSnapshot(t *testing.T) {
	// 首次发现、无历史：gained=0，只有 trending 加成（rank 1-3 → +6）
	snaps := []Snapshot{{FullName: "a/b", At: base, Stars: 30000, TrendingRank: 2}}
	h := Hotness(HotnessInput{Snapshots: snaps, Now: base})
	if h != 6.0 {
		t.Fatalf("首次发现无增长应只有 trending 加成 6.0，得到 %.1f", h)
	}
}

func TestHotnessNovelty(t *testing.T) {
	// 首次发现 + 有增长 → novelty +5
	snaps := []Snapshot{
		{FullName: "a/b", At: base.Add(-20 * time.Hour), Stars: 10},
		{FullName: "a/b", At: base, Stars: 74},
	}
	h := Hotness(HotnessInput{Snapshots: snaps, Now: base})
	want := 10*math.Log2(65) + 5
	if math.Abs(h-want) > 0.1 {
		t.Fatalf("热度应约 %.1f，得到 %.1f", want, h)
	}
}

func TestHotnessRepeatedScrapesDoNotInflate(t *testing.T) {
	// 同一天抓 5 次，热度不变（快照差分不虚增）
	now := base
	few := []Snapshot{
		{FullName: "a/b", At: now.Add(-24 * time.Hour), Stars: 100},
		{FullName: "a/b", At: now, Stars: 160},
	}
	many := append([]Snapshot{}, few...)
	for i := 1; i <= 3; i++ {
		many = append(many, Snapshot{FullName: "a/b", At: now.Add(-time.Duration(24-i) * time.Hour), Stars: 160})
	}
	if Hotness(HotnessInput{Snapshots: few, Now: now}) != Hotness(HotnessInput{Snapshots: many, Now: now}) {
		t.Fatal("重复抓取不应虚增热度")
	}
}

func TestStarsGainedIn(t *testing.T) {
	snaps := []Snapshot{
		{FullName: "a/b", At: base.Add(-72 * time.Hour), Stars: 50},
		{FullName: "a/b", At: base.Add(-30 * time.Hour), Stars: 100},
		{FullName: "a/b", At: base, Stars: 160},
	}
	if g := StarsGainedIn(snaps, base, 24*time.Hour); g != 60 {
		t.Fatalf("24h 窗口增量应 60，得到 %d", g)
	}
}

func TestNewProjectValidation(t *testing.T) {
	if _, err := New("bad-name", "", 0, base); err == nil {
		t.Fatal("非法仓库名应报错")
	}
	p, err := New("owner/repo", "", 42, base)
	if err != nil {
		t.Fatal(err)
	}
	if p.HTMLURL != "https://github.com/owner/repo" {
		t.Fatalf("默认 URL 错误: %s", p.HTMLURL)
	}
}

func TestMergePrefersRicher(t *testing.T) {
	p, _ := New("o/r", "", 10, base)
	p.Merge(Project{Description: "更好的描述", Language: "Go", Stars: 20, TrendingRank: 3, LastSeenAt: base})
	if p.Description != "更好的描述" || p.Stars != 20 || p.TrendingRank != 3 {
		t.Fatalf("合并结果不符: %+v", p)
	}
}
