package fpstats

import (
	"testing"
	"time"
)

func sample(fp string, visits int, ageHours float64, flags ...string) FlagSample {
	return FlagSample{
		FP:       fp,
		Flags:    flags,
		Visits:   visits,
		LastSeen: time.Now().Add(-time.Duration(ageHours * float64(time.Hour))),
	}
}

func TestAggregateFlagsCountsAndOrder(t *testing.T) {
	samples := []FlagSample{
		sample("fp1", 3, 1, "headless-ua", "no-plugins"),
		sample("fp2", 1, 2, "headless-ua"),
		sample("fp3", 5, 3, "botd_headless"),
	}
	got := AggregateFlags(samples, time.Time{}, 0)
	want := []FlagStat{
		{Key: "headless-ua", Hits: 4, AffectedFPs: 2},
		{Key: "botd_headless", Hits: 5, AffectedFPs: 1}, // Hits 更高应排前
		{Key: "no-plugins", Hits: 3, AffectedFPs: 1},
	}
	// 重新按规则核对排序：Hits 降序
	if got[0].Key != "botd_headless" || got[1].Key != "headless-ua" || got[2].Key != "no-plugins" {
		t.Fatalf("排序应为 Hits 降序，实际 %#v", got)
	}
	for _, w := range want {
		var found bool
		for _, g := range got {
			if g.Key == w.Key {
				found = true
				if g.Hits != w.Hits || g.AffectedFPs != w.AffectedFPs {
					t.Fatalf("%s = %#v, want %#v", w.Key, g, w)
				}
			}
		}
		if !found {
			t.Fatalf("缺少 %s", w.Key)
		}
	}
}

func TestAggregateFlagsWindowFilter(t *testing.T) {
	samples := []FlagSample{
		sample("fresh", 1, 1, "fpb_canvas_diverge"),
		sample("stale", 1, 24*10, "fpb_canvas_diverge"),
	}
	got := AggregateFlags(samples, time.Now().Add(-7*24*time.Hour), 0)
	if len(got) != 1 || got[0].Hits != 1 || got[0].AffectedFPs != 1 {
		t.Fatalf("7 天窗只应统计新鲜样本，实际 %#v", got)
	}
}

func TestAggregateFlagsDedupeAndEmpty(t *testing.T) {
	samples := []FlagSample{
		{FP: "fp1", Visits: 2, Flags: []string{"a", "a", "", "b"}},
	}
	got := AggregateFlags(samples, time.Time{}, 0)
	if len(got) != 2 {
		t.Fatalf("重复与空 key 应去重，实际 %#v", got)
	}
	for _, g := range got {
		if g.Hits != 2 || g.AffectedFPs != 1 {
			t.Fatalf("同一指纹内重复 key 只计一次上报次数，实际 %#v", g)
		}
	}
}

func TestAggregateFlagsVisitsFloorAndLimit(t *testing.T) {
	// Visits=0（脏数据/未上报次数）按 1 计，避免统计被吞掉
	got := AggregateFlags([]FlagSample{
		{FP: "fp1", Visits: 0, Flags: []string{"x"}},
		{FP: "fp2", Visits: 9, Flags: []string{"y"}},
	}, time.Time{}, 1)
	if len(got) != 1 || got[0].Key != "y" {
		t.Fatalf("limit=1 应只返回 Hits 最高者，实际 %#v", got)
	}
	all := AggregateFlags([]FlagSample{{FP: "fp1", Visits: 0, Flags: []string{"x"}}}, time.Time{}, 0)
	if all[0].Hits != 1 {
		t.Fatalf("Visits=0 应按 1 计，实际 %#v", all)
	}
}

func TestAggregateFlagsEmptyInput(t *testing.T) {
	if got := AggregateFlags(nil, time.Time{}, 15); len(got) != 0 {
		t.Fatalf("空输入应返回空切片，实际 %#v", got)
	}
}
