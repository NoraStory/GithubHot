package iprisk

import (
	"testing"
	"time"
)

// replayFixture 合成事件流：覆盖召回 / 滞后 / 误报 / 软封锁 / 零信号 / 未召回六种形态。
//   - A：强证据两种互证 → 模型在真实封禁前触发（召回）
//   - B：单强证据 70 分 → 不封但达软封锁线（软封锁误伤代理）
//   - C：两种弱证据 30+30 → 不封但达软封锁线
//   - E：强证据互证但从未被封（真实世界 = 应封未封）→ 误报代理
//   - F：只有弱证据单类但真实被封（他因封禁）→ 未召回
//   - G：仅 0 分事件 → 无任何信号
func replayFixture(now time.Time) ([]ReplayEvent, []ReplayBan) {
	m := func(min float64) time.Time { return now.Add(time.Duration(min * float64(time.Minute))) }
	events := []ReplayEvent{
		{IP: "A", Kind: "admin-brute", Score: 60, At: m(0)},
		{IP: "A", Kind: "device-mismatch", Score: 70, At: m(5)}, // 互证 → 触发
		{IP: "A", Kind: "rate", Score: 30, At: m(20)},           // 封禁后噪声，不作证据
		{IP: "B", Kind: "device-mismatch", Score: 70, At: m(0)},
		{IP: "C", Kind: "rate", Score: 30, At: m(0)},
		{IP: "C", Kind: "scanner", Score: 30, At: m(0)}, // 同分钟，无衰减，eff=60 恰达软封锁线
		{IP: "E", Kind: "admin-brute", Score: 60, At: m(0)},
		{IP: "E", Kind: "device-mismatch", Score: 70, At: m(5)},
		{IP: "F", Kind: "rate", Score: 30, At: m(0)},
		{IP: "F", Kind: "rate", Score: 30, At: m(3)},
		{IP: "G", Kind: "cluster-linked", Score: 0, At: m(0)},
	}
	bans := []ReplayBan{
		{IP: "A", BannedAt: m(7)}, // 模型在 m(5) 触发 → 提前 2 分钟
		{IP: "F", BannedAt: m(10)},
	}
	return events, bans
}

func TestReplaySummarizeDefaultParams(t *testing.T) {
	now := time.Now()
	events, bans := replayFixture(now)
	results := Replay(events, bans, DefaultParams(), 500)
	s := Summarize(results, DefaultParams())

	if s.BannedIPs != 2 {
		t.Fatalf("已封 IP 应为 2（A/F），实际 %d", s.BannedIPs)
	}
	if s.Caught != 1 || s.Missed != 1 {
		t.Fatalf("召回/未召回应为 1/1，实际 %d/%d", s.Caught, s.Missed)
	}
	if s.MedianLeadMin != 2 {
		t.Fatalf("提前量中位应为 2 分钟，实际 %.1f", s.MedianLeadMin)
	}
	if s.CleanIPs != 3 {
		t.Fatalf("未封 IP 应为 3（B/C/E），实际 %d", s.CleanIPs)
	}
	if s.FalsePositives != 1 {
		t.Fatalf("误报代理应为 1（E），实际 %d", s.FalsePositives)
	}
	if s.SoftFPs != 2 {
		t.Fatalf("软封锁误伤代理应为 2（B=70/C=60），实际 %d", s.SoftFPs)
	}
	if s.Recall() != 0.5 || s.FPProxy() != 1.0/3.0 {
		t.Fatalf("召回/误报率应为 0.5 / 0.333，实际 %.2f / %.3f", s.Recall(), s.FPProxy())
	}
}

func TestReplayParamSensitivity(t *testing.T) {
	now := time.Now()
	events, bans := replayFixture(now)

	// 阈值抬到 120：E 的互证 112 分不再触发（误报消失），A 也不触发（召回消失）
	high := DefaultParams()
	high.Threshold = 120
	s := Summarize(Replay(events, bans, high, 500), high)
	if s.FalsePositives != 0 {
		t.Fatalf("阈值 120 下误报应为 0，实际 %d", s.FalsePositives)
	}
	if s.Caught != 0 {
		t.Fatalf("阈值 120 下召回应为 0，实际 %d", s.Caught)
	}

	// 半衰期拉长：旧事件衰减更慢，B 的单证据持续窗口内仍是 70（软封锁线不变），
	// 但 A 的首事件衰减后仍贡献更多 → 触发不变。至少验证参数确实改变判定路径。
	slow := DefaultParams()
	slow.HalfLifeMin = 30
	r := Summarize(Replay(events, bans, slow, 500), slow)
	if r.Caught != 1 {
		t.Fatalf("半衰 30 下召回应保持 1，实际 %d", r.Caught)
	}
}

func TestReplayPostBanEventsNotEvidence(t *testing.T) {
	now := time.Now()
	// H：真实封禁前只有 0 分事件，触发级事件全部发生在封禁之后 → 不得算召回
	events := []ReplayEvent{
		{IP: "H", Kind: "cluster-linked", Score: 0, At: now.Add(-time.Hour)},
		{IP: "H", Kind: "admin-brute", Score: 60, At: now.Add(30 * time.Minute)}, // 封禁后
		{IP: "H", Kind: "device-mismatch", Score: 70, At: now.Add(35 * time.Minute)},
	}
	bans := []ReplayBan{{IP: "H", BannedAt: now}}
	s := Summarize(Replay(events, bans, DefaultParams(), 500), DefaultParams())
	if s.Caught != 0 || s.Missed != 1 {
		t.Fatalf("封禁后事件不应作为召回证据，实际 caught=%d missed=%d", s.Caught, s.Missed)
	}
}
