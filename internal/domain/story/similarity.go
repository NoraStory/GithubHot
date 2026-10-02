package story

import (
	"math"
	"strings"
	"unicode"
)

// LexicalSimilarity 词面相似度：英文按词元、中日韩按二元组，Jaccard 系数。
// 用途：未配置向量模型时，为 LLM 聚簇判断提供候选召回；向量可用时仅作降级路径。
// 返回值域 [0,1]。
func LexicalSimilarity(a, b string) float64 {
	ta := tokenize(a)
	tb := tokenize(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// tokenize 产生词元集合：连续拉丁字母数字按词；连续 CJK 字符按二元组。
func tokenize(s string) map[string]bool {
	out := map[string]bool{}
	var word strings.Builder
	var cjk []rune
	flushWord := func() {
		if word.Len() > 1 {
			out[strings.ToLower(word.String())] = true
		}
		word.Reset()
	}
	flushCJK := func() {
		for i := 0; i+1 < len(cjk); i++ {
			out[string([]rune{cjk[i], cjk[i+1]})] = true
		}
		cjk = cjk[:0]
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word.WriteRune(r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return out
}

// Cosine 余弦相似度（向量聚簇用）。
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
