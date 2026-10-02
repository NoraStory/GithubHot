package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/NoraStory/GithubHot/internal/application"
)

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name                              string
		baseURL, modelA, embedURL, embedM string
		wantErr                           bool
	}{
		{"缺 base URL", "", "m", "", "", true},
		{"缺主模型", "https://api.deepseek.com", "", "", "", true},
		{"非法 scheme", "ftp://api.deepseek.com", "m", "", "", true},
		{"非法向量 scheme", "https://api.deepseek.com", "m", "ftp://x.com", "e", true},
		{"正常", "https://api.deepseek.com", "m", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.baseURL, "k", tc.modelA, "", EmbedConfig{BaseURL: tc.embedURL, Model: tc.embedM})
			if tc.wantErr && err == nil {
				t.Fatal("应报错")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("不应报错: %v", err)
			}
		})
	}
}

func TestEmbedConfigFallback(t *testing.T) {
	// 留空向量端点/Key → 复用对话端点（同一家服务商零配置）
	o, err := New("https://api.deepseek.com", "main-key", "m", "", EmbedConfig{Model: "emb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if o.embedEndpoint() != "https://api.deepseek.com/embeddings" {
		t.Fatalf("向量端点应复用对话 BaseURL，得到 %s", o.embedEndpoint())
	}
	if o.embed.APIKey != "main-key" {
		t.Fatalf("向量 Key 应复用主 Key，得到 %q", o.embed.APIKey)
	}

	// 显式混搭：DeepSeek 对话 + 另一家向量
	o2, err := New("https://api.deepseek.com", "main-key", "m", "", EmbedConfig{
		BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIKey: "embed-key", Model: "embedding-3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if o2.embedEndpoint() != "https://open.bigmodel.cn/api/paas/v4/embeddings" {
		t.Fatalf("混搭向量端点错误: %s", o2.embedEndpoint())
	}
	if o2.embed.APIKey != "embed-key" {
		t.Fatalf("混搭向量 Key 错误: %q", o2.embed.APIKey)
	}
}

func TestEmbedUnsupportedWithoutModel(t *testing.T) {
	// 未配向量模型：不发任何网络请求，直接返回哨兵错误
	o, err := New("https://api.deepseek.com", "k", "m", "", EmbedConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = o.Embed(context.Background(), []string{"文本"})
	if !errors.Is(err, application.ErrEmbeddingsUnsupported) {
		t.Fatalf("应返回 ErrEmbeddingsUnsupported，得到 %v", err)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`{"a":1}`, `{"a":1}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"好的，结果如下：{\"a\":1} 以上。", `{"a":1}`},
		{`{"a":{"b":2}}`, `{"a":{"b":2}}`},
	}
	for _, c := range cases {
		if got := extractJSON(c.in); got != c.want {
			t.Errorf("extractJSON(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestChatEndpointComposition(t *testing.T) {
	o, err := New("https://api.deepseek.com/", "k", "m", "", EmbedConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(o.BaseURL, "/api.deepseek.com") {
		t.Fatalf("末尾斜杠应被去掉: %s", o.BaseURL)
	}
}

func TestChunkStrings(t *testing.T) {
	xs := []string{"a", "b", "c", "d", "e"}
	chunks := chunkStrings(xs, 2)
	if len(chunks) != 3 || strings.Join(chunks[0], "") != "ab" ||
		strings.Join(chunks[1], "") != "cd" || strings.Join(chunks[2], "") != "e" {
		t.Fatalf("分块错误: %v", chunks)
	}
	if got := chunkStrings(nil, 4); len(got) != 0 {
		t.Fatalf("空输入应无分块: %v", got)
	}
	// 恰好整除
	chunks2 := chunkStrings([]string{"1", "2", "3", "4"}, 2)
	if len(chunks2) != 2 {
		t.Fatalf("整除应得 2 块: %v", chunks2)
	}
}

func TestEmbedRequestBodyShape(t *testing.T) {
	// 维度取最大（4096）时请求体应带 dimensions；0 时应省略
	withDims, err := json.Marshal(embedRequest{
		Model: "Qwen/Qwen3-Embedding-8B", Input: []string{"x"},
		Dimensions: 4096, EncodingFormat: "float",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"dimensions":4096`, `"encoding_format":"float"`, `"model":"Qwen/Qwen3-Embedding-8B"`} {
		if !strings.Contains(string(withDims), want) {
			t.Fatalf("请求体缺 %s: %s", want, withDims)
		}
	}
	withoutDims, _ := json.Marshal(embedRequest{Model: "m", Input: []string{"x"}, EncodingFormat: "float"})
	if strings.Contains(string(withoutDims), "dimensions") {
		t.Fatalf("dimensions=0 应被省略: %s", withoutDims)
	}
}
