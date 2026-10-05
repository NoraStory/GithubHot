package fpmath

import "testing"

func TestParsePHash(t *testing.T) {
	if v, err := ParsePHash("0000000000000001"); err != nil || v != 1 {
		t.Fatalf("解析失败: %v %d", err, v)
	}
	if _, err := ParsePHash("xyz"); err == nil {
		t.Fatalf("非法输入应报错")
	}
	if _, err := ParsePHash("1234567890abcdef0"); err == nil {
		t.Fatalf("长度不符应报错")
	}
	if _, err := ParsePHash(""); err == nil {
		t.Fatalf("空串应报错")
	}
}

func TestPHashDistance(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"完全相同", "0f0f0f0f0f0f0f0f", "0f0f0f0f0f0f0f0f", 0},
		{"一位不同", "0000000000000000", "0000000000000001", 1},
		{"全反", "0000000000000000", "ffffffffffffffff", 64},
		{"半不同", "00000000ffffffff", "0000000000000000", 32},
		{"非法输入", "0000000000000000", "zz", -1},
		{"大写等价", "ABCDEF0123456789", "abcdef0123456789", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PHashDistance(c.a, c.b); got != c.want {
				t.Fatalf("distance = %d, want %d", got, c.want)
			}
		})
	}
}

func TestSimilarPHash(t *testing.T) {
	a := "0f0f0f0f0f0f0f0f"
	near := "0f0f0f0f0f0f0f0e" // 距离 1
	far := "f0f0f0f0f0f0f0f0"  // 距离 64
	if !SimilarPHash(a, near, 0) {
		t.Fatalf("距离 1 应同源（默认阈值 10）")
	}
	if SimilarPHash(a, far, 0) {
		t.Fatalf("距离 64 不应同源")
	}
	if SimilarPHash(a, "bad", 0) {
		t.Fatalf("非法输入不应同源")
	}
	if !SimilarPHash(a, far, 64) {
		t.Fatalf("阈值放大后应为同源")
	}
}

func TestSimilarPHashCandidates(t *testing.T) {
	target := "0000000000000000"
	cands := []PHashCandidate{
		{FP: "far", PHash: "ffffffffffffffff"},   // 64
		{FP: "near2", PHash: "0000000000000003"}, // 2
		{FP: "near1", PHash: "0000000000000001"}, // 1
		{FP: "bad", PHash: ""},
		{FP: "", PHash: "0000000000000000"},
	}
	got := SimilarPHashCandidates(target, cands, 10)
	if len(got) != 2 {
		t.Fatalf("应命中 2 条，实际 %#v", got)
	}
	if got[0].FP != "near1" || got[0].Distance != 1 {
		t.Fatalf("应按距离升序，实际 %#v", got[0])
	}
	if got[1].FP != "near2" || got[1].Distance != 2 {
		t.Fatalf("第二位应为 near2，实际 %#v", got[1])
	}
	// 目标非法 → 不做任何关联（宁可漏，不误关联）
	if out := SimilarPHashCandidates("zz", cands, 10); out != nil {
		t.Fatalf("非法目标应返回 nil，实际 %#v", out)
	}
	// 空候选集
	if out := SimilarPHashCandidates(target, nil, 10); len(out) != 0 {
		t.Fatalf("空候选集应返回空，实际 %#v", out)
	}
}
