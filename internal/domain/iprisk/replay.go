// replay.go 离线回放评估（算法改进 A 批）：把历史违规事件流按时间升序重放给
// 候选参数组，与真实封禁记录（标签）对比，输出召回率 / 误报代理率 / 触发提前量。
//
// 方法学口径（读数字前必读）：
//   - 与生产 decideBan 一致的语义：每个事件落地后立刻评估一次，事件流按 IP 分组、
//     时间升序，评估只看该时刻之前的事件（衰减 + 窗口由参数决定）。
//   - 回放无法还原当时的 UA 多样性（ip_events 不存 UA），统一按单用户（不稀释）
//     处理——这是召回与误报的**保守上界**：共享出口的真实场景两者都会更低。
//   - "召回"= 模型在该 IP 真实封禁时刻（含）前触发；封禁后的事件不作为证据
//     （封禁已发生，事后事件是噪声）。
//   - "误报代理"= 模型对从未被封（有人工解封记录的会被算作已封）的 IP 触发封禁。
//     部分是真实的应封未封（管理员手软），部分是误伤——需结合管理端记录人工研判。
//   - "软封锁误伤代理"= 未封 IP 的有效分达 0.6×阈值（httpapi 软封锁线同款口径），
//     用于评估 P1-7 软封锁阶梯的误伤面。
package iprisk

import (
	"sort"
	"time"
)

// ReplayEvent 回放输入事件。
type ReplayEvent struct {
	IP    string
	Kind  string
	Score int
	At    time.Time
}

// ReplayBan 真实封禁标签。
type ReplayBan struct {
	IP       string
	BannedAt time.Time
	Reason   string
}

// IPResult 单个 IP 的回放结果。
type IPResult struct {
	IP string
	// ModelBanAt 模型首次触发封禁的时刻（零值 = 全程未触发）
	ModelBanAt time.Time
	// ModelEff 触发时刻的有效分
	ModelEff float64
	// MaxEff 回放全程达到过的最大有效分（软封锁评估用）
	MaxEff float64
	// ActualBanAt 真实封禁时刻（零值 = 该 IP 从未被封）
	ActualBanAt time.Time
}

// ReplaySummary 一组参数在一份历史数据上的总体表现。
type ReplaySummary struct {
	Params         Params  `json:"params"`
	BannedIPs      int     `json:"banned_ips"`      // 标签：实际被封 IP 数
	Caught         int     `json:"caught"`          // 召回：模型在真实封禁时刻前触发
	Missed         int     `json:"missed"`          // 未召回
	MedianLeadMin  float64 `json:"median_lead_min"` // 触发提前量中位数（正 = 早于真实封禁）
	CleanIPs       int     `json:"clean_ips"`       // 有事件历史但从未被封
	FalsePositives int     `json:"false_positives"` // 误报代理：模型对未封 IP 触发封禁
	SoftFPs        int     `json:"soft_fps"`        // 软封锁误伤代理：未封 IP 有效分 ≥ 0.6×阈值
}

// Recall 召回率（0-1；无标签 IP 时返回 NaN 口径的 0）。
func (s ReplaySummary) Recall() float64 {
	if s.BannedIPs == 0 {
		return 0
	}
	return float64(s.Caught) / float64(s.BannedIPs)
}

// FPProxy 误报代理率（对未封 IP 的触发比例）。
func (s ReplaySummary) FPProxy() float64 {
	if s.CleanIPs == 0 {
		return 0
	}
	return float64(s.FalsePositives) / float64(s.CleanIPs)
}

// softBlockFactor 与 httpapi 软封锁线同口径（有效分 0.6×阈值）。
const replaySoftBlockFactor = 0.6

// Replay 逐 IP 重放：按时间升序每落一个事件评估一次（与生产 decideBan 同语义），
// 记录模型首次触发封禁的时刻/有效分及全程最大有效分。
// maxEventsPerIP 限制单 IP 回放的事件数（取最近的，防个别超高频 IP 拖爆计算）。
func Replay(events []ReplayEvent, bans []ReplayBan, p Params, maxEventsPerIP int) []IPResult {
	banAt := map[string]time.Time{}
	for _, b := range bans {
		if b.BannedAt.After(banAt[b.IP]) {
			banAt[b.IP] = b.BannedAt // 多次封禁取最近一次（解封后再封）
		}
	}
	byIP := map[string][]ReplayEvent{}
	for _, e := range events {
		byIP[e.IP] = append(byIP[e.IP], e)
	}
	if maxEventsPerIP <= 0 {
		maxEventsPerIP = 500
	}

	out := make([]IPResult, 0, len(byIP))
	for ip, evs := range byIP {
		sort.Slice(evs, func(i, j int) bool { return evs[i].At.Before(evs[j].At) })
		if len(evs) > maxEventsPerIP {
			evs = evs[len(evs)-maxEventsPerIP:]
		}
		res := IPResult{IP: ip}
		if at, ok := banAt[ip]; ok {
			res.ActualBanAt = at
		}
		// 滑窗重放：in-window 事件列表 + 每步评估。
		window := make([]Event, 0, 64)
		for _, e := range evs {
			// 封禁后事件不作为"触发"证据（封禁已发生），但仍推进时钟清窗
			window = append(window, Event{Kind: e.Kind, Score: e.Score, At: e.At})
			// 裁掉窗口外旧事件（EvaluateWith 内部也会按 age 过滤，这里只是控制容量）
			cut := e.At.Add(-time.Duration(p.DecayWindowMin * float64(time.Minute)))
			k := 0
			for _, w := range window {
				if w.At.After(cut) {
					window[k] = w
					k++
				}
			}
			window = window[:k]

			d := EvaluateWith(p, window, e.At, 1 /* 单用户口径：不稀释 */)
			if d.Effective > res.MaxEff {
				res.MaxEff = d.Effective
			}
			if !res.ModelBanAt.IsZero() {
				continue // 已记录首次触发，后续只更新 MaxEff
			}
			if d.Ban && (res.ActualBanAt.IsZero() || !e.At.After(res.ActualBanAt)) {
				res.ModelBanAt = e.At
				res.ModelEff = d.Effective
			}
		}
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

// Summarize 逐 IP 结果 × 真实标签 → 单参数组指标。
func Summarize(results []IPResult, p Params) ReplaySummary {
	s := ReplaySummary{Params: p}
	leads := make([]float64, 0, 16)
	for _, r := range results {
		if r.ActualBanAt.IsZero() {
			// 零信号 IP（全程只有 0 分事件）不进分母——它永远不会触发，只会稀释误报率
			if r.MaxEff <= 0 {
				continue
			}
			// 未封 IP：模型触发封禁 = 误报代理；未触发但有效分达软封锁线 = 软封锁误伤代理
			s.CleanIPs++
			if !r.ModelBanAt.IsZero() {
				s.FalsePositives++
			} else if r.MaxEff >= p.Threshold*replaySoftBlockFactor {
				s.SoftFPs++
			}
			continue
		}
		s.BannedIPs++
		if r.ModelBanAt.IsZero() {
			s.Missed++
			continue
		}
		s.Caught++
		lead := r.ActualBanAt.Sub(r.ModelBanAt).Minutes()
		if lead < 0 {
			lead = 0 // 滞后触发按 0 记（不奖励滞后）
		}
		leads = append(leads, lead)
	}
	if len(leads) > 0 {
		sort.Float64s(leads)
		s.MedianLeadMin = leads[len(leads)/2]
	}
	return s
}
