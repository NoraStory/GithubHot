package story

import (
	"math"
	"testing"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/source"
)

func fixedNow() time.Time { return time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC) }

func member(id string, publishedAgo time.Duration, now time.Time) Member {
	return Member{
		ItemID:      id,
		SourceID:    "src-" + id,
		Domain:      "example.com",
		PublishedAt: now.Add(-publishedAgo),
		TitleZh:     "测试标题" + id,
	}
}

func TestHotnessIndependentSources(t *testing.T) {
	now := fixedNow()
	// 同一事件：两家 T1 媒体同时报道
	m1 := member("a", 1*time.Hour, now)
	m2 := member("b", 1*time.Hour, now)

	h := Hotness(HotnessInput{Members: []Member{m1, m2}, Tiers: map[string]source.Tier{
		"src-a": source.TierFirstParty, "src-b": source.TierFirstParty,
	}, Now: now})
	// 两个独立来源 ×1.0 ×衰减(≈0.97) ×10 ≈ 19.4
	if h < 18 || h > 20 {
		t.Fatalf("两个 T1 来源的热度应约 19.4，得到 %.1f", h)
	}

	// 同一家媒体发两篇（同 sourceID + 同域名）只算一个独立来源
	dup := m1
	dup.ItemID = "a2"
	h2 := Hotness(HotnessInput{Members: []Member{m1, dup}, Tiers: map[string]source.Tier{
		"src-a": source.TierFirstParty,
	}, Now: now})
	if math.Abs(h2-9.7) > 0.5 {
		t.Fatalf("同一来源两篇只应算一次（≈9.7），得到 %.1f", h2)
	}
}

func TestHotnessDecayAndWindow(t *testing.T) {
	now := fixedNow()
	// 24 小时前的报道衰减一半
	old24 := member("old", 24*time.Hour, now)
	h24 := Hotness(HotnessInput{Members: []Member{old24}, Tiers: map[string]source.Tier{
		"src-old": source.TierFirstParty,
	}, Now: now})
	if math.Abs(h24-5.0) > 0.3 {
		t.Fatalf("24h 前的 T1 来源应衰减为 5.0，得到 %.1f", h24)
	}

	// 48h 窗口之外：热度保底（不为 0，但不随成员数增长）
	ancient := member("ancient", 72*time.Hour, now)
	hAnc := Hotness(HotnessInput{Members: []Member{ancient}, Tiers: map[string]source.Tier{
		"src-ancient": source.TierFirstParty,
	}, Now: now})
	if hAnc <= 0 || hAnc > 2 {
		t.Fatalf("72h 前的事件应只剩保底热度（0-2），得到 %.1f", hAnc)
	}
}

func TestHotnessFusionBoost(t *testing.T) {
	now := fixedNow()
	m := member("x", 0, now)
	tiers := map[string]source.Tier{"src-x": source.TierFirstParty}

	base := Hotness(HotnessInput{Members: []Member{m}, Tiers: tiers, Now: now})
	fused := Hotness(HotnessInput{Members: []Member{m}, Tiers: tiers, HasFusionLink: true, Now: now})
	if math.Abs(fused-base*FusionBoost) > 0.01 {
		t.Fatalf("融合加成应为 ×%.2f：base=%.1f fused=%.1f", FusionBoost, base, fused)
	}
}

func TestHotnessTierWeights(t *testing.T) {
	now := fixedNow()
	m := member("t", 0, now)
	h1 := Hotness(HotnessInput{Members: []Member{m}, Tiers: map[string]source.Tier{"src-t": source.TierFirstParty}, Now: now})
	h2 := Hotness(HotnessInput{Members: []Member{m}, Tiers: map[string]source.Tier{"src-t": source.TierMedia}, Now: now})
	if math.Abs(h1-10) > 0.01 || math.Abs(h2-6) > 0.01 {
		t.Fatalf("T1 应=10 T2 应=6：得到 %.1f / %.1f", h1, h2)
	}
}

func TestStoryIndependentSourceCount(t *testing.T) {
	s, err := NewNews("s1", Member{ItemID: "1", SourceID: "src1", Domain: "a.com"}, fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	s.Members = append(s.Members,
		Member{ItemID: "2", SourceID: "src1", Domain: "a.com"}, // 同信源同域
		Member{ItemID: "3", SourceID: "src2", Domain: "a.com"}, // 同域不同信源
		Member{ItemID: "4", SourceID: "src2", Domain: "b.com"}, // 不同信源不同域
	)
	if got := s.IndependentSourceCount(); got != 3 {
		t.Fatalf("独立来源数应为 3（同信源+同域各去重），得到 %d", got)
	}
}

func TestRisingAndIsNew(t *testing.T) {
	now := fixedNow()
	if !Rising(12.0, 10.0) {
		t.Fatal("12 vs 10 应判定为上升（+20%）")
	}
	if Rising(10.5, 10.0) {
		t.Fatal("10.5 vs 10 不应判定为上升（+5%）")
	}
	if !IsNew(now.Add(-11*time.Hour), now) {
		t.Fatal("11h 前首次发现应为新")
	}
	if IsNew(now.Add(-13*time.Hour), now) {
		t.Fatal("13h 前首次发现不应为新")
	}
}
