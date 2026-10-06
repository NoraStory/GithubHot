// P5-3 逻辑回归前向推理（规格书 §8 P5-3）：标准化 + 点积 + sigmoid，零依赖。
// 模型文件 {version, feature_names[], mu[], sigma[], weights[], bias, metrics, active}
// 由 scripts/ml/train_behavior.py 产出；Go 与 Python predict_proba 对拍误差 < 1e-9。
// 行为特征向量顺序 = behavior.FeatureVector（P4-5），与 FEATURE_NAMES 严格对齐。
package fpmath

import (
	"encoding/json"
	"fmt"
	"math"
)

// LRModel 已加载的 LR 模型。
type LRModel struct {
	Version      string    `json:"version"`
	FeatureNames []string  `json:"feature_names"`
	Mu           []float64 `json:"mu"`
	Sigma        []float64 `json:"sigma"`
	Weights      []float64 `json:"weights"`
	Bias         float64   `json:"bias"`
	Active       bool      `json:"active"`
	Metrics      struct {
		AUC float64 `json:"auc"`
		FPR float64 `json:"fpr"`
	} `json:"metrics"`
}

// Predict 单样本前向：z = Σ w_i·(x_i−μ_i)/σ_i + b；p = sigmoid(z)。
// x 与模型 weights 维度一致；σ_i 为 0 时按 1 处理（防除零）。
func (m *LRModel) Predict(x []float64) float64 {
	if m == nil || !m.Active || len(x) != len(m.Weights) {
		return -1 // 调用方约定：<0 = 模型不可用
	}
	z := m.Bias
	for i, w := range m.Weights {
		s := m.Sigma[i]
		if s == 0 {
			s = 1
		}
		z += w * (x[i] - m.Mu[i]) / s
	}
	return sigmoid(z)
}

func sigmoid(z float64) float64 { return 1 / (1 + math.Exp(-z)) }

// Validate 验证模型参数的合法性。
func (m *LRModel) Validate() error {
	if m.Version == "" {
		return fmt.Errorf("模型版本号不能为空")
	}
	
	if len(m.Weights) == 0 {
		return fmt.Errorf("模型权重为空")
	}
	
	// 检查维度一致性
	if len(m.Mu) != len(m.Weights) {
		return fmt.Errorf("Mu维度(%d)与Weights维度(%d)不一致", len(m.Mu), len(m.Weights))
	}
	if len(m.Sigma) != len(m.Weights) {
		return fmt.Errorf("Sigma维度(%d)与Weights维度(%d)不一致", len(m.Sigma), len(m.Weights))
	}
	
	// 检查权重是否包含NaN或Inf
	for i, w := range m.Weights {
		if math.IsNaN(w) || math.IsInf(w, 0) {
			return fmt.Errorf("权重[%d]包含非法值: %v", i, w)
		}
	}
	
	if math.IsNaN(m.Bias) || math.IsInf(m.Bias, 0) {
		return fmt.Errorf("Bias包含非法值: %v", m.Bias)
	}
	
	// 验证性能指标
	if m.Metrics.AUC < 0 || m.Metrics.AUC > 1 {
		return fmt.Errorf("AUC值超出范围[0,1]: %v", m.Metrics.AUC)
	}
	
	if m.Metrics.FPR < 0 || m.Metrics.FPR > 1 {
		return fmt.Errorf("FPR值超出范围[0,1]: %v", m.Metrics.FPR)
	}
	
	return nil
}

// LoadFromJSON 从JSON字节数组加载模型。
func LoadFromJSON(data []byte) (*LRModel, error) {
	var model LRModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}
	
	if err := model.Validate(); err != nil {
		return nil, fmt.Errorf("模型验证失败: %w", err)
	}
	
	return &model, nil
}
