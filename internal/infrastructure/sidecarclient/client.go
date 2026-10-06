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
	"log"
	"net/http"
	"sync"
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

	// 健康检查状态
	healthMu    sync.RWMutex
	healthy     bool
	failCount   int
	lastCheck   time.Time
	lastError   error
	checkTicker *time.Ticker
	stopCh      chan struct{}
}

// New 创建客户端；baseURL 为空 → 纯离线模式（Enabled()==false）。
func New(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	c := &Client{
		base:    baseURL,
		token:   token,
		hc:      &http.Client{Timeout: timeout},
		healthy: true, // 初始假设健康，首次检查时验证
		stopCh:  make(chan struct{}),
	}
	// 启用时开启后台健康检查
	if c.Enabled() {
		c.startHealthCheck()
	}
	return c
}

func (c *Client) Enabled() bool { return c != nil && c.base != "" }

// IsHealthy 返回sidecar当前是否健康（快速读锁检查）。
func (c *Client) IsHealthy() bool {
	if !c.Enabled() {
		return false
	}
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthy
}

// HealthStatus 返回健康状态详情（用于监控/诊断）。
func (c *Client) HealthStatus() map[string]interface{} {
	if !c.Enabled() {
		return map[string]interface{}{"enabled": false}
	}
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	status := map[string]interface{}{
		"enabled":    true,
		"healthy":    c.healthy,
		"fail_count": c.failCount,
		"last_check": c.lastCheck,
	}
	if c.lastError != nil {
		status["last_error"] = c.lastError.Error()
	}
	return status
}

// startHealthCheck 启动后台健康检查goroutine（30秒间隔）。
func (c *Client) startHealthCheck() {
	c.checkTicker = time.NewTicker(30 * time.Second)
	go func() {
		// 立即执行首次检查
		c.performHealthCheck()
		for {
			select {
			case <-c.checkTicker.C:
				c.performHealthCheck()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// performHealthCheck 执行一次健康检查（调用/health端点）。
func (c *Client) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		c.recordHealthCheckFailure(fmt.Errorf("build health request: %w", err))
		return
	}
	if c.token != "" {
		req.Header.Set("X-Sidecar-Token", c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		c.recordHealthCheckFailure(fmt.Errorf("health check failed: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.recordHealthCheckFailure(fmt.Errorf("health check status %d", resp.StatusCode))
		return
	}

	// 健康检查成功
	c.recordHealthCheckSuccess()
}

// recordHealthCheckFailure 记录健康检查失败。
func (c *Client) recordHealthCheckFailure(err error) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	c.failCount++
	c.lastCheck = time.Now()
	c.lastError = err

	// 连续3次失败标记为不健康
	if c.failCount >= 3 {
		if c.healthy {
			c.healthy = false
			log.Printf("[CRITICAL] GNN sidecar不可达，连续失败%d次: %v", c.failCount, err)
		}
	} else {
		log.Printf("[WARNING] GNN sidecar健康检查失败 (%d/3): %v", c.failCount, err)
	}
}

// recordHealthCheckSuccess 记录健康检查成功。
func (c *Client) recordHealthCheckSuccess() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	wasUnhealthy := !c.healthy
	c.healthy = true
	c.failCount = 0
	c.lastCheck = time.Now()
	c.lastError = nil

	if wasUnhealthy {
		log.Printf("[INFO] GNN sidecar已恢复健康")
	}
}

// Close 停止健康检查goroutine。
func (c *Client) Close() {
	if c.checkTicker != nil {
		c.checkTicker.Stop()
		close(c.stopCh)
	}
}

// Score 调用 sidecar /v1/score。任何失败统一包装为 ErrSidecar（保留原错误链）。
func (c *Client) Score(ctx context.Context, req ScoreRequest) (*ScoreResult, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	// 快速健康检查：不健康时直接返回，避免浪费资源
	if !c.IsHealthy() {
		return nil, fmt.Errorf("%w: sidecar unhealthy", ErrSidecar)
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
