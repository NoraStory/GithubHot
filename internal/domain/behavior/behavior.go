// Package behavior — P4-5 行为生物特征的服务端规则判定（规格书 §7 P4-5 规则版）。
//
// 客户端（behavior.js）在滑窗内统计鼠标/击键特征并随 fp/report 上报 JSON；本包只做：
//   - 形状校验与清洗（事件数下限、数值范围）；
//   - 机器特征规则：速度方差 == 0 / 曲率均值 == 0 / 按键 dwell 方差 == 0（有数据为前提）
//     → behavior_machine +15（灰度受 FP_SCORE_SHADOW 控制）；
//   - 余弦相似度（P4-4 图聚类边：跨指纹 behavior 相似 > 0.9）。
// 纯函数，零 IO。
package behavior

import (
	"encoding/json"
	"math"
)

// MouseFeatures 鼠标滑窗统计量。
type MouseFeatures struct {
	SpeedMean      float64 `json:"speed_mean"`
	SpeedVar       float64 `json:"speed_var"`
	CurvatureMean  float64 `json:"curvature_mean"`
	JerkVar        float64 `json:"jerk_var"`
	DirChangeRate  float64 `json:"dir_change_rate"`
	Events         int     `json:"events"`
}

// KeyFeatures 击键滑窗统计量。
type KeyFeatures struct {
	DwellMean float64 `json:"dwell_mean"`
	DwellVar  float64 `json:"dwell_var"`
	FlightMean float64 `json:"flight_mean"`
	FlightVar float64 `json:"flight_var"`
	Events    int     `json:"events"`
}

// Features fp/report 的 behavior 字段。
type Features struct {
	Mouse MouseFeatures `json:"mouse"`
	Keys  KeyFeatures   `json:"keys"`
}

const (
	minMouseEvents = 20 // 少于该事件数视为无有效行为数据（headless 无鼠标事件 ≠ 机器）
	minKeyEvents   = 10
	// maxEvents 防伪造超大窗口
	maxMouseEvents = 5000
	maxKeyEvents   = 2000
)

// Parse 清洗上报的 behavior JSON：结构合法 + 事件数与数值范围合理。非法返回 nil。
func Parse(raw string) *Features {
	if raw == "" || len(raw) > 8192 {
		return nil
	}
	var f Features
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return nil
	}
	if f.Mouse.Events < 0 || f.Mouse.Events > maxMouseEvents || f.Keys.Events < 0 || f.Keys.Events > maxKeyEvents {
		return nil
	}
	if !sane(f.Mouse.SpeedMean) || !sane(f.Mouse.SpeedVar) || !sane(f.Mouse.CurvatureMean) ||
		!sane(f.Mouse.JerkVar) || !sane(f.Mouse.DirChangeRate) || !sane(f.Keys.DwellMean) ||
		!sane(f.Keys.DwellVar) || !sane(f.Keys.FlightMean) || !sane(f.Keys.FlightVar) {
		return nil
	}
	return &f
}

func sane(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) < 1e12 }

// MachineSignals 机器特征规则（规格书 P4-5 规则版）：有数据的前提下，
// 完全匀速 / 纯直线 / 机械击键 → 行为机器。返回命中的信号名（空 = 通过）。
func MachineSignals(f *Features) []string {
	if f == nil {
		return nil
	}
	signals := []string{}
	if f.Mouse.Events >= minMouseEvents {
		if f.Mouse.SpeedVar == 0 {
			signals = append(signals, "speed_var_zero") // 完全匀速
		}
		if f.Mouse.CurvatureMean == 0 {
			signals = append(signals, "curvature_zero") // 纯直线
		}
	}
	if f.Keys.Events >= minKeyEvents && f.Keys.DwellVar == 0 {
		signals = append(signals, "dwell_var_zero") // 机械击键
	}
	return signals
}

// CosineSimilarity 向量余弦（长度取短者对齐；空向量返回 0）。
func CosineSimilarity(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// FeatureVector 把特征压成固定顺序向量（P4-4 图聚类边与 P5 导出共用）。
func FeatureVector(f *Features) []float64 {
	if f == nil {
		return nil
	}
	return []float64{
		f.Mouse.SpeedMean, f.Mouse.SpeedVar, f.Mouse.CurvatureMean, f.Mouse.JerkVar,
		f.Mouse.DirChangeRate, float64(f.Mouse.Events),
		f.Keys.DwellMean, f.Keys.DwellVar, f.Keys.FlightMean, f.Keys.FlightVar,
		float64(f.Keys.Events),
	}
}
