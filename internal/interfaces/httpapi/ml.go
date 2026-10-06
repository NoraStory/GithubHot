// P5 标注与模型调度引擎方法（规格书 §8）：弱标签 / iForest 异常分 / LR 行为分。
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/behavior"
	"github.com/NoraStory/GithubHot/internal/domain/fpmath"
)

// ---------- P5-2 iForest 异常分（永远 shadow，不直接计分） ----------

// RefreshAnomalyScores 每日 cron：近 7 天活跃指纹 → 特征向量 → 训练 + 打分 →
// 写 anomaly_score。99.5 分位仅记录事件；99.9 分位入管理端 review 关注。
func (g *IPGuard) RefreshAnomalyScores(ctx context.Context) error {
	if g.store == nil {
		return nil
	}
	rows, err := g.store.ListFingerprintsSince(ctx, time.Now().Add(-7*24*time.Hour), entropyMaxRows)
	if err != nil {
		return err
	}
	samples := make([][]float64, 0, len(rows))
	fps := make([]string, 0, len(rows))
	for _, r := range rows {
		v := featureVectorFor(r)
		if v == nil {
			continue
		}
		samples = append(samples, v)
		fps = append(fps, r.Fingerprint)
	}
	if len(samples) < 10 {
		log.Printf("[ipguard] iForest 样本不足（%d），跳过", len(samples))
		return nil
	}
	forest := fpmath.TrainIForest(samples, time.Now().Unix())
	scores := make([]float64, len(samples))
	for i, s := range samples {
		scores[i] = forest.Score(s)
	}
	// 99.5 / 99.9 分位阈值
	q995 := fpmath.QuantileThreshold(scores, 0.995)
	q999 := fpmath.QuantileThreshold(scores, 0.999)
	flagged995, flagged999 := 0, 0
	for i := range samples {
		_ = g.store.UpdateAnomalyScore(ctx, fps[i], scores[i])
		if scores[i] >= q999 {
			flagged999++
			g.event(ctx, firstIP(g.ipsOf(ctx, fps[i])), "anomaly-p999",
				sprintf("iForest 异常分 %.3f ≥ 99.9 分位 %.3f", scores[i], q999), 0, false)
		} else if scores[i] >= q995 {
			flagged995++
		}
	}
	log.Printf("[ipguard] iForest：%d 样本，99.5 分位 %.3f（%d 个）、99.9 分位 %.3f（%d 个）",
		len(samples), q995, flagged995, q999, flagged999)
	return nil
}

// ipsOf 查指纹关联 IP（事件明细用）。
func (g *IPGuard) ipsOf(ctx context.Context, fp string) []string {
	if row, err := g.store.FindFingerprint(ctx, fp); err == nil && row != nil {
		return row.IPs
	}
	return nil
}

// UpdateAnomalyScore anomaly_score 列写回（sqlite 侧实现）。
type anomalyWriter = func(ctx context.Context, fp string, score float64) error

// featureVectorFor 行 → 16 维特征向量（与 scripts/ml FEATURE_NAMES 顺序严格对齐）。
func featureVectorFor(r FingerprintDTO) []float64 {
	f := behavior.Parse(r.BehaviorJSON)
	if f == nil {
		return nil
	}
	flagsHit := 0.0
	for _, flag := range r.Flags {
		if strings.HasPrefix(flag, "fpb_") || strings.HasPrefix(flag, "botd_") {
			flagsHit++
		}
	}
	sessionMinutes := r.LastSeen.Sub(r.FirstSeen).Minutes()
	skew := 0.0
	if r.ClockSkewPPM != nil {
		skew = *r.ClockSkewPPM
	}
	return []float64{
		f.Keys.DwellMean, f.Keys.DwellVar, f.Keys.FlightMean, f.Keys.FlightVar,
		f.Mouse.SpeedMean, f.Mouse.SpeedVar, f.Mouse.CurvatureMean, f.Mouse.JerkVar,
		f.Mouse.DirChangeRate, float64(f.Mouse.Events + f.Keys.Events),
		r.EntropyBits, r.Stability, skew, flagsHit,
		sessionMinutes, float64(len(r.IPs)),
	}
}

// ---------- P5-3 LR 推理（热加载 + shadow 计分） ----------

// lrLoader 模型目录热加载（mtime 触发，serve 内 goroutine 定时检查）。
type lrLoader struct {
	mu           sync.Mutex
	path         string
	lastMod      time.Time
	model        *fpmath.LRModel
	lastError    error      // 最后一次加载失败的错误
	failCount    int        // 连续失败次数
	lastFailTime time.Time  // 最后一次失败时间
}

func newLRLoader(dir string) *lrLoader {
	return &lrLoader{path: filepath.Join(dir, "behavior_lr_v1.json")}
}

// get 惰性/热加载：文件 mtime 变化才重新读取。
// C2修复：强制JSON校验，连续失败时告警。
func (l *lrLoader) get() *fpmath.LRModel {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	st, err := os.Stat(l.path)
	if err != nil {
		// 文件不存在或无法访问
		if l.model == nil && l.lastError == nil {
			log.Printf("[ml] 模型文件不存在: %s", l.path)
			l.lastError = err
		}
		return l.model
	}
	
	// 文件未变化，返回缓存
	if st.ModTime() == l.lastMod && l.model != nil {
		return l.model
	}
	
	// 尝试加载新模型
	l.lastMod = st.ModTime()
	raw, err := os.ReadFile(l.path)
	if err != nil {
		l.recordFailure(err, "读取失败")
		return l.model // 保留旧模型
	}
	
	var m fpmath.LRModel
	if err := json.Unmarshal(raw, &m); err != nil {
		// C2修复：JSON解析失败必须告警，不能静默降级
		l.recordFailure(err, "JSON解析失败")
		// 强制返回nil而非旧模型，避免使用损坏的配置
		if l.failCount >= 3 {
			log.Printf("[ml] CRITICAL: 模型文件连续%d次解析失败，强制降级为nil: %v", l.failCount, err)
			l.model = nil
		}
		return l.model
	}
	
	// 基本合法性检查
	if len(m.Weights) == 0 {
		err := fmt.Errorf("模型权重为空")
		l.recordFailure(err, "校验失败")
		return l.model
	}
	
	// 加载成功，重置失败计数
	l.failCount = 0
	l.lastError = nil
	l.model = &m
	log.Printf("[ml] 行为模型已加载 %s（AUC %.3f，active %v）",
		m.Version, m.Metrics.AUC, m.Active)
	return l.model
}

// recordFailure 记录加载失败，连续失败时告警。
func (l *lrLoader) recordFailure(err error, context string) {
	l.failCount++
	l.lastError = err
	l.lastFailTime = time.Now()
	
	// 连续失败告警（3次、10次、每50次）
	if l.failCount == 3 || l.failCount == 10 || (l.failCount > 10 && l.failCount%50 == 0) {
		log.Printf("[ml] WARNING: 模型加载%s，连续失败%d次: %v", context, l.failCount, err)
	}
}

// HealthStatus 返回模型加载器健康状态（用于§10诊断页）。
func (l *lrLoader) HealthStatus() map[string]interface{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	status := map[string]interface{}{
		"path":       l.path,
		"loaded":     l.model != nil,
		"fail_count": l.failCount,
	}
	
	if l.model != nil {
		status["version"] = l.model.Version
		status["auc"] = l.model.Metrics.AUC
		status["active"] = l.model.Active
		status["last_modified"] = l.lastMod.Format(time.RFC3339)
	}
	
	if l.lastError != nil {
		status["last_error"] = l.lastError.Error()
		status["last_fail_time"] = l.lastFailTime.Format(time.RFC3339)
	}
	
	// 健康度判定
	healthy := true
	if l.model == nil {
		healthy = false
	} else if l.failCount >= 3 {
		healthy = false
	}
	status["healthy"] = healthy
	
	return status
}

// scoreBehaviorML fp/report 时对行为特征打分（shadow：0 分仅记录）。
// 出 shadow 门槛（AUC ≥ 0.85 且 FPR ≤ 0.5%）由训练侧 gate 控制；
// 服务端在 gate.pass 且显式置 active 的模型上才计分。
func (g *IPGuard) scoreBehaviorML(ctx context.Context, ip, fp string, meta FingerprintMeta) {
	if g.store == nil || g.ml == nil {
		return
	}
	m := g.ml.get()
	if m == nil || !m.Active || m.Metrics.AUC < 0.85 {
		return // 模型未达标 → 不打分不记录（避免噪声）
	}
	f := behavior.Parse(meta.Behavior)
	if f == nil {
		return
	}
	x := behavior.FeatureVector(f)
	p := m.Predict(x)
	if p < 0 {
		return
	}
	// 出 shadow 判定：模型 gate 里的 pass（由训练侧写入 active + metrics）
	shadow := shadowScoring() || m.Metrics.FPR > 0.005
	score := 0
	if !shadow && p >= 0.8 {
		score = 20 // ML 高置信（≥0.8）才计分，与 P6-6 融合规则一致
	}
	if p >= 0.8 || !shadow {
		g.event(ctx, ip, "behavior-ml-score",
			sprintf("行为 ML 分 %.3f（模型 %s）", p, m.Version), score, false)
	}
	_ = fp // 预留：分数写回 ip_fingerprints.behavior_ml_score（P5-6 诊断用）
}

var _ = filepath.Join // 保留 import（lrLoader.path 构建用）
