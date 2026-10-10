package github

import (
	"math"
	"testing"
	"time"
)

// hotSnaps 构造固定星标增量的快照序列：base 在 25h 前，latest 在现在。
func hotSnaps(base, latest int, now time.Time) []Snapshot {
	return []Snapshot{
		{Stars: base, At: now.Add(-25 * time.Hour)},
		{Stars: latest, At: now},
	}
}

// TestHotnessLogScale 对数刻度：增量 1000→2000 的热度必须显著高于 0→1000
// 的十倍情形（头部增量被压制，长尾增量有存在感）。
func TestHotnessLogScale(t *testing.T) {
	now := time.Now()
	head := Hotness(HotnessInput{Snapshots: hotSnaps(9000, 10000, now), Now: now})
	tail := Hotness(HotnessInput{Snapshots: hotSnaps(0, 1000, now), Now: now})
	if head >= tail*3 {
		t.Fatalf("对数刻度失效：头部热度 %.1f 不应达到长尾 %.1f 的 3 倍", head, tail)
	}
	if tail < 1 {
		t.Fatalf("1000 star 增量应有实质热度，实际 %.1f", tail)
	}
}

// TestHotnessResonance 多源共振（算法改进 C 批）：k 单调不减、k=0 不变、
// k≥5 封顶、乘数与故事侧融合加成（×1.25）同数量级。
func TestHotnessResonance(t *testing.T) {
	now := time.Now()
	snaps := hotSnaps(0, 500, now)
	base := Hotness(HotnessInput{Snapshots: snaps, Now: now})

	if got := Hotness(HotnessInput{Snapshots: snaps, Now: now, Resonance: 0}); got != base {
		t.Fatalf("Resonance=0 应与基线一致，%.1f vs %.1f", got, base)
	}
	prev := base
	for k := 1; k <= 6; k++ {
		got := Hotness(HotnessInput{Snapshots: snaps, Now: now, Resonance: k})
		if got < prev {
			t.Fatalf("Resonance=%d 应不低于 %d 家（%.1f < %.1f）", k, k-1, got, prev)
		}
		prev = got
	}
	// 封顶：6 家与 5 家相同（×1.4）
	if got := Hotness(HotnessInput{Snapshots: snaps, Now: now, Resonance: 6}); math.Abs(got-prev) > 0.05 {
		t.Fatalf("Resonance 封顶失效：6 家 %.1f != 5 家 %.1f", got, prev)
	}
	// 数量级：单家 ×1.08
	want := math.Round(base*1.08*10) / 10
	if got := Hotness(HotnessInput{Snapshots: snaps, Now: now, Resonance: 1}); math.Abs(got-want) > 0.05 {
		t.Fatalf("单家共振应 ×1.08（期望 %.1f），实际 %.1f", want, got)
	}
}

// TestHotnessZeroGrowth 零增长项目：无 trending 无新爆 → 热度 0，不赖榜。
func TestHotnessZeroGrowth(t *testing.T) {
	now := time.Now()
	snaps := []Snapshot{{Stars: 5000, At: now.Add(-72 * time.Hour)}, {Stars: 5000, At: now}}
	if h := Hotness(HotnessInput{Snapshots: snaps, Now: now}); h != 0 {
		t.Fatalf("零增长老项目热度应为 0（窗口自清理），实际 %.1f", h)
	}
}
