package fpmath

import (
	"math"
	"testing"
)

// TestIForestOutlierScores 离群点得分应显著高于内群点。
func TestIForestOutlierScores(t *testing.T) {
	// 内群：集中在 (0,0) 附近的二维点
	inliers := make([][]float64, 200)
	for i := range inliers {
		inliers[i] = []float64{float64(i%7) * 0.1, float64(i%5) * 0.1}
	}
	f := TrainIForest(inliers, 42)
	inlierScore := f.Score([]float64{0.1, 0.2})
	outlierScore := f.Score([]float64{100.0, -100.0})
	if inlierScore >= outlierScore {
		t.Fatalf("离群分应更高：内群 %.3f vs 离群 %.3f", inlierScore, outlierScore)
	}
	if outlierScore <= 0.5 {
		t.Fatalf("极端离群点得分应 > 0.5，实际 %.3f", outlierScore)
	}
}

// TestIForestDeterminism 同种子训练结果确定性。
func TestIForestDeterminism(t *testing.T) {
	samples := [][]float64{{1, 2}, {3, 4}, {5, 6}, {7, 8}}
	f1 := TrainIForest(samples, 7)
	f2 := TrainIForest(samples, 7)
	x := []float64{2.5, 3.5}
	if f1.Score(x) != f2.Score(x) {
		t.Fatalf("同种子应确定性")
	}
}

// TestQuantileThreshold 分位阈值。
func TestQuantileThreshold(t *testing.T) {
	scores := make([]float64, 100)
	for i := range scores {
		scores[i] = float64(i) / 100
	}
	if q := QuantileThreshold(scores, 0.99); q < 0.98 || q > 1 {
		t.Fatalf("99 分位应 ≈ 0.99，实际 %.3f", q)
	}
	if q := QuantileThreshold(nil, 0.99); q != 1 {
		t.Fatalf("空集阈值应为 1（全拒），实际 %.3f", q)
	}
}

// TestLRPredict 与手算对拍：z = w·(x−μ)/σ + b，p = sigmoid(z)。
func TestLRPredict(t *testing.T) {
	m := &LRModel{
		Active:  true,
		Mu:      []float64{10, 5},
		Sigma:   []float64{2, 1},
		Weights: []float64{0.5, -1},
		Bias:    0.1,
	}
	x := []float64{12, 3} // (12-10)/2=1, (3-5)/1=-2 → z = 0.5*1 + (-1)*(-2) + 0.1 = 2.6
	want := 1 / (1 + math.Exp(-2.6))
	if got := m.Predict(x); math.Abs(got-want) > 1e-9 {
		t.Fatalf("predict = %.12f, want %.12f", got, want)
	}
	// σ=0 → 按 1 处理
	m2 := &LRModel{Active: true, Mu: []float64{0}, Sigma: []float64{0}, Weights: []float64{1}, Bias: 0}
	if got := m2.Predict([]float64{3}); math.Abs(got-1/(1+math.Exp(-3))) > 1e-9 {
		t.Fatalf("σ=0 应按 1 处理")
	}
	// 未激活 / 维度不符 → -1
	m.Active = false
	if got := m.Predict(x); got != -1 {
		t.Fatalf("未激活应返回 -1")
	}
	m.Active = true
	if got := m.Predict([]float64{1}); got != -1 {
		t.Fatalf("维度不符应返回 -1")
	}
}
