package fpmath

import (
	"fmt"
	"math"
	"testing"
)

// comps 生成 n 个组件元素，用于模拟一条指纹的组件集合。
func comps(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("%s-%d", prefix, i))
	}
	return out
}

// overlapSets 构造两个集合（元素取自同一元素池），交集恰好 overlap 个元素。
func overlapSets(sizeA, sizeB, overlap int) ([]string, []string) {
	pool := comps("e", sizeA+sizeB-overlap)
	a := append([]string{}, pool[:sizeA]...)
	b := append([]string{}, pool[sizeA-overlap:sizeA-overlap+sizeB]...)
	return a, b
}

// exactJaccard 精确 Jaccard（集合语义），作为估计误差对比基准。
func exactJaccard(a, b []string) float64 {
	sa := make(map[string]struct{}, len(a))
	for _, x := range a {
		sa[x] = struct{}{}
	}
	sb := make(map[string]struct{}, len(b))
	for _, x := range b {
		sb[x] = struct{}{}
	}
	inter := 0
	for x := range sa {
		if _, ok := sb[x]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// sigOf 用标准参数（k=128）对集合求签名。
func sigOf(items []string) []uint64 {
	return NewMinHash(MinHashK).Signature(items)
}

// sharedBands 统计两组桶键相同的带数。
func sharedBands(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	same := 0
	for i := 0; i < n; i++ {
		if a[i] == b[i] {
			same++
		}
	}
	return same
}

// TestJaccardIdenticalAndDisjoint 完全相同集合 → 1.0；完全不相交集合 → 0.0。
//
// 说明：哈希族 x ↦ (a_i*x + b_i) mod p 是 Z_p 上的置换，两个不相交集合的最小值要相同，
// 必须存在两个元素哈希值碰撞，因此不相交集合的估计值实际上精确为 0（不是"近似为 0"）。
func TestJaccardIdenticalAndDisjoint(t *testing.T) {
	base := comps("base", 20)

	t.Run("完全相同集合估计为1", func(t *testing.T) {
		sigA := sigOf(base)
		sigB := sigOf(append([]string{}, base...))
		if got := JaccardEstimate(sigA, sigB); got != 1.0 {
			t.Fatalf("相同集合 JaccardEstimate = %v, 期望 1.0", got)
		}
	})

	t.Run("顺序不同不算差异", func(t *testing.T) {
		shuffled := []string{base[3], base[17], base[0], base[9], base[3], base[11]}
		shuffled = append(shuffled, base...)
		if got := JaccardEstimate(sigOf(base), sigOf(shuffled)); got != 1.0 {
			t.Fatalf("乱序+重复集合 JaccardEstimate = %v, 期望 1.0", got)
		}
	})

	t.Run("完全不相交估计为0", func(t *testing.T) {
		cases := []struct {
			name string
			a, b []string
		}{
			{"20对20不相交", comps("left", 20), comps("right", 20)},
			{"5对5不相交", comps("aa", 5), comps("bb", 5)},
			{"规模悬殊不相交", comps("big", 50), comps("small", 3)},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				exact := exactJaccard(c.a, c.b)
				if exact != 0 {
					t.Fatalf("测试数据构造错误：精确 Jaccard = %v, 期望 0", exact)
				}
				if got := JaccardEstimate(sigOf(c.a), sigOf(c.b)); got != 0 {
					t.Fatalf("不相交集合 JaccardEstimate = %v, 期望 0", got)
				}
			})
		}
	})

	t.Run("不相交集合估计远小于阈值", func(t *testing.T) {
		// 不相交但元素池不同的多组组合：估计值只应来自桶巧合，必须远小于 0.1。
		pairs := [][2]string{
			{"f1", "f2"}, {"g1", "g2"}, {"h1", "h2"}, {"i1", "i2"},
		}
		for _, p := range pairs {
			a, b := comps(p[0], 24), comps(p[1], 24)
			got := JaccardEstimate(sigOf(a), sigOf(b))
			if got >= 0.1 {
				t.Errorf("%s/%s 不相交但估计值 = %v, 期望 < 0.1", p[0], p[1], got)
			}
		}
	})
}

// TestOneElementChangeTolerance 20 个组件的集合改动 1 个元素：估计值仍 >= 0.85，且至少 1 条带相同。
func TestOneElementChangeTolerance(t *testing.T) {
	base := comps("comp", 20)
	sigBase := sigOf(base)
	bandsBase := Bands(sigBase, LSHBands, LSHRows)

	cases := []struct {
		name string
		// mutate 返回改动后的集合
		mutate func() []string
	}{
		{"替换1个元素", func() []string {
			out := append([]string{}, base...)
			out[2] = "comp-replaced"
			return out
		}},
		{"新增1个元素", func() []string {
			out := append([]string{}, base...)
			return append(out, "comp-added")
		}},
		{"删除1个元素", func() []string {
			out := append([]string{}, base[:len(base)-1]...)
			return out
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed := c.mutate()
			exact := exactJaccard(base, changed)
			if exact < 0.85 {
				t.Fatalf("测试数据构造错误：精确 Jaccard = %v, 期望 >= 0.85", exact)
			}
			est := JaccardEstimate(sigBase, sigOf(changed))
			if est < 0.85 {
				t.Fatalf("改动1个元素后估计值 = %v, 期望 >= 0.85", est)
			}
			shared := sharedBands(bandsBase, Bands(sigOf(changed), LSHBands, LSHRows))
			if shared < 1 {
				t.Fatalf("改动1个元素后共享带数 = 0, 期望 >= 1（LSH 应有容错）")
			}
			t.Logf("精确=%.4f 估计=%.4f 共享带=%d/%d", exact, est, shared, LSHBands)
		})
	}
}

// TestJaccardEstimateAccuracy 不同重叠度（50%/80%/90%）下估计值与精确 Jaccard 的误差 < 0.1。
func TestJaccardEstimateAccuracy(t *testing.T) {
	cases := []struct {
		name            string
		sizeA, sizeB    int
		overlap         int
		exactWant       float64
		maxAbsDeviation float64
	}{
		{"重叠50%", 30, 30, 20, 0.5, 0.1},
		{"重叠80%", 45, 45, 40, 0.8, 0.1},
		{"重叠90%", 95, 95, 90, 0.9, 0.1},
		{"重叠33%", 20, 20, 10, 1.0 / 3.0, 0.1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := overlapSets(c.sizeA, c.sizeB, c.overlap)
			exact := exactJaccard(a, b)
			if math.Abs(exact-c.exactWant) > 1e-9 {
				t.Fatalf("测试数据构造错误：精确 Jaccard = %v, 期望 %v", exact, c.exactWant)
			}
			est := JaccardEstimate(sigOf(a), sigOf(b))
			if math.Abs(est-exact) >= c.maxAbsDeviation {
				t.Fatalf("估计值 = %v, 精确值 = %v, 误差 %v, 期望 < %v",
					est, exact, math.Abs(est-exact), c.maxAbsDeviation)
			}
			t.Logf("精确=%.4f 估计=%.4f 误差=%.4f", exact, est, math.Abs(est-exact))
		})
	}
}

// TestSignatureDeterminism 同一集合多次调用签名逐元素相等（同一次测试内稳定，无隐藏随机性）。
func TestSignatureDeterminism(t *testing.T) {
	items := comps("det", 37)

	t.Run("同集合重复调用一致", func(t *testing.T) {
		first := sigOf(items)
		for round := 0; round < 5; round++ {
			again := sigOf(append([]string{}, items...))
			if len(again) != len(first) {
				t.Fatalf("第 %d 次调用长度 = %d, 期望 %d", round, len(again), len(first))
			}
			for i := range first {
				if again[i] != first[i] {
					t.Fatalf("第 %d 次调用第 %d 位 = %d, 期望 %d", round, i, again[i], first[i])
				}
			}
		}
	})

	t.Run("乱序与重复输入结果一致", func(t *testing.T) {
		shuffled := make([]string, 0, len(items)*2)
		for i := len(items) - 1; i >= 0; i-- {
			shuffled = append(shuffled, items[i], items[i])
		}
		shuffled = append(shuffled, items...)
		want := sigOf(items)
		got := sigOf(shuffled)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("乱序+重复输入第 %d 位 = %d, 期望 %d", i, got[i], want[i])
			}
		}
	})

	t.Run("签名长度遵循k", func(t *testing.T) {
		lenCases := []struct {
			k    int
			want int
		}{
			{MinHashK, 128},
			{32, 32},
			{1, 1},
			{0, 128},
			{-7, 128},
		}
		for _, c := range lenCases {
			if got := len(NewMinHash(c.k).Signature(items)); got != c.want {
				t.Errorf("NewMinHash(%d).Signature 长度 = %d, 期望 %d", c.k, got, c.want)
			}
		}
	})
}

// TestEmptyAndNilInputs 空集合与 nil 输入不 panic，签名全 0、长度正常，空签名估计值为 0。
func TestEmptyAndNilInputs(t *testing.T) {
	m := NewMinHash(MinHashK)

	t.Run("nil集合不panic且签名全0", func(t *testing.T) {
		sig := m.Signature(nil)
		if len(sig) != MinHashK {
			t.Fatalf("nil 集合签名长度 = %d, 期望 %d", len(sig), MinHashK)
		}
		for i, v := range sig {
			if v != 0 {
				t.Fatalf("nil 集合第 %d 位 = %d, 期望 0（空集合哨兵）", i, v)
			}
		}
	})

	t.Run("空切片与全重复元素遵循同一哨兵规则", func(t *testing.T) {
		cases := []struct {
			name     string
			items    []string
			wantZero bool
		}{
			{"空切片", []string{}, true},
			{"单元素", []string{"only"}, false},
			{"多元素", []string{"a", "b"}, false},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				sig := m.Signature(c.items)
				if len(sig) != MinHashK {
					t.Fatalf("输入 %v 签名长度 = %d, 期望 %d", c.items, len(sig), MinHashK)
				}
				if got := allZero(sig); got != c.wantZero {
					t.Fatalf("输入 %v 全0 = %v, 期望 %v", c.items, got, c.wantZero)
				}
			})
		}
	})

	t.Run("空签名估计值为0", func(t *testing.T) {
		estCases := []struct {
			name string
			a, b []uint64
		}{
			{"双nil", nil, nil},
			{"空切片", []uint64{}, []uint64{}},
			{"一侧为空", sampleSig(), nil},
			{"一侧长度0另一侧有值", nil, []uint64{1, 2, 3}},
		}
		for _, c := range estCases {
			t.Run(c.name, func(t *testing.T) {
				if got := JaccardEstimate(c.a, c.b); got != 0 {
					t.Fatalf("JaccardEstimate = %v, 期望 0", got)
				}
			})
		}
	})

	t.Run("零值与空签名不panic", func(t *testing.T) {
		var zero MinHash
		if got := len(zero.Signature(nil)); got != MinHashK {
			t.Fatalf("零值 MinHash 签名长度 = %d, 期望 %d", got, MinHashK)
		}
		var nilSig []uint64
		if got := Bands(nilSig, LSHBands, LSHRows); len(got) != LSHBands {
			t.Fatalf("空签名分带数 = %d, 期望 %d", len(got), LSHBands)
		}
		if got := JaccardEstimate(zero.Signature(nil), nilSig); got != 0 {
			t.Fatalf("空签名估计 = %v, 期望 0", got)
		}
	})
}

// sampleSig 返回一条长度为 MinHashK 且非全 0 的签名，用于"一侧为空"的估计测试。
func sampleSig() []uint64 {
	return sigOf([]string{"x1", "x2"})
}

// allZero 判断签名是否全为 0（空集合哨兵）。
func allZero(sig []uint64) bool {
	for _, v := range sig {
		if v != 0 {
			return false
		}
	}
	return true
}

// TestBandsDeterministicAndDiscriminative 分带可重复（相同输入 → 相同输出），不同集合至少一条带不同。
func TestBandsDeterministicAndDiscriminative(t *testing.T) {
	a := comps("band-a", 20)
	b := comps("band-b", 20)

	t.Run("可重复且长度固定", func(t *testing.T) {
		sig := sigOf(a)
		first := Bands(sig, LSHBands, LSHRows)
		if len(first) != LSHBands {
			t.Fatalf("分带数 = %d, 期望 %d", len(first), LSHBands)
		}
		for round := 0; round < 3; round++ {
			again := Bands(sigOf(a), LSHBands, LSHRows)
			for i := range first {
				if again[i] != first[i] {
					t.Fatalf("第 %d 轮第 %d 带 = %q, 期望 %q", round, i, again[i], first[i])
				}
			}
		}
		for i, key := range first {
			if key == "" {
				t.Fatalf("第 %d 带桶键为空", i)
			}
		}
	})

	t.Run("不同集合至少一条带不同", func(t *testing.T) {
		bandsA := Bands(sigOf(a), LSHBands, LSHRows)
		bandsB := Bands(sigOf(b), LSHBands, LSHRows)
		if shared := sharedBands(bandsA, bandsB); shared == LSHBands {
			t.Fatalf("完全不同的集合全部 %d 条带相同，缺少区分度", shared)
		}
	})

	t.Run("带键可作SQLite文本列", func(t *testing.T) {
		for _, key := range Bands(sigOf(a), LSHBands, LSHRows) {
			for _, r := range key {
				if r > 0x7f || r < 0x21 {
					t.Fatalf("桶键 %q 含非可见 ASCII 字符 %q", key, r)
				}
			}
		}
	})

	t.Run("非法参数返回nil", func(t *testing.T) {
		sig := sigOf(a)
		for _, c := range []struct{ bands, rows int }{{0, 8}, {16, 0}, {-1, -1}} {
			if got := Bands(sig, c.bands, c.rows); got != nil {
				t.Fatalf("Bands(sig,%d,%d) = %v, 期望 nil", c.bands, c.rows, got)
			}
		}
	})

	t.Run("签名短于带总长仍确定性", func(t *testing.T) {
		short := NewMinHash(8).Signature(a) // 8 行 < 16 带 × 8 行
		first := Bands(short, LSHBands, LSHRows)
		if len(first) != LSHBands {
			t.Fatalf("短签名分带数 = %d, 期望 %d", len(first), LSHBands)
		}
		again := Bands(short, LSHBands, LSHRows)
		for i := range first {
			if again[i] != first[i] {
				t.Fatalf("短签名第 %d 带不可重复：%q vs %q", i, first[i], again[i])
			}
		}
		other := Bands(NewMinHash(8).Signature(b), LSHBands, LSHRows)
		if sharedBands(first, other) == LSHBands {
			t.Fatalf("短签名下不同集合全部带相同，缺少区分度")
		}
	})
}
