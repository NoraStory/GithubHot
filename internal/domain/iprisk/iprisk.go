// Package iprisk 违规积分封禁决策（纯算法，零 IO、零依赖）。
//
// 设计目标：**结构性防误封**——让任何单一证据在默认参数下无法单独触达封禁阈值，
// 同时保留对真实滥用与多证据互证的判定能力。五条机制：
//
//  1. 证据分级：每个违规类型（kind）归属一个大类，并区分**强证据**（身份令牌 /
//     设备指纹 / APP 签名——绑定设备而非出口）与**弱证据**（速率 / 协议 / 客户端自报
//     环境核验——会被共享出口分摊）。
//  2. 单证据上限：每种违规类型在决策中的贡献封顶（弱 40 / 强 70，均 < 阈值），
//     因此"同一条规则反复计分"无法把人磨到封禁线（旧实现的误封主因）。
//  3. 时间半衰期：事件按 0.5^(age/半衰期) 衰减（半衰期 10 分钟，统计窗口 40 分钟），
//     取代"固定窗口线性求和"。
//  4. 共享出口稀释：同一 IP 窗口内 UA 种类多 = 多人共用出口（公司网络 / 机场 WiFi /
//     运营商 CGNAT），**仅对弱证据**按 3/UA种类 稀释（下限 0.4）。
//  5. 持续滥用通道：单一违规类型的稀释后原始分越线即可单独封禁——弱证据需 1.5×阈值
//     （150）以抵消"人多势众"的误伤，强证据一倍阈值（100）即可。这样稀释保护的是
//     "被分摊的噪声"，而不是"独自持续胡作非为的人"。
//
// 判定顺序：持续通道 → 逐类型封顶累加 → 有效分 ≥ 阈值且**不同违规类型 ≥ 2** 才封。
//
// 与旧实现的差异（关键）：旧实现所有事件按 IP 线性求和 ≥100 即封——一条 rate 规则
// 每 5 分钟 +30 分，20 分钟就能封掉整条 CGNAT 出口；一条 headless-ua 被标 severe
// 立即封 7 天。新实现让这些单信号只能"计分观察"，必须互证或持续到更高门槛才封。
package iprisk

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// 决策参数（命名常量，便于回归与调参）
const (
	Threshold = 100.0 // 封禁阈值（有效分）

	HalfLifeMin    = 10.0 // 时间半衰期（分钟）
	DecayWindowMin = 40.0 // 只统计近 40 分钟事件（4 个半衰期，更早贡献 < 7%）

	UADiversityFree = 3   // UA 种类 ≤ 3 视为单一用户，不稀释
	DilutionFloor   = 0.4 // 稀释下限

	WeakSoloFactor = 1.5 // 弱证据单类封禁需 1.5× 阈值（150）

	CapWeak   = 40.0 // 弱证据单类型贡献上限
	CapStrong = 70.0 // 强证据单类型贡献上限
)

// 证据大类
const (
	ClassRate     = "R" // 速率/流量：弱（NAT/CGNAT 聚合大量正常用户）
	ClassProtocol = "P" // 扫描器/机器 UA/管理端探测：弱
	ClassEnv      = "E" // 客户端自报环境核验：弱（可伪造且会被出口分摊）
	ClassIdentity = "I" // 身份令牌（伪造/漂移/设备不符/爆破）：强
	ClassDevice   = "F" // 设备指纹劣迹（连坐/漂移）：强
	ClassApp      = "A" // APP 签名与自检：强
)

var strongClasses = map[string]bool{ClassIdentity: true, ClassDevice: true, ClassApp: true}

func dilutedClasses() map[string]bool {
	return map[string]bool{ClassRate: true, ClassProtocol: true, ClassEnv: true}
}

// Event 一条违规事件。
type Event struct {
	Kind  string
	Score int
	At    time.Time
}

// KindResult 单个违规类型的核算结果。
type KindResult struct {
	Kind  string  `json:"kind"`
	Class string  `json:"class"`
	Raw   float64 `json:"raw"`   // 衰减后原始分
	Score float64 `json:"score"` // 计入决策的分（稀释 + 上限后；持续通道时不受上限）
	Count int     `json:"count"`
}

// Decision 决策结果。
type Decision struct {
	Ban         bool         `json:"ban"`
	Effective   float64      `json:"effective"`
	Kinds       int          `json:"kinds"` // 不同违规类型数（互证依据）
	Dilution    float64      `json:"dilution"`
	UADiversity int          `json:"ua_diversity"`
	ByKind      []KindResult `json:"by_kind"`
	Reason      string       `json:"reason"`
}

// ClassOf 违规类型 → 证据大类。未知类型归入最弱类（无法单独致封）。
func ClassOf(kind string) string {
	switch kind {
	case "rate", "traffic-attack", "fp-flood":
		// fp-flood（新身份农场）：归速率弱类，随共享出口 UA 稀释——办公/校园
		// 多 UA 场景不误伤，农场单 UA 不稀释可与他类互证。
		return ClassRate
	case "scanner", "bot-ua", "admin-probe":
		return ClassProtocol
	case "env-flag":
		return ClassEnv
	case "id-forgery", "id-token-stale", "id-ip-drift", "device-mismatch", "admin-brute",
		"admin-session-ip-mismatch":
		return ClassIdentity
	case "fp-linked", "fp-linked-watch", "fp-churn", "ban-evasion":
		// ban-evasion（封禁设备签名命中）：强类设备劣迹。
		return ClassDevice
	}
	if strings.HasPrefix(kind, "app-") {
		return ClassApp
	}
	// 环境核验按 flag 细分 kind（env-flag:headless-ua 等）：既让不同 flag 可互证，
	// 也让 5 分钟去重按 flag 粒度生效（否则所有 env 证据被压成一条）
	if strings.HasPrefix(kind, "env-flag") {
		return ClassEnv
	}
	return ClassProtocol
}

// IsStrong 是否强证据类。
func IsStrong(class string) bool { return strongClasses[class] }

// dilution 共享出口稀释系数。
func dilution(uaDiversity int) float64 {
	if uaDiversity <= UADiversityFree {
		return 1
	}
	d := float64(UADiversityFree) / float64(uaDiversity)
	if d < DilutionFloor {
		return DilutionFloor
	}
	return d
}

// Evaluate 计算决策：给定该 IP 近期事件、当前时间与该 IP 的 UA 种类数。
func Evaluate(events []Event, now time.Time, uaDiversity int) Decision {
	dil := dilution(uaDiversity)
	agg := map[string]*KindResult{}
	order := make([]string, 0, len(events))

	for _, e := range events {
		if e.Score <= 0 {
			continue // 0 分事件（如"同设备换网络宽限"）只记录，不参与决策
		}
		age := now.Sub(e.At)
		if age < 0 {
			age = 0
		}
		if age.Minutes() > DecayWindowMin {
			continue
		}
		class := ClassOf(e.Kind)
		r := agg[e.Kind]
		if r == nil {
			r = &KindResult{Kind: e.Kind, Class: class}
			agg[e.Kind] = r
			order = append(order, e.Kind)
		}
		r.Raw += float64(e.Score) * math.Pow(0.5, age.Minutes()/HalfLifeMin)
		r.Count++
	}

	diluted := dilutedClasses()

	// ① 持续滥用通道：单类型稀释后原始分越线 → 单独封禁
	for _, k := range order {
		r := agg[k]
		adj := r.Raw
		if diluted[r.Class] {
			adj *= dil
		}
		line := Threshold
		if !IsStrong(r.Class) {
			line = Threshold * WeakSoloFactor
		}
		if adj >= line {
			r.Score = adj
			return Decision{
				Ban: true, Effective: adj, Kinds: 1, Dilution: dil,
				UADiversity: uaDiversity, ByKind: sortedBy(order, agg),
				Reason: fmt.Sprintf("持续性滥用：%s（%s类）累计 %.0f 分 / %d 次，越单类门槛 %.0f",
					r.Kind, r.Class, adj, r.Count, line),
			}
		}
	}

	// ② 常规通道：逐类型封顶累加，需不同类型 ≥ 2 互证
	eff := 0.0
	kinds := 0
	for _, k := range order {
		r := agg[k]
		adj := r.Raw
		if diluted[r.Class] {
			adj *= dil
		}
		cap := CapWeak
		if IsStrong(r.Class) {
			cap = CapStrong
		}
		if adj > cap {
			adj = cap
		}
		r.Score = adj
		eff += adj
		if adj > 0 {
			kinds++
		}
	}
	d := Decision{
		Effective: eff, Kinds: kinds, Dilution: dil,
		UADiversity: uaDiversity, ByKind: sortedBy(order, agg),
	}
	if eff >= Threshold && kinds >= 2 {
		d.Ban = true
		d.Reason = fmt.Sprintf("多证据互证：有效分 %.0f（%d 种类型：%s）", eff, kinds, kindSummary(order, agg))
	}
	return d
}

func sortedBy(order []string, m map[string]*KindResult) []KindResult {
	out := make([]KindResult, 0, len(m))
	for _, k := range order {
		out = append(out, *m[k])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func kindSummary(order []string, m map[string]*KindResult) string {
	parts := make([]string, 0, len(m))
	for _, k := range order {
		r := m[k]
		parts = append(parts, fmt.Sprintf("%s=%.0f", r.Kind, r.Score))
	}
	return strings.Join(parts, " ")
}
