// P4-5 行为生物特征采集 + P4-6 时钟偏移（规格书 §7）。
// 隐私边界：只上报**时序统计量**（均值/方差/计数），不含坐标原值、按键内容、
// 页面内容；采样全部异步静默，任何失败不影响站点功能。
//   - 鼠标：mousemove 节流 16ms、滑窗 500 事件 → 速度均值/方差、曲率均值、
//     jerk（加加速度）方差、方向变化率；
//   - 击键：keydown/keyup 滑窗 100 键 → dwell（按下时长）均值/方差、flight（键间）均值/方差；
//   - 时钟偏移：每 60s 采样 Date.now() - performance.now()（仅页面可见时），
//     20 点最小二乘斜率 × 1e6 = clock_skew_ppm；
//   - 上报：每 5 分钟或页面卸载（fetch keepalive）→ fp/report 仅携带 fp + 统计量。
//   - 自动化检测：集成 behavior-collector 进行实时自动化模式检测。

import { behaviorCollector } from './behavior-collector.js'

const MOUSE_WINDOW = 500
const KEY_WINDOW = 100
const SEND_INTERVAL_MS = 5 * 60 * 1000
const SKEW_INTERVAL_MS = 60 * 1000
const SKEW_POINTS = 20

let mouseBuf = [] // {x, y, t}
let keyDownAt = {} // key → down 时间戳
let dwell = [] // 按下时长样本
let flight = [] // 键间间隔样本
let skewSamples = [] // {t, offset}
let lastMouseSent = 0
let started = false

function pushMouse(x, y, t) {
  mouseBuf.push({ x, y, t })
  if (mouseBuf.length > MOUSE_WINDOW) mouseBuf.shift()
}

// ---------- 纯函数（node --test 覆盖） ----------

export function mean(xs) {
  if (!xs.length) return 0
  return xs.reduce((a, b) => a + b, 0) / xs.length
}

export function variance(xs) {
  if (xs.length < 2) return 0
  const m = mean(xs)
  return xs.reduce((a, b) => a + (b - m) * (b - m), 0) / xs.length
}

// mouseFeatures 从鼠标事件序列计算统计量（序列需按时间升序）。
export function mouseFeatures(events) {
  const speeds = [], accels = [], jerks = [], curvatures = []
  let dirChanges = 0
  for (let i = 1; i < events.length; i++) {
    const dx = events[i].x - events[i - 1].x, dy = events[i].y - events[i - 1].y
    const dt = events[i].t - events[i - 1].t
    if (dt <= 0) continue
    const v = Math.sqrt(dx * dx + dy * dy) / dt
    speeds.push(v)
    const ang = Math.atan2(dy, dx)
    if (i >= 2) {
      const pdx = events[i - 1].x - events[i - 2].x, pdy = events[i - 1].y - events[i - 2].y
      const pdt = events[i - 1].t - events[i - 2].t
      if (pdt > 0) {
        const pv = Math.sqrt(pdx * pdx + pdy * pdy) / pdt
        const pang = Math.atan2(pdy, pdx)
        let dAng = Math.abs(ang - pang)
        if (dAng > Math.PI) dAng = 2 * Math.PI - dAng
        curvatures.push(dAng / (v * dt + 1e-6)) // 单位弧度/像素
        if (dAng > Math.PI / 6) dirChanges++    // >30° 记一次方向变化
        const a = (v - pv) / dt
        accels.push(a)
        if (accels.length >= 2) {
          const a2 = accels[accels.length - 1], a1 = accels[accels.length - 2]
          const jdt = dt + pdt
          if (jdt > 0) jerks.push((a2 - a1) / jdt)
        }
      }
    }
  }
  return {
    speed_mean: r2(mean(speeds)), speed_var: r2(variance(speeds)),
    curvature_mean: r2(mean(curvatures)), jerk_var: r2(variance(jerks)),
    dir_change_rate: r2(events.length > 2 ? dirChanges / (events.length - 2) : 0),
    events: events.length,
  }
}

// keyFeatures 从 dwell/flight 样本序列计算统计量。
export function keyFeatures(dwellSamples, flightSamples) {
  return {
    dwell_mean: r2(mean(dwellSamples)), dwell_var: r2(variance(dwellSamples)),
    flight_mean: r2(mean(flightSamples)), flight_var: r2(variance(flightSamples)),
    events: Math.min(dwellSamples.length, KEY_WINDOW),
  }
}

// clockSkewPPM 最小二乘斜率 × 1e6（样本 {t 秒, offset ms}）。
export function clockSkewPPM(samples) {
  if (samples.length < 3) return null
  const ts = samples.map(s => s.t), os = samples.map(s => s.offset)
  const tm = mean(ts), om = mean(os)
  let num = 0, den = 0
  for (let i = 0; i < ts.length; i++) {
    num += (ts[i] - tm) * (os[i] - om)
    den += (ts[i] - tm) * (ts[i] - tm)
  }
  if (den === 0) return null
  return r2((num / den) * 1e6) // 斜率（ms/ms = 无量纲漂移）× 1e6 = ppm
}

function r2(v) { return Math.round(v * 1000) / 1000 }

// ---------- 采集与上报 ----------

export function initBehaviorReporting(getFp) {
  if (started) return
  started = true
  
  // 启动高级行为采集器（用于自动化检测）
  behaviorCollector.start()
  
  let lastMove = 0
  window.addEventListener('mousemove', e => {
    const now = performance.now()
    if (now - lastMove < 16) return
    lastMove = now
    pushMouse(e.clientX, e.clientY, now)
  }, { passive: true })
  window.addEventListener('keydown', e => {
    if (e.repeat) return
    keyDownAt[e.code || e.key] = performance.now()
  }, { passive: true })
  window.addEventListener('keyup', e => {
    const down = keyDownAt[e.code || e.key]
    if (down === undefined) return
    const now = performance.now()
    dwell.push(now - down)
    if (flight.length === 0 || now - down > 0) flight.push(now - down) // 简化 flight：相邻 keyup 间隔
    if (dwell.length > KEY_WINDOW) dwell.shift()
    if (flight.length > KEY_WINDOW) flight.shift()
    delete keyDownAt[e.code || e.key]
  }, { passive: true })
  // 时钟偏移：仅页面可见时采样（后台节流会造成假点，规格 P4-6 注意项）
  setInterval(() => {
    if (document.visibilityState !== 'visible') return
    skewSamples.push({ t: performance.now() / 1000, offset: Date.now() - performance.now() })
    if (skewSamples.length > SKEW_POINTS) skewSamples.shift()
  }, SKEW_INTERVAL_MS)
  // 每 5 分钟或页面隐藏/卸载时上报
  setInterval(() => maybeSend(getFp), SEND_INTERVAL_MS)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') maybeSend(getFp)
  })
  window.addEventListener('pagehide', () => maybeSend(getFp))
}

function maybeSend(getFp) {
  const fp = getFp()
  if (!fp) return
  const mouse = mouseFeatures(mouseBuf)
  const keys = keyFeatures(dwell, flight)
  const skew = clockSkewPPM(skewSamples)
  const hasMouse = mouse.events >= 20
  const hasKeys = keys.events >= 10
  if (!hasMouse && !hasKeys && skew === null) return
  
  // 获取自动化检测结果
  const automationDetection = behaviorCollector.detectAutomation()
  
  const body = JSON.stringify({
    fp,
    behavior: { mouse, keys },
    clock_skew_ppm: skew === null ? undefined : skew,
    automation: {
      detected: automationDetection.isAutomated,
      flags: automationDetection.flags,
      confidence: automationDetection.confidence
    }
  })
  try {
    fetch('/api/v1/fp/report', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body, keepalive: true,
    }).catch(() => {})
  } catch { /* 静默 */ }
}
