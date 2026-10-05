package fpmath

import (
	"math"
	"testing"
)

// EntropyWeights 熵权计算（P2-3）：值越罕见权重越高；全同值无区分度（0）；
// 缺失键不参与该键统计；输出与输入等长。
func TestEntropyWeights(t *testing.T) {
	sets := []map[string]string{
		{"canvas": "common", "fonts": "rare-x"},
		{"canvas": "common", "fonts": "rare-x"},
		{"canvas": "common", "fonts": "unique-y"},
		{}, // 空分量表：bits=0，且不改变其他行的分母（按"有该键的行数"统计）
	}
	got := EntropyWeights(sets)
	if len(got) != len(sets) {
		t.Fatalf("输出长度 %d，应为 %d", len(got), len(sets))
	}
	// canvas 三条同值 → 0；fonts: 2/3 → 0.585、1/3 → 1.585
	want0 := -math.Log2(2.0 / 3.0)
	want1 := -math.Log2(1.0 / 3.0)
	if math.Abs(got[0]-want0) > 0.011 {
		t.Fatalf("rows[0] 应 ≈ %.3f（rounded），实际 %.2f", want0, got[0])
	}
	if math.Abs(got[2]-want1) > 0.011 {
		t.Fatalf("rows[2] 应 ≈ %.3f（rounded），实际 %.2f", want1, got[2])
	}
	if got[3] != 0 {
		t.Fatalf("空分量表 bits 应为 0，实际 %.2f", got[3])
	}
	// 全同值 → 0（无区分度）
	same := EntropyWeights([]map[string]string{
		{"canvas": "x"}, {"canvas": "x"},
	})
	if same[0] != 0 {
		t.Fatalf("全同值权重应为 0，实际 %.2f", same[0])
	}
	if got := EntropyWeights(nil); len(got) != 0 {
		t.Fatalf("空输入应返回空切片")
	}
}
