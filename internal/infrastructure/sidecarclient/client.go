// Package sidecarclient 是主服务调用 mlserve（GNN 在线推理 sidecar）的客户端。
// 三态契约（规格书 §9 P6-3b / §13.1）：
//   1. 未配置（baseURL 为空）→ ErrDisabled，调用方保持纯离线模式；
//   2. 调用失败/超时 → ErrSidecar，调用方回落 Louvain 社区均值；
//   3. 成功 → ScoreResult 写回 gnn_score / gnn_embedding。
// 超时硬约束 500ms（规格书 P6-3b），不阻塞 fp/report 上报路径。
// Package sidecarclient 是主服务调用 mlserve（GNN 在线推理 sidecar）的客户端（P6-3b）。
// 三态契约：未配置→ErrDisabled；失败/超时→ErrSidecar；成功→ScoreResult 写回。
// 从沙盒 go-client/ 迁入，模块路径改为 github.com/NoraStory/GithubHot/internal/infrastructure/sidecarclient。
package sidecarclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrDisabled 表示 GNN_SIDECAR_URL 未配置（纯离线模式）。
	ErrDisabled = errors.New("sidecar: disabled (GNN_SIDECAR_URL empty)")
	// ErrSidecar 表示 sidecar 不可达、超时或返回错误（调用方应回落离线分数）。
	ErrSidecar = errors.New("sidecar: unavailable")
)

const DefaultTimeout = 500 * time.Millisecond

type Node struct {
	ID       string    `json:"id"`
	Features []float32 `json:"features"`
}

type ScoreRequest struct {
	Nodes []Node    `json:"nodes"`
	Edges [][2]int  `json:"edges"`
}

type ScoreResult struct {
	Scores       map[string]float32   `json:"scores"`
	Embeddings   map[string][]float32 `json:"embeddings"`
	ModelVersion string               `json:"model_version"`
	ElapsedMS    int64                `json:"elapsed_ms"`
}

type Client struct {
	base  string
	token string
	hc    *http.Client
}

// New 创建客户端；baseURL 为空 → 纯离线模式（Enabled()==false）。
func New(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		base:  baseURL,
		token: token,
		hc:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool { return c != nil && c.base != "" }

// Score 调用 sidecar /v1/score。任何失败统一包装为 ErrSidecar（保留原错误链）。
func (c *Client) Score(ctx context.Context, req ScoreRequest) (*ScoreResult, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrSidecar, err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/v1/score", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrSidecar, err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		hreq.Header.Set("X-Sidecar-Token", c.token)
	}
	hresp, err := c.hc.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSidecar, err)
	}
	defer hresp.Body.Close()
	if hresp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrSidecar, hresp.StatusCode)
	}
	var result ScoreResult
	if err := json.NewDecoder(hresp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrSidecar, err)
	}
	return &result, nil
}
