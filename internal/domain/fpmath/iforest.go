// P5-2 Isolation Forest（规格书 §8 P5-2）：自研纯 Go，零依赖，确定性（固定种子）。
//
// 算法：iTree 递归随机选特征 + 随机选 [min,max] 分割点，直到节点样本数 ≤ 1 或
// 高度达 ceil(log2(采样数))；异常分 = 2^(-平均路径长 / c(样本数))，c(n) 为 BST 平均
// 不成功查找路径长约 2·ln(n−1)+0.5772。分数 ∈ (0,1)，越接近 1 越异常。
// 训练与打分均为纯函数；采样与特征选择用注入的 *rand.Rand 保证跨进程可复现。
package fpmath

import (
	"math"
	"math/rand"
	"sort"
)

// IForest 隔离森林。
type IForest struct {
	trees  []*iTreeNode
	nTrees int
}

type iTreeNode struct {
	feature   int
	threshold float64
	left      *iTreeNode
	right     *iTreeNode
	isLeaf    bool
	size      int
}

// TrainIForest 训练：samples 为行向量，nTrees 默认 100，子采样 256（规格经典参数）。
func TrainIForest(samples [][]float64, seed int64) *IForest {
	nTrees := 100
	subsample := 256
	if len(samples) < subsample {
		subsample = len(samples)
	}
	rng := rand.New(rand.NewSource(seed))
	f := &IForest{nTrees: nTrees}
	for i := 0; i < nTrees; i++ {
		// 随机子采样
		idx := rng.Perm(len(samples))[:subsample]
		sub := make([][]float64, 0, subsample)
		for _, j := range idx {
			sub = append(sub, samples[j])
		}
		f.trees = append(f.trees, buildITree(sub, 0, heightLimit(subsample), rng))
	}
	return f
}

func heightLimit(n int) int { return int(math.Ceil(math.Log2(float64(n)))) }

func buildITree(samples [][]float64, depth, limit int, rng *rand.Rand) *iTreeNode {
	node := &iTreeNode{size: len(samples)}
	if depth >= limit || len(samples) <= 1 {
		node.isLeaf = true
		return node
	}
	// 随机选一个有区分度的特征
	feats := make([]int, 0, 4)
	if len(samples) > 0 {
		for j := range samples[0] {
			min, max := samples[0][j], samples[0][j]
			for _, s := range samples {
				if s[j] < min {
					min = s[j]
				}
				if s[j] > max {
					max = s[j]
				}
			}
			if max > min {
				feats = append(feats, j)
			}
		}
	}
	if len(feats) == 0 {
		node.isLeaf = true
		return node
	}
	feat := feats[rng.Intn(len(feats))]
	var min, max float64 = samples[0][feat], samples[0][feat]
	for _, s := range samples {
		if s[feat] < min {
			min = s[feat]
		}
		if s[feat] > max {
			max = s[feat]
		}
	}
	split := min + rng.Float64()*(max-min)
	var left, right [][]float64
	for _, s := range samples {
		if s[feat] < split {
			left = append(left, s)
		} else {
			right = append(right, s)
		}
	}
	if len(left) == 0 || len(right) == 0 {
		node.isLeaf = true
		return node
	}
	node.feature = feat
	node.threshold = split
	node.left = buildITree(left, depth+1, limit, rng)
	node.right = buildITree(right, depth+1, limit, rng)
	return node
}

// pathLength 单样本在单棵树的路径长（未达叶时以 c(节点样本数) 补偿）。
func (t *iTreeNode) pathLength(x []float64, depth int) float64 {
	if t.isLeaf || t.left == nil {
		return float64(depth) + cFactor(float64(t.size))
	}
	if x[t.feature] < t.threshold {
		return t.left.pathLength(x, depth+1)
	}
	return t.right.pathLength(x, depth+1)
}

// Score 异常分：2^(-E[h]/c(n)) ∈ (0,1)；>0.6 明显异常，≈0.5 正常。
// 输入向量必须与训练同维。
func (f *IForest) Score(x []float64) float64 {
	if f == nil || len(f.trees) == 0 {
		return 0.5
	}
	sum := 0.0
	for _, t := range f.trees {
		sum += t.pathLength(x, 0)
	}
	avg := sum / float64(len(f.trees))
	n := float64(f.trees[0].size)
	return math.Pow(2, -avg/cFactor(n))
}

// cFactor BST 平均不成功查找路径长：c(n) = 2·H(n−1) − 2(n−1)/n，H(i) ≈ ln(i) + 0.5772。
func cFactor(n float64) float64 {
	if n <= 1 {
		return 0
	}
	return 2*(math.Log(n-1)+0.5772156649) - 2*(n-1)/n
}

// QuantileThreshold 分位阈值（按打分集排序取分位；规格：99.5 分位仅记录 / 99.9 入队列）。
func QuantileThreshold(scores []float64, q float64) float64 {
	if len(scores) == 0 {
		return 1
	}
	sorted := append([]float64{}, scores...)
	sort.Float64s(sorted)
	idx := int(q * float64(len(sorted)-1))
	return sorted[idx]
}
