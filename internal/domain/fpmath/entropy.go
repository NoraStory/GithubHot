package fpmath

// P2-3 熵值加权：把"分量的罕见程度"折算成指纹置信度（信息量，bit）。
// 大众配置（人手一份的 canvas/webgl/字体组合）熵低 → 违规时封禁更保守；
// 罕见组合熵高 → 可信度高，违规时封禁更果断。封禁触发系数 = min(1, bits/40)，
// 由引擎侧（ipguard）套用，本包只做纯计算。以下均为纯函数。

import "math"

// EntropyWeights 计算每条指纹的分量熵权（规格书 §5 P2-3）。
//
// 输入：各指纹的分量表（键 → 值，如 canvas/webgl/fonts → 各自的指纹值）。
// 对每个键统计各值在"有该键的指纹"中的出现次数 c(v)，权重 w = -log2(c(v)/N_key)；
// 指纹整体置信度 bits = Σ_key w(key→value)。
//
// 语义约定：
//   - N_key = 该键非空的指纹数（缺失该键的指纹不参与该键的分母，避免拉低权重）；
//   - 某键全部指纹同值（c(v)=N_key）→ w=0，无区分度；
//   - 键缺失的指纹不累加该键权重（缺失本身按 0 信息处理，不臆测）；
//   - 输出与输入等长、顺序一致；空输入返回空切片。
func EntropyWeights(components []map[string]string) []float64 {
	out := make([]float64, len(components))
	if len(components) == 0 {
		return out
	}
	// 每键的值计数与有该键的行数
	type keyStat struct {
		counts map[string]int
		rows   int
	}
	stats := map[string]*keyStat{}
	for _, m := range components {
		for k, v := range m {
			if v == "" {
				continue
			}
			st := stats[k]
			if st == nil {
				st = &keyStat{counts: map[string]int{}}
				stats[k] = st
			}
			st.counts[v]++
			st.rows++
		}
	}
	for i, m := range components {
		bits := 0.0
		for k, v := range m {
			st := stats[k]
			if st == nil || v == "" {
				continue
			}
			bits += -math.Log2(float64(st.counts[v]) / float64(st.rows))
		}
		out[i] = math.Round(bits*100) / 100
	}
	return out
}
