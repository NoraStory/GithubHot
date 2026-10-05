// P4-5/P4-6 采集纯函数：鼠标/击键统计量、时钟偏移 ppm。
import test from 'node:test'
import assert from 'node:assert/strict'
import { mouseFeatures, keyFeatures, clockSkewPPM } from './behavior.js'

test('mouseFeatures：机械匀速直线运动 → 速度方差 0、曲率 0（规则命中前提）', () => {
  // 完美匀速直线：等距等时同方向
  const events = []
  for (let i = 0; i < 30; i++) events.push({ x: i * 10, y: 0, t: i * 16 })
  const f = mouseFeatures(events)
  assert.equal(f.speed_var, 0, '匀速 → 速度方差 0')
  assert.equal(f.curvature_mean, 0, '直线 → 曲率 0')
  assert.equal(f.events, 30)
})

test('mouseFeatures：真实抖动轨迹 → 方差与曲率非零', () => {
  const events = []
  let t = 0
  for (let i = 0; i < 60; i++) {
    t += 8 + ((i * i) % 23)          // 不规则时间步
    events.push({
      x: ((i * i * 7) % 97) * 3,     // 非周期横坐标
      y: ((i * 13) % 53) * 2 - 40,   // 非周期纵坐标
      t,
    })
  }
  const f = mouseFeatures(events)
  assert.ok(f.speed_var > 0, '速度方差应 > 0')
  assert.ok(f.curvature_mean > 0, '曲率均值应 > 0')
  assert.ok(f.jerk_var > 0, 'jerk 方差应 > 0')
})

test('keyFeatures：等长按等间隔（机械）→ dwell 方差 0', () => {
  const dwell = Array.from({ length: 20 }, (_, i) => 90)
  const flight = Array.from({ length: 20 }, (_, i) => 110)
  const f = keyFeatures(dwell, flight)
  assert.equal(f.dwell_var, 0)
  assert.equal(f.events, 20)
  // 人类击键：时长有波动 → 方差 > 0
  const human = Array.from({ length: 20 }, (_, i) => 80 + (i % 4) * 13)
  assert.ok(keyFeatures(human, flight).dwell_var > 0)
})

test('clockSkewPPM：固定漂移 → ppm = 斜率×1e6；样本不足 → null', () => {
  // 每 60s 偏移固定漂 3ms → 斜率 0.05 ms/s → 50000 ppm
  const samples = []
  for (let i = 0; i < 20; i++) samples.push({ t: i * 60, offset: 100 + i * 3 })
  const ppm = clockSkewPPM(samples)
  assert.ok(ppm !== null && Math.abs(ppm - 50000) < 1, `应为 50000ppm 附近，实际 ${ppm}`)
  assert.equal(clockSkewPPM([{ t: 0, offset: 1 }, { t: 1, offset: 2 }]), null)
})

test('mouseFeatures：空序列安全', () => {
  const f = mouseFeatures([])
  assert.equal(f.events, 0)
  assert.equal(f.speed_mean, 0)
})
