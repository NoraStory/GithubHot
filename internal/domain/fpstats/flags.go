// Package fpstats 指纹命中统计（纯算法，零 IO、零依赖）。
//
// 输入是"某时间窗内活跃指纹"的 flags 快照（ip_fingerprints.flags 存的是历史并集），
// 输出各 flag 的命中排行——P1/P2 新增检测的灰度观察面板靠它判断假阳性率。
package fpstats

import (
	"sort"
	"time"
)

// FlagSample 一条指纹的 flags 快照。
type FlagSample struct {
	FP       string
	Flags    []string
	Visits   int       // 该指纹累计上报次数（近似"命中次数"权重）
	LastSeen time.Time // 最近一次上报时间（窗口判定依据）
}

// FlagStat 单个 flag 的命中统计。
type FlagStat struct {
	Key         string `json:"key"`
	Hits        int    `json:"hits"`         // 命中次数（Σ 命中指纹的上报次数，下限 1/指纹）
	AffectedFPs int    `json:"affected_fps"` // 涉及指纹数
}

// AggregateFlags 统计窗口内各 flag 的命中情况。
//
//   - since 非零时忽略 LastSeen 早于它的样本（窗口外）；
//   - 同一指纹内重复出现的 key 只计一次（flags 列本身是并集，去重防脏数据放大）；
//   - 空 key 忽略；
//   - 排序：Hits 降序 → AffectedFPs 降序 → key 升序（结果稳定可测）；
//   - limit <= 0 返回全部。
func AggregateFlags(samples []FlagSample, since time.Time, limit int) []FlagStat {
	hits := map[string]int{}
	fps := map[string]int{}
	for _, s := range samples {
		if !since.IsZero() && s.LastSeen.Before(since) {
			continue
		}
		seen := map[string]bool{}
		weight := s.Visits
		if weight < 1 {
			weight = 1
		}
		for _, k := range s.Flags {
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			hits[k] += weight
			fps[k]++
		}
	}
	out := make([]FlagStat, 0, len(hits))
	for k, h := range hits {
		out = append(out, FlagStat{Key: k, Hits: h, AffectedFPs: fps[k]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		if out[i].AffectedFPs != out[j].AffectedFPs {
			return out[i].AffectedFPs > out[j].AffectedFPs
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
