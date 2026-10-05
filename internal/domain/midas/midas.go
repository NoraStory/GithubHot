// Package midas — P6-4 流式边异常检测（规格书 §9 P6-4，WSDM 2020 MIDAS 简化实现）。
//
// 双 count-min sketch（当前时间片 / 历史累计），对边 (ip→fp) 算卡方型统计量
// (a−s)²/max(1,s)：a = 当前片该边桶计数（count-min 碰撞天然放大协同突发），
// s = 历史累计桶计数（增量前）。新边 s=0 → score = a²（fresh 突发即高分）；
// 稳态重复边 a≈s → score ≈ 0。时间片到期把 cur 并入 total 并清零。
//
// sketch 宽度决定协同检测灵敏度：宽度小 → 400 条新边碰撞堆桶 → a 抬升 → 告警；
// 宽度大 → 无碰撞 → a=1 → score=1（低于告警线）。默认 256 桶（抓百级突发）。
// 纯内存实现（随一层防护重启丢失，可接受——规格允许）。
package midas

import (
	"hash/fnv"
	"math"
	"sync"
	"time"
)

// Detector MIDAS 检测器。
type Detector struct {
	mu           sync.Mutex
	depth        int
	width        uint32
	cur          [][]uint32 // 当前时间片计数（count-min）
	total        [][]uint32 // 历史累计计数（count-min）
	curTotal     int        // 当前片边事件总数
	sliceStart   time.Time
	sliceTTL     time.Duration
	historyTotal int        // 历史片边事件总数（0 = 冷启动第一片，跳过评分）
}

// Options 检测器参数。
type Options struct {
	SliceTTL  time.Duration // 时间片长度（默认 60s）
	Depth     int           // sketch 深度（默认 3）
	WidthBits int           // sketch 宽度 2^WidthBits（默认 8 = 256 桶，抓百级突发）
}

// New 构造检测器。
func New(o Options) *Detector {
	if o.SliceTTL <= 0 {
		o.SliceTTL = 60 * time.Second
	}
	if o.Depth <= 0 {
		o.Depth = 3
	}
	if o.WidthBits <= 0 || o.WidthBits > 20 {
		o.WidthBits = 8
	}
	w := uint32(1) << o.WidthBits
	d := &Detector{
		depth:      o.Depth,
		width:      w,
		cur:        make([][]uint32, o.Depth),
		total:      make([][]uint32, o.Depth),
		sliceTTL:   o.SliceTTL,
	}
	for i := range d.cur {
		d.cur[i] = make([]uint32, w)
		d.total[i] = make([]uint32, w)
	}
	return d
}

func (d *Detector) hash(src, dst string) (h1, h2 uint32) {
	f := fnv.New64a()
	_, _ = f.Write([]byte(src))
	_, _ = f.Write([]byte{0})
	_, _ = f.Write([]byte(dst))
	sum := f.Sum64()
	return uint32(sum), uint32(sum >> 32)
}

// rollSlice 时间片到期：cur 并入 total 并清零。
// sliceStart 为零值时以第一条观测校准（测试可传过去的时间戳驱动片滚动）。
func (d *Detector) rollSlice(now time.Time) {
	if d.sliceStart.IsZero() {
		d.sliceStart = now
		return
	}
	if now.Sub(d.sliceStart) < d.sliceTTL {
		return
	}
	for i := 0; i < d.depth; i++ {
		for j, v := range d.cur[i] {
			d.total[i][j] += v
			d.cur[i][j] = 0
		}
	}
	d.historyTotal += d.curTotal
	d.curTotal = 0
	d.sliceStart = now
}

// Observe 记录一次边出现（ip→fp），返回归一化异常分：
//   score = max over sketch depths of (a−s)²/max(1,s)
//   a = 当前片该桶计数（本边增量后），s = 历史累计该桶计数（增量前）。
// 返回值 ≥ 3 视为告警候选（规格书：>3σ 积分、>5σ 入 review）。
func (d *Detector) Observe(src, dst string, now time.Time) float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rollSlice(now)

	d.curTotal++
	h1, h2 := d.hash(src, dst)
	maxScore := 0.0
	for i := 0; i < d.depth; i++ {
		pos := (h1 + uint32(i)*h2) % d.width
		d.cur[i][pos]++  // a：当前片计数（含本边）
		s := float64(d.total[i][pos]) // s：历史累计（不含本时间片）
		a := float64(d.cur[i][pos])
		// 冷启动：第一片 historyTotal=0，没有基线可比较——递增 sketch 但不打分
		if d.historyTotal == 0 {
			continue
		}
		// 只计正偏差（a > s）：负偏差 = 正常回访（历史基线高、当前低），不是涌现
		if a <= s {
			continue
		}
		score := (a - s) * (a - s) / math.Max(1, s)
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// DebugHash 暴露哈希定位（测试诊断用）。
func (d *Detector) DebugHash(src, dst string, depth int) uint32 {
	h1, h2 := d.hash(src, dst)
	return (h1 + uint32(depth)*h2) % d.width
}

// DebugBin 返回指定深度的桶计数（测试诊断用）。
func (d *Detector) DebugBin(depth int, pos uint32) (cur, total uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if depth >= d.depth || int(pos) >= len(d.cur[depth]) {
		return 0, 0
	}
	return d.cur[depth][pos], d.total[depth][pos]
}

// DebugSliceState 当前片状态（测试诊断用）。
func (d *Detector) DebugSliceState() (curTotal, histTotal int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.curTotal, d.historyTotal
}

// CurTotal 当前时间片总边数（诊断/测试用）。
func (d *Detector) CurTotal() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.curTotal
}
