// Package llm 实现 OpenAI 兼容的大模型网关（chat JSON + embeddings）。
// 出站请求统一经 safehttp 做 SSRF 校验——因此自建内网推理端点（localhost/
// 私有地址）会被拒绝，这是有意的安全设计；请使用公网推理服务。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// EmbedConfig 向量端点配置。BaseURL / APIKey 留空时复用对话端点的值——
// 同一家服务商零额外配置；混搭（如 DeepSeek 对话 + 智谱向量）时显式指定。
type EmbedConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 留空 = 不启用向量，聚簇退化为词面相似度
}

// OpenAI 兼容网关。BaseURL 形如 https://api.deepseek.com（自动补 /v1/chat/completions）。
type OpenAI struct {
	BaseURL string
	APIKey  string
	modelA  string
	modelB  string
	embed   EmbedConfig
}

// New 构造；modelB 为空时复用 modelA（以温度差异近似"独立第二次评分"）。
func New(baseURL, apiKey, modelA, modelB string, embed EmbedConfig) (*OpenAI, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("LLM_BASE_URL 不能为空")
	}
	if modelA == "" {
		return nil, errors.New("LLM_MODEL 不能为空")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("LLM_BASE_URL 仅允许 http/https: %s", baseURL)
	}
	if modelB == "" {
		modelB = modelA
	}
	e := EmbedConfig{
		BaseURL: strings.TrimRight(strings.TrimSpace(embed.BaseURL), "/"),
		APIKey:  embed.APIKey,
		Model:   strings.TrimSpace(embed.Model),
	}
	if e.BaseURL == "" {
		e.BaseURL = baseURL
	}
	if e.APIKey == "" {
		e.APIKey = apiKey
	}
	if !strings.HasPrefix(e.BaseURL, "http://") && !strings.HasPrefix(e.BaseURL, "https://") {
		return nil, fmt.Errorf("LLM_EMBED_BASE_URL 仅允许 http/https: %s", e.BaseURL)
	}
	return &OpenAI{BaseURL: baseURL, APIKey: apiKey, modelA: modelA, modelB: modelB, embed: e}, nil
}

// ModelA 主模型。
func (o *OpenAI) ModelA() string { return o.modelA }

// ModelB 第二评分模型。
func (o *OpenAI) ModelB() string { return o.modelB }

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ChatJSON 发起一次要求 JSON 输出的对话；429/5xx 指数退避重试两次。
func (o *OpenAI) ChatJSON(ctx context.Context, system, user, model string, temperature float64) (string, error) {
	req := chatRequest{
		Model:       model,
		Messages:    []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature: temperature,
	}
	req.ResponseFormat = &struct {
		Type string `json:"type"`
	}{Type: "json_object"}

	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimSuffix(o.BaseURL, "/") + "/chat/completions"
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + o.APIKey,
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * 2 * time.Second):
			}
		}
		body, status, err := safehttp.Do(ctx, "POST", endpoint, headers, bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			continue
		}
		if status == 429 || status >= 500 {
			lastErr = fmt.Errorf("LLM 返回 %d: %s", status, safeSnippet(body))
			continue
		}
		if status != 200 {
			return "", fmt.Errorf("LLM 返回 %d: %s", status, safeSnippet(body))
		}
		var resp chatResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", fmt.Errorf("解析 LLM 响应: %w", err)
		}
		if resp.Error != nil {
			return "", fmt.Errorf("LLM 错误: %s", resp.Error.Message)
		}
		if len(resp.Choices) == 0 {
			return "", errors.New("LLM 返回空 choices")
		}
		return extractJSON(resp.Choices[0].Message.Content), nil
	}
	return "", fmt.Errorf("LLM 调用重试耗尽: %w", lastErr)
}

// Embed 批量语义向量；未配置向量模型返回 ErrEmbeddingsUnsupported。
// 端点与 Key 独立于对话端点（EmbedConfig），支持跨服务商混搭。
func (o *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if o.embed.Model == "" {
		return nil, application.ErrEmbeddingsUnsupported
	}
	payload, err := json.Marshal(struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{Model: o.embed.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	endpoint := o.embedEndpoint()
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + o.embed.APIKey,
	}
	body, status, err := safehttp.Do(ctx, "POST", endpoint, headers, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("embeddings 返回 %d: %s", status, safeSnippet(body))
	}
	var resp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析 embeddings 响应: %w", err)
	}
	out := make([][]float32, len(resp.Data))
	for _, d := range resp.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	return out, nil
}

// embedEndpoint 向量端点地址（独立于对话端点，可测试）。
func (o *OpenAI) embedEndpoint() string {
	return strings.TrimSuffix(o.embed.BaseURL, "/") + "/embeddings"
}

// extractJSON 从回复中提取 JSON（容忍 markdown 代码块包裹）。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i:]
		rest = strings.TrimPrefix(rest, "```json")
		rest = strings.TrimPrefix(rest, "```")
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		s = strings.TrimSpace(rest)
	}
	// 有些模型在 JSON 前后带说明文字：截取首个 { 到末个 }
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	return s
}

func safeSnippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return strings.TrimSpace(s)
}

// 编译期接口满足性检查。
var _ application.LLMGateway = (*OpenAI)(nil)
