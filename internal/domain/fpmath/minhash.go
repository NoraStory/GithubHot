// Package fpmath 指纹相似度计算（纯算法域包：零 IO、零外部依赖，仅使用标准库）。
//
// 用途：把一条指纹表达成"组件字符串集合"，用 MinHash + LSH 判断两条指纹是否高度重合，
// 为持久层按桶召回候选、再按估计相似度归并"疑似同一来源"提供无状态算法能力。
//
// 算法依据：
//   - MinHash（Broder 1997）：k 个哈希函数 h_i(x) = (a_i*h(x) + b_i) mod p 的最小值签名，
//     两条签名等位的比例即 Jaccard 相似度的无偏估计；
//   - LSH 分带（Indyk & Motwani 1998）：把 k 位签名切成 bands 个带、每带 rows 行，
//     任一带完全相同即判定为候选对，把近似去重从 O(n*m) 全量对比降到按桶召回。
//
// 规格常量：模数 p = 2^61 - 1（Mersenne 素数，64 位乘法可用 Mul64/Div64 安全取模）；
// 本项目标准参数为签名 k = 128 位 = 16 带 × 8 行：单带完全相同的概率为 J^rows，
// J = 0.9 时整体漏检率约 (1 - 0.9^8)^16 ≈ 1.2e-4，J = 0.85 时约 6e-3。
//
// 确定性与纯函数性：哈希族系数由固定常量经 splitmix64 派生，元素先做 FNV-1a 64 位哈希、
// 再做模 p 的多项式哈希；全程不依赖随机数发生器、时间、IO 或任何外部状态，
// 同一份代码在任何进程、任何机器上都得到完全一致的签名与桶键。
package fpmath

import (
	"encoding/binary"
	"fmt"
	"math/bits"
)

// 本项目标准参数：签名 128 位 = 16 带 × 8 行。
const (
	// MinHashK 标准签名长度（哈希函数个数）。
	MinHashK = 128
	// LSHBands 标准分带数。
	LSHBands = 16
	// LSHRows 标准每带行数（LSHBands*LSHRows == MinHashK）。
	LSHRows = 8
)

// primeP 哈希族使用的素数模数 p = 2^61 - 1。
const primeP = uint64(1)<<61 - 1

// FNV-1a 64 位常量（仅在包内用于元素哈希）。
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// 哈希族系数的固定派生种子：常量取值本身无意义，只要求写死在代码里以保证跨进程一致。
const (
	coeffSeedA = 0x9E3779B97F4A7C15
	coeffSeedB = 0xD1B54A32D192ED03
)

// MinHash 固定长度的最小哈希签名器。
type MinHash struct{ k int }

// NewMinHash 构造签名长度为 k 的 MinHash；k <= 0 时使用标准长度 128（MinHashK）。
func NewMinHash(k int) *MinHash {
	if k <= 0 {
		k = MinHashK
	}
	return &MinHash{k: k}
}

// sigLen 返回有效签名长度：零值 MinHash 与非法长度都退回标准长度 128，避免调用方踩空指针/空切片。
func (m *MinHash) sigLen() int {
	if m == nil || m.k <= 0 {
		return MinHashK
	}
	return m.k
}

// Signature 对集合（元素为字符串）计算 k 个最小值签名。
//
// 语义：
//   - 顺序无关：结果与 items 的排列无关；
//   - 内部去重：重复元素只参与一次；
//   - 空集合（含 items 为 nil 或全部为重复元素）返回长度 k 的全 0 签名——
//     全 0 是"空集合"的哨兵值（单项哈希值被强制为非 0，因此非空集合不会产生全 0 签名）；
//   - 纯函数：同一集合多次调用逐元素相等。
func (m *MinHash) Signature(items []string) []uint64 {
	k := m.sigLen()
	sig := make([]uint64, k)
	seen := make(map[string]struct{}, len(items))
	first := true
	for _, item := range items {
		if _, dup := seen[item]; dup {
			continue
		}
		seen[item] = struct{}{}
		h := elemHash(item)
		for i := 0; i < k; i++ {
			v := minHashValue(i, h)
			if first || v < sig[i] {
				sig[i] = v
			}
		}
		first = false
	}
	return sig
}

// JaccardEstimate 由两条签名估计 Jaccard 相似度（等位比例）。
//
// 长度不一致时按两者较短者对齐比较（标准调用下长度都为 k）；任一签名为空返回 0。
// 这是无偏估计值：完全不相交的两个集合通常精确得到 0，只有在两个元素哈希值发生碰撞时
// 才可能出现非 0（概率量级 1/2^61）。
func JaccardEstimate(a, b []uint64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	same := 0
	for i := 0; i < n; i++ {
		if a[i] == b[i] {
			same++
		}
	}
	return float64(same) / float64(n)
}

// Bands 把签名切成 bands 个带、每带 rows 行，返回每带的桶键。
//
// 桶键形如 "b03-7f0c1a2b3d4e5f60"（带号 + 该带所有行的十六进制哈希），是纯 ASCII 文本，
// 可直接写入 SQLite 文本列做等值匹配召回。带号参与哈希，避免不同带的相同行值在同一列里互相误撞。
//
// 边界行为：
//   - bands <= 0 或 rows <= 0 返回 nil；
//   - 返回值长度恒为 bands（用 LSHBands/LSHRows 调用即 16 个键）；
//   - 签名长度不足 bands*rows 时，末尾带按实际行数处理；整段落在签名之外的带退化为
//     "整条签名参与哈希"，保证输出的确定性与可重复，正常参数（128 = 16*8）下不会走到该分支。
func Bands(sig []uint64, bands, rows int) []string {
	if bands <= 0 || rows <= 0 {
		return nil
	}
	out := make([]string, 0, bands)
	for b := 0; b < bands; b++ {
		start := b * rows
		if start >= len(sig) {
			out = append(out, bandKey(b, sig))
			continue
		}
		end := start + rows
		if end > len(sig) {
			end = len(sig)
		}
		out = append(out, bandKey(b, sig[start:end]))
	}
	return out
}

// minHashValue 计算第 i 个哈希函数在元素哈希 h 上的取值 h_i = (a_i*h + b_i) mod p。
//
// 乘加都在模 p 意义下完成：a_i*h 用 Mul64+Div64，加 b_i 后结果 < 2p，单次条件减法即可。
// 取值强制非 0，使"全 0 签名"唯一地表示空集合。
//
// a_i 与素数 p 互素，x ↦ (a_i*x + b_i) mod p 是 Z_p 上的置换（等价于一次随机排列），
// 这正是"等位比例的期望 = Jaccard 相似度"这一无偏性的前提。
func minHashValue(i int, h uint64) uint64 {
	a, b := coefficients(i)
	v := mulModP(a, h) + b
	if v >= primeP {
		v -= primeP
	}
	if v == 0 {
		v = 1
	}
	return v
}

// coefficients 返回第 i 组哈希族系数 (a_i, b_i)，取值均在 [1, p-1]。
//
// 系数由固定常量经 splitmix64 派生（等价于固定种子序列，但不依赖 math/rand 的实现版本），
// 因此任意进程、任意机器、任意 Go 版本都得到同一组系数。
func coefficients(i int) (uint64, uint64) {
	a := splitmix64(uint64(i) + coeffSeedA)
	b := splitmix64(uint64(i) + coeffSeedB)
	return a%(primeP-1) + 1, b%(primeP-1) + 1
}

// mulModP 计算 (a*b) mod p：64×64 → 128 位乘法后整除取余，不经过浮点、不丢高位。
// 前置条件 a, b < p，此时 Mul64 的高位 < p，Div64 不会溢出。
func mulModP(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	_, rem := bits.Div64(hi, lo, primeP)
	return rem
}

// splitmix64 固定常量的确定性混合函数（Steele 等 2014），无状态、跨平台位运算一致。
func splitmix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// elemHash 元素字符串 → uint64：先 FNV-1a 64 位，再把 8 个字节作为多项式系数在模 p 下求值。
//
// 二次多项式哈希让 8 个字节全部参与扩散，并强制结果非 0——直接对 FNV 结果取模可能出现
// 0 值，使该元素在所有 h_i 上退化成同一个常数 b_i。
func elemHash(s string) uint64 {
	return polyHashP(fnv1a64(s))
}

// fnv1a64 计算字符串的 FNV-1a 64 位哈希（逐字节，无额外分配）。
func fnv1a64(s string) uint64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(s); i++ {
		h = fnvByte(h, s[i])
	}
	return h
}

// fnvByte FNV-1a 的单字节步进。
func fnvByte(h uint64, b byte) uint64 {
	return (h ^ uint64(b)) * fnvPrime64
}

// polyHashP 把 x 的 8 个字节从低位到高位当作多项式系数，在模 p 下求值；结果为 0 时取 1。
func polyHashP(x uint64) uint64 {
	acc := uint64(0)
	for i := 0; i < 8; i++ {
		acc = mulModP(acc, fnvPrime64) + (x>>(8*uint(i)))&0xff
		if acc >= primeP {
			acc -= primeP
		}
	}
	if acc == 0 {
		acc = 1
	}
	return acc
}

// bandKey 计算第 band 带的桶键：带号 + 行值序列的 FNV-1a 哈希，输出十六进制文本。
func bandKey(band int, vals []uint64) string {
	h := fnvByte(uint64(fnvOffset64), byte(band))
	h = fnvByte(h, byte(band>>8))
	var buf [8]byte
	for _, v := range vals {
		binary.LittleEndian.PutUint64(buf[:], v)
		for i := 0; i < len(buf); i++ {
			h = fnvByte(h, buf[i])
		}
	}
	return fmt.Sprintf("b%02d-%016x", band, h)
}
