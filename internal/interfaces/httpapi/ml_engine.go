// ML统一调度引擎（P5-2 iForest + P5-3 LR + P6-3 GNN）。
package httpapi

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
	
	"github.com/NoraStory/GithubHot/internal/domain/fpmath"
)

// MLEngine 统一ML调度器。
type MLEngine struct {
	mu      sync.RWMutex
	// state 用独立锁：setState 会在持有 mu 的调用链（checkLRModel）里被调用，
	// 若复用 mu 则 sync.Mutex 不可重入直接死锁；Health() 与后台 run() 并发
	// 读写 state 也需要此锁保证可见性（数据竞态修复）。
	stateMu sync.RWMutex
	state   MLState
	lr      *lrLoader
	guard   *IPGuard // 反向引用，用于调用store

	// 健康度指标
	metrics MLMetrics

	// 配置
	cfg MLConfig
}

type MLState int

const (
	MLStateInit MLState = iota
	MLStateLoading
	MLStateHealthy
	MLStateDegraded
	MLStateFailed
)

type MLMetrics struct {
	IForestLastRun     time.Time
	IForestSampleCount int
	IForestDuration    time.Duration
	IForestError       error
	
	LRVersion          string
	LRAUC              float64
	LRActive           bool
	LRLoadError        error
	
	TotalInferences    int64
	TotalErrors        int64
}

type MLConfig struct {
	ModelDir            string
	IForestInterval     time.Duration
	IForestMinSamples   int
	IForestMaxSamples   int
	LRCheckInterval     time.Duration
	EnableIForest       bool
	EnableLR            bool
}

// NewMLEngine 创建ML引擎（延迟启动，等待guard注入后调用Start）。
func NewMLEngine(cfg MLConfig) *MLEngine {
	if cfg.ModelDir == "" {
		cfg.ModelDir = getEnv("ML_MODEL_DIR", "./models")
	}
	if cfg.IForestInterval == 0 {
		cfg.IForestInterval = 24 * time.Hour
	}
	if cfg.IForestMinSamples == 0 {
		cfg.IForestMinSamples = 10
	}
	if cfg.IForestMaxSamples == 0 {
		cfg.IForestMaxSamples = 10000
	}
	if cfg.LRCheckInterval == 0 {
		cfg.LRCheckInterval = 1 * time.Minute
	}
	
	return &MLEngine{
		state: MLStateInit,
		lr:    newLRLoader(cfg.ModelDir),
		cfg:   cfg,
	}
}

// Start 启动调度器（需要在guard初始化后调用，因为需要访问store）。
func (e *MLEngine) Start(guard *IPGuard) {
	e.guard = guard
	goSafe("ml-engine-run", func() { e.run() })
}

func (e *MLEngine) run() {
	e.setState(MLStateLoading)
	
	// 初始化LR模型
	if e.cfg.EnableLR {
		m := e.lr.get()
		if m != nil && m.Active {
			log.Printf("[ml-engine] LR模型加载成功: %s (AUC=%.3f)", m.Version, m.Metrics.AUC)
			e.setState(MLStateHealthy)
		} else {
			log.Printf("[ml-engine] LR模型未就绪，以降级模式运行")
			e.setState(MLStateDegraded)
		}
	} else {
		e.setState(MLStateHealthy)
	}
	
	// 首次iForest延迟1分钟执行
	if e.cfg.EnableIForest {
		time.Sleep(1 * time.Minute)
		e.runIForest(context.Background())
	}
	
	// 定时任务
	iforestTicker := time.NewTicker(e.cfg.IForestInterval)
	lrCheckTicker := time.NewTicker(e.cfg.LRCheckInterval)
	defer iforestTicker.Stop()
	defer lrCheckTicker.Stop()
	
	for {
		select {
		case <-iforestTicker.C:
			if e.cfg.EnableIForest {
				goSafe("iforest-run", func() { e.runIForest(context.Background()) })
			}
			
		case <-lrCheckTicker.C:
			if e.cfg.EnableLR {
				e.checkLRModel()
			}
		}
	}
}

func (e *MLEngine) runIForest(ctx context.Context) {
	if e.guard == nil || e.guard.store == nil {
		log.Printf("[ml-engine] iForest跳过：store未初始化")
		return
	}
	
	start := time.Now()
	defer func() {
		e.mu.Lock()
		e.metrics.IForestDuration = time.Since(start)
		e.metrics.IForestLastRun = start
		e.mu.Unlock()
	}()
	
	log.Printf("[ml-engine] iForest任务开始...")
	
	// 调用IPGuard的RefreshAnomalyScores
	if err := e.guard.RefreshAnomalyScores(ctx); err != nil {
		e.mu.Lock()
		e.metrics.IForestError = err
		e.mu.Unlock()
		log.Printf("[ml-engine] iForest任务失败: %v", err)
		return
	}
	
	log.Printf("[ml-engine] iForest任务完成，耗时 %v", time.Since(start))
}

func (e *MLEngine) checkLRModel() {
	m := e.lr.get()
	
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if m != nil {
		e.metrics.LRVersion = m.Version
		e.metrics.LRAUC = m.Metrics.AUC
		e.metrics.LRActive = m.Active
		e.metrics.LRLoadError = nil
		
		// 模型恢复健康
		if st := e.getState(); st == MLStateDegraded || st == MLStateFailed {
			e.setState(MLStateHealthy)
			log.Printf("[ml-engine] LR模型恢复: %s", m.Version)
		}
	} else {
		e.metrics.LRLoadError = e.lr.lastError

		// 降级
		if e.getState() == MLStateHealthy {
			e.setState(MLStateDegraded)
			log.Printf("[ml-engine] LR模型不可用，降级运行")
		}
	}
}

func (e *MLEngine) setState(s MLState) {
	e.stateMu.Lock()
	e.state = s
	e.stateMu.Unlock()
}

// getState 并发安全读取引擎状态。
func (e *MLEngine) getState() MLState {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	return e.state
}

func stateString(s MLState) string {
	switch s {
	case MLStateInit:
		return "INIT"
	case MLStateLoading:
		return "LOADING"
	case MLStateHealthy:
		return "HEALTHY"
	case MLStateDegraded:
		return "DEGRADED"
	case MLStateFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// Health 健康检查接口。
func (e *MLEngine) Health() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	var iforestError string
	if e.metrics.IForestError != nil {
		iforestError = e.metrics.IForestError.Error()
	}
	
	return map[string]interface{}{
		"state": stateString(e.getState()),
		"iforest": map[string]interface{}{
			"enabled":      e.cfg.EnableIForest,
			"last_run":     e.metrics.IForestLastRun.Format(time.RFC3339),
			"sample_count": e.metrics.IForestSampleCount,
			"duration_ms":  e.metrics.IForestDuration.Milliseconds(),
			"error":        iforestError,
		},
		"lr": e.lr.HealthStatus(),
		"metrics": map[string]interface{}{
			"total_inferences": e.metrics.TotalInferences,
			"total_errors":     e.metrics.TotalErrors,
		},
	}
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetLRModel 获取LR模型（供scoreBehaviorML调用）。
func (e *MLEngine) GetLRModel() *fpmath.LRModel {
	if e.lr == nil {
		return nil
	}
	return e.lr.get()
}

// RecordInference 记录推理指标。
func (e *MLEngine) RecordInference(success bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.metrics.TotalInferences++
	if !success {
		e.metrics.TotalErrors++
	}
}

// HealthStatus 健康状态（Health方法的别名，用于兼容）。
func (e *MLEngine) HealthStatus() map[string]interface{} {
	return e.Health()
}

// RefreshIForestNow 手动触发iForest。
func (e *MLEngine) RefreshIForestNow(ctx context.Context) error {
	if !e.cfg.EnableIForest {
		return fmt.Errorf("iForest未启用")
	}
	// 后台任务用独立 context：调用方（未来可能是 handler）的 ctx 会随响应
	// 结束被取消，长跑的 iForest 任务会被半途掐断
	goSafe("iforest-run", func() { e.runIForest(context.Background()) })
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
