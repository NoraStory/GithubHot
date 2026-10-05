import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  accessorTamper, timingOutliers, nativeTamper, stackEngine, expectedEngine, forensicFlags
} from './forensics.js'

// ---------- accessor 原生性取证（核心确定性信号） ----------

test('accessorTamper：非原生 getter / 名字或形参不符都算被 patch', () => {
  const items = [
    { path: 'navigator.userAgent', exists: true, getter: true, native: true, nameOk: true, lengthOk: true },
    { path: 'navigator.webdriver', exists: true, getter: true, native: false, nameOk: true, lengthOk: true },
    { path: 'screen.width', exists: true, getter: true, native: true, nameOk: false, lengthOk: true },
    { path: 'screen.height', exists: true, getter: true, native: true, nameOk: true, lengthOk: false },
    // 数据属性/不存在的属性不参与 accessor 判定（避免误报）
    { path: 'navigator.languages', exists: false, getter: false, native: false, nameOk: false, lengthOk: false },
    { path: 'screen.colorDepth', exists: true, getter: false, native: false, nameOk: false, lengthOk: false }
  ]
  assert.deepEqual(accessorTamper(items), ['navigator.webdriver', 'screen.width', 'screen.height'])
})

// ---------- 读取耗时侧信道（弱信号：实测 Chrome 无分离度，只记录） ----------

test('timingOutliers：超过同类原生属性 5× 才算离群，并受 100ns 下限保护', () => {
  // 实测值域：原生跨对象 getter ~120-170ns、简单 JS getter ~80ns → 一律不报
  assert.deepEqual(timingOutliers(0.00015, [{ name: 'a', ms: 0.000079 }, { name: 'b', ms: 0.000166 }]), [])
  // 明显慢一个量级（例如 patch 里做了重活）才记录
  assert.deepEqual(timingOutliers(0.00002, [{ name: 'slow', ms: 0.0005 }]), ['slow'])
  // 极快基线（20ns）时不会把 80ns 的 JS getter 误判为离群（20×5=100ns 下限 vs 噪声下限 100ns）
  assert.deepEqual(timingOutliers(0.00002, [{ name: 'jsgetter', ms: 0.00008 }]), [])
})

// ---------- native function 取证 ----------

test('nativeTamper：非原生实现/名字或形参不符都算异常', () => {
  const items = [
    { path: 'ok', exists: true, native: true, nameOk: true, lengthOk: true },
    { path: 'wrapped', exists: true, native: false, nameOk: true, lengthOk: true },
    { path: 'renamed', exists: true, native: true, nameOk: false, lengthOk: true },
    { path: 'arity', exists: true, native: true, nameOk: true, lengthOk: false },
    { path: 'missing', exists: false, native: false, nameOk: false, lengthOk: false }
  ]
  assert.deepEqual(nativeTamper(items), ['wrapped', 'renamed', 'arity'])
})

// ---------- 错误栈版本指纹 ----------

test('Error.stack 引擎识别：V8 与 SpiderMonkey/JSC 两种格式', () => {
  assert.equal(stackEngine('Error: x\n    at foo (http://a/b.js:1:2)\n    at bar'), 'v8')
  assert.equal(stackEngine('foo@http://a/b.js:1:2\nbar@http://a/b.js:3:4'), 'jsc')
  assert.equal(stackEngine(''), 'unknown')
})

test('UA 声称内核 ↔ 栈格式矛盾 → fpb_stack_version_mismatch', () => {
  assert.equal(expectedEngine('Mozilla/5.0 Chrome/126'), 'v8')
  assert.equal(expectedEngine('Mozilla/5.0 Firefox/128'), 'spidermonkey')
  assert.equal(expectedEngine('Mozilla/5.0 (Macintosh) Version/17 Safari/605'), 'jsc')
  assert.equal(expectedEngine('curl/8.4'), '')
  const flags = forensicFlags({ stackSupported: true, stackEngine: 'jsc', expectedEngine: 'v8' })
  assert.deepEqual(flags, ['fpb_stack_version_mismatch'])
})

// ---------- 汇总 ----------

test('取证命中按属性细分 flag（不同属性是独立证据）', () => {
  const flags = forensicFlags({
    stackSupported: true,
    stackEngine: 'v8',
    expectedEngine: 'v8',
    accessorTampered: ['navigator.webdriver', 'screen.colorDepth'],
    timingOutliers: ['navigator.plugins'],
    nativeTampered: ['HTMLCanvasElement.prototype.toDataURL']
  })
  assert.deepEqual(flags, [
    'fpb_getter_patched:navigator.webdriver',
    'fpb_getter_patched:screen.colorDepth',
    'fpb_getter_timing:navigator.plugins',
    'fpb_native_fn_tamper'
  ])
})

test('取证全部干净 → 零 flag（真人浏览器红线）', () => {
  assert.deepEqual(forensicFlags({
    stackSupported: true, stackEngine: 'v8', expectedEngine: 'v8',
    accessorTampered: [], timingOutliers: [], nativeTampered: []
  }), [])
  assert.deepEqual(forensicFlags({ stackSupported: false }), ['fpb_stack_unavailable'])
})
