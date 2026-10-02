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

// EmbedStyle 向量接口风格。
type EmbedStyle string

const (
	// EmbedStyleOpenAI 标准 OpenAI /embeddings：批量输入、支持 dimensions。
	EmbedStyleOpenAI EmbedStyle = "openai"
	// EmbedStyleArkMultimodal 火山方舟多模态 /embeddings/multimodal：
	// 单条输入（input 为 type/text 块数组）、固定维度（不支持 dimensions）。
	// 用于 doubao-embedding-vision 系列。
	EmbedStyleArkMultimodal EmbedStyle = "ark-multimodal"
)

// EmbedConfig 向量端点配置。BaseURL / APIKey 留空时复用对话端点的值——
// 同一家服务商零额外配置；混搭（如 DeepSeek 对话 + 智谱向量）时显式指定。
type EmbedConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 留空 = 不启用向量，聚簇退化为词面相似度
	// Dimensions 输出维度（MRL 可调表示）。Qwen3-Embedding-8B 最大 4096；
	// 0 = 使用服务商默认值。ark-multimodal 风格忽略（固定维度）。
	Dimensions int
	// Style 接口风格，见 EmbedStyle 常量；空 = openai。
	Style EmbedStyle
}

// embedBatchSize 每次 embeddings 请求的最大输入条数。
// 各服务商批量上限不一（有的低至 16/32），取保守值分批请求。
const embedBatchSize = 16

// OpenAI 兼容网关。BaseURL 形如 https://api.deepseek.com（自动补 /v1/chat/completions）。
type OpenAI struct {
	BaseURL string
	APIKey  string
	// Thinking 思考模式开关（火山方舟 doubao-seed 系列）：
	// "disabled" 关闭深度思考（快约 5 倍，适合结构化判定任务），
	// "enabled" 强制开启，空 = 服务商默认。
	Thinking string
	modelA   string
	modelB   string
	embed    EmbedConfig
	onUsage  UsageFunc
}

// Option 网关可选参数。
type Option func(*OpenAI)

// WithThinking 设置思考模式（LLM_THINKING: disabled / enabled）。
func WithThinking(mode string) Option {
	return func(o *OpenAI) { o.Thinking = mode }
}

// New 构造；modelB 为空时复用 modelA（以温度差异近似"独立第二次评分"）。
func New(baseURL, apiKey, modelA, modelB string, embed EmbedConfig, opts ...Option) (*OpenAI, error) {
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
		BaseURL:    strings.TrimRight(strings.TrimSpace(embed.BaseURL), "/"),
		APIKey:     embed.APIKey,
		Model:      strings.TrimSpace(embed.Model),
		Dimensions: embed.Dimensions,
		Style:      embed.Style,
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
	switch e.Style {
	case "", EmbedStyleOpenAI, EmbedStyleArkMultimodal:
		// 合法
	default:
		return nil, fmt.Errorf("LLM_EMBED_STYLE 非法: %q（支持 openai / ark-multimodal）", e.Style)
	}
	if e.Style == "" {
		e.Style = EmbedStyleOpenAI
	}
	o := &OpenAI{BaseURL: baseURL, APIKey: apiKey, modelA: modelA, modelB: modelB, embed: e}
	for _, opt := range opts {
		opt(o)
	}
	return o, nil
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
	Thinking *struct {
		Type string `json:"type"`
	} `json:"thinking,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage 一次调用的 token 用量。
type Usage struct {
	Kind             string // chat / embed
	Model            string
	PromptTokens     int
	CompletionTokens int
}

// UsageFunc 用量回调（预算熔断与计量在应用层实现，网关只上报）。
type UsageFunc func(u Usage)

// WithUsageRecorder 注册用量回调。
func WithUsageRecorder(fn UsageFunc) Option {
	return func(o *OpenAI) { o.onUsage = fn }
}

// SetUsageRecorder 组合根在包装网关后设置用量回调。
func (o *OpenAI) SetUsageRecorder(fn UsageFunc) { o.onUsage = fn }

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage chatUsage `json:"usage"`
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
	switch o.Thinking {
	case "disabled", "enabled":
		req.Thinking = &struct {
			Type string `json:"type"`
		}{Type: o.Thinking}
	}

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
		if o.onUsage != nil {
			o.onUsage(Usage{Kind: "chat", Model: model,
				PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens})
		}
		return extractJSON(resp.Choices[0].Message.Content), nil
	}
	return "", fmt.Errorf("LLM 调用重试耗尽: %w", lastErr)
}

// embedRequest OpenAI 兼容 embeddings 请求体。
// encoding_format 显式声明为 float——OpenAI 规范默认 base64，
// 不声明可能拿到 base64 编码而破坏数字数组解析。
type embedRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	Dimensions     int      `json:"dimensions,omitempty"`
	EncodingFormat string   `json:"encoding_format"`
}

// Embed 批量语义向量；未配置向量模型返回 ErrEmbeddingsUnsupported。
// 端点与 Key 独立于对话端点（EmbedConfig），支持跨服务商混搭与两种接口风格。
func (o *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if o.embed.Model == "" {
		return nil, application.ErrEmbeddingsUnsupported
	}
	if o.embed.Style == EmbedStyleArkMultimodal {
		return o.embedArkMultimodal(ctx, texts)
	}
	return o.embedOpenAI(ctx, texts)
}

// embedOpenAI 标准 OpenAI 风格：批量输入按 embedBatchSize 分批，
// 每批最多重试两次（向量模型冷启动/限流常见瞬时错误）。
func (o *OpenAI) embedOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	endpoint := o.embedEndpoint()
	headers := o.embedHeaders()
	out := make([][]float32, 0, len(texts))
	for _, chunk := range chunkStrings(texts, embedBatchSize) {
		payload, err := json.Marshal(embedRequest{
			Model:          o.embed.Model,
			Input:          chunk,
			Dimensions:     o.embed.Dimensions,
			EncodingFormat: "float",
		})
		if err != nil {
			return nil, err
		}
		body, err := postWithRetry(ctx, endpoint, headers, payload)
		if err != nil {
			return nil, err
		}
		part, promptTokens, err := parseEmbedResponse(body, len(chunk))
		if err != nil {
			return nil, err
		}
		if o.onUsage != nil && promptTokens > 0 {
			o.onUsage(Usage{Kind: "embed", Model: o.embed.Model, PromptTokens: promptTokens})
		}
		out = append(out, part...)
	}
	return out, nil
}

// arkInput 方舟多模态输入块。
type arkInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// embedArkMultimodal 火山方舟多模态风格：该接口每次只产出一条向量
// （多条文本块会被合并编码），因此逐条请求；日常数百条 × 约 300ms
// 在日更流水线里可接受。doubao-embedding-vision 系列固定维度。
func (o *OpenAI) embedArkMultimodal(ctx context.Context, texts []string) ([][]float32, error) {
	endpoint := strings.TrimSuffix(o.embed.BaseURL, "/") + "/embeddings/multimodal"
	headers := o.embedHeaders()
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		payload, err := json.Marshal(struct {
			Model string     `json:"model"`
			Input []arkInput `json:"input"`
		}{Model: o.embed.Model, Input: []arkInput{{Type: "text", Text: text}}})
		if err != nil {
			return nil, err
		}
		body, err := postWithRetry(ctx, endpoint, headers, payload)
		if err != nil {
			return nil, err
		}
		vec, promptTokens, err := parseArkEmbedResponse(body)
		if err != nil {
			return nil, err
		}
		if o.onUsage != nil && promptTokens > 0 {
			o.onUsage(Usage{Kind: "embed", Model: o.embed.Model, PromptTokens: promptTokens})
		}
		out = append(out, vec)
	}
	return out, nil
}

// embedHeaders 向量请求头（Key 独立于对话端点）。
func (o *OpenAI) embedHeaders() map[string]string {
	return map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + o.embed.APIKey,
	}
}

// postWithRetry POST JSON：传输错误与 429/5xx 指数退避重试（共 3 次尝试）。
func postWithRetry(ctx context.Context, endpoint string, headers map[string]string, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * 2 * time.Second):
			}
		}
		body, status, err := safehttp.Do(ctx, "POST", endpoint, headers, bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			continue
		}
		if status == 429 || status >= 500 {
			lastErr = fmt.Errorf("端点返回 %d: %s", status, safeSnippet(body))
			continue
		}
		if status != 200 {
			return nil, fmt.Errorf("端点返回 %d: %s", status, safeSnippet(body))
		}
		return body, nil
	}
	return nil, fmt.Errorf("请求重试耗尽: %w", lastErr)
}

// parseEmbedResponse 解析并校验一批向量：条数必须与请求一致，
// 拒绝"HTTP 200 + 错误体"（如 {"code":20015,...}）被静默当成空向量。
// 返回对齐后的向量与该批输入 token 数。
func parseEmbedResponse(body []byte, want int) ([][]float32, int, error) {
	var resp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage chatUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("解析 embeddings 响应: %w", err)
	}
	if len(resp.Data) != want {
		return nil, 0, fmt.Errorf("embeddings 响应条数不符（期望 %d 得到 %d）: %s", want, len(resp.Data), safeSnippet(body))
	}
	// 按 index 对齐，容忍服务商乱序返回
	part := make([][]float32, want)
	for _, d := range resp.Data {
		if d.Index >= 0 && d.Index < want {
			part[d.Index] = d.Embedding
		}
	}
	for i, v := range part {
		if len(v) == 0 {
			return nil, 0, fmt.Errorf("embeddings 响应第 %d 条向量为空: %s", i, safeSnippet(body))
		}
	}
	return part, resp.Usage.PromptTokens, nil
}

// chunkStrings 按每批 n 条切分输入（n<=0 时视为 1）。
func chunkStrings(xs []string, n int) [][]string {
	if n <= 0 {
		n = 1
	}
	var out [][]string
	for i := 0; i < len(xs); i += n {
		end := i + n
		if end > len(xs) {
			end = len(xs)
		}
		out = append(out, xs[i:end])
	}
	return out
}

// embedEndpoint 向量端点地址（按风格路由，独立于对话端点，可测试）。
func (o *OpenAI) embedEndpoint() string {
	if o.embed.Style == EmbedStyleArkMultimodal {
		return strings.TrimSuffix(o.embed.BaseURL, "/") + "/embeddings/multimodal"
	}
	return strings.TrimSuffix(o.embed.BaseURL, "/") + "/embeddings"
}

// parseArkEmbedResponse 解析方舟多模态向量：data 为单对象（data.embedding），
// 同时返回 usage 的输入 token 数。
func parseArkEmbedResponse(body []byte) (vec []float32, promptTokens int, err error) {
	var resp struct {
		Data struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage chatUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("解析方舟多模态向量响应: %w", err)
	}
	if len(resp.Data.Embedding) == 0 {
		return nil, 0, fmt.Errorf("方舟多模态向量响应为空: %s", safeSnippet(body))
	}
	return resp.Data.Embedding, resp.Usage.PromptTokens, nil
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
