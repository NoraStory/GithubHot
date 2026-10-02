package story

import (
	"math"
	"testing"
)

func TestLexicalSimilarityEnglish(t *testing.T) {
	a := "OpenAI releases GPT-5 with reasoning mode"
	b := "OpenAI released GPT-5 reasoning mode today"
	if s := LexicalSimilarity(a, b); s < 0.4 {
		t.Fatalf("同题英文报道相似度应较高，得到 %.2f", s)
	}
	if s := LexicalSimilarity(a, "Kubernetes 1.33 adds sidecar support"); s > 0.15 {
		t.Fatalf("无关内容相似度应很低，得到 %.2f", s)
	}
}

func TestLexicalSimilarityCJK(t *testing.T) {
	a := "OpenAI 发布新一代推理模型"
	b := "OpenAI 推出新一代推理模型"
	if s := LexicalSimilarity(a, b); s < 0.4 {
		t.Fatalf("中文同题相似度应较高，得到 %.2f", s)
	}
}

func TestLexicalSimilarityEmpty(t *testing.T) {
	if LexicalSimilarity("", "abc") != 0 {
		t.Fatal("空串相似度应为 0")
	}
}

func TestCosine(t *testing.T) {
	a := []float32{1, 0, 1}
	b := []float32{1, 0, 1}
	if c := Cosine(a, b); math.Abs(c-1) > 1e-6 {
		t.Fatalf("同向向量余弦应为 1，得到 %.4f", c)
	}
	c := Cosine([]float32{1, 0}, []float32{0, 1})
	if math.Abs(c) > 1e-6 {
		t.Fatalf("正交向量余弦应为 0，得到 %.4f", c)
	}
	if Cosine(nil, nil) != 0 {
		t.Fatal("空向量余弦应为 0")
	}
}
