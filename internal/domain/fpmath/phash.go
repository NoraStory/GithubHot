package fpmath

// P2-1 pHash：前端把 Canvas 渲染结果压成 64bit 感知哈希（hex16），服务端只做汉明距离与
// 同源判定——同一设备因驱动更新/抗指纹噪声导致的像素微改，pHash 距离仍很小，从而把
// "换了手指纹"关联回同一台设备（图聚类与轮换检测的依据）。以下均为纯函数。

import (
	"encoding/hex"
	"errors"
	"math/bits"
	"strings"
)

// PHashDefaultMaxDistance 汉明距离 ≤ 10 视为同源（规格 §5 P2-1）。
const PHashDefaultMaxDistance = 10

// PHashHexLen 64bit 感知哈希的十六进制长度。
const PHashHexLen = 16

var errBadPHash = errors.New("非法 pHash（需 16 位十六进制）")

// ParsePHash 解析 hex16 感知哈希为 uint64。
func ParsePHash(s string) (uint64, error) {
	t := strings.TrimSpace(strings.ToLower(s))
	if len(t) != PHashHexLen {
		return 0, errBadPHash
	}
	b, err := hex.DecodeString(t)
	if err != nil || len(b) != 8 {
		return 0, errBadPHash
	}
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, nil
}

// PHashDistance 两个 hex16 感知哈希的汉明距离；任一非法返回 -1（调用方按"不同源"处理）。
func PHashDistance(a, b string) int {
	va, err1 := ParsePHash(a)
	vb, err2 := ParsePHash(b)
	if err1 != nil || err2 != nil {
		return -1
	}
	return bits.OnesCount64(va ^ vb)
}

// SimilarPHash 汉明距离 ≤ maxDistance 视为同源；maxDistance <= 0 时用默认 10。
// 非法输入一律返回 false（宁可漏关联，不误关联）。
func SimilarPHash(a, b string, maxDistance int) bool {
	if maxDistance <= 0 {
		maxDistance = PHashDefaultMaxDistance
	}
	d := PHashDistance(a, b)
	return d >= 0 && d <= maxDistance
}

// SimilarPHashCandidates 在候选集中挑出与 target 同源者，按距离升序返回（距离相同时按 fp 升序）。
// 纯函数：候选集由调用方（存储层）提供。
func SimilarPHashCandidates(target string, candidates []PHashCandidate, maxDistance int) []PHashCandidate {
	if _, err := ParsePHash(target); err != nil {
		return nil
	}
	if maxDistance <= 0 {
		maxDistance = PHashDefaultMaxDistance
	}
	out := make([]PHashCandidate, 0, 4)
	for _, c := range candidates {
		if c.FP == "" {
			continue
		}
		d := PHashDistance(target, c.PHash)
		if d < 0 || d > maxDistance {
			continue
		}
		out = append(out, PHashCandidate{FP: c.FP, PHash: c.PHash, Distance: d})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if out[j-1].Distance < out[j].Distance ||
				(out[j-1].Distance == out[j].Distance && out[j-1].FP <= out[j].FP) {
				break
			}
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// PHashCandidate 候选指纹（距离字段由计算填充）。
type PHashCandidate struct {
	FP       string
	PHash    string
	Distance int
}
