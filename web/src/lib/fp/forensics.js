// P1-3 JS 拦截取证：反检测浏览器主要靠 patch（重定义 accessor / 包装原生函数）来伪装，
// 本模块从三条互不依赖的线索取证：
//
//   1. accessor 原生性取证（核心，确定性）：目标属性在原型上的 getter 必须是原生实现
//      （Function.prototype.toString 含 [native code]、name/length 与原生签名一致）。
//      JS 层 patch 一定留下非原生源码 → 零误报可检出。
//   2. native function 取证：热点函数必须同为原生实现（同上判据）。
//   3. 读取耗时侧信道（弱，实测记录用）：被 patch 的属性理论上更慢，但**实测在 Chrome 上
//      不成立**——跨对象原生 getter（navigator.userAgent 166ns）比简单 JS getter（79ns）更慢，
//      且计时器被 Spectre 缓解粗化到 ~100µs，1e5 次循环摊薄后仍无分离度。故该通道只做
//      "同类原生属性中的相对离群"记录，阈值极保守（>5× 同类中位且 >100ns），不作为判定依据。
//   4. Error.stack 引擎格式 ↔ UA 声称内核交叉核验（UA 撒谎会露馅）。
//
// 判定逻辑与 DOM 探测分离（forensicFlags 是纯函数，可用 node --test 直接测）。

const TIMING_ITER = 100000
const TIMING_ROUNDS = 3
const TIMING_FACTOR = 5        // 相对同类原生属性的离群倍数
const TIMING_FLOOR_MS = 0.0001 // 100ns 噪声下限

// HOT_NATIVE 热点函数原生签名（name / length）。name 允许为空串（部分实现不设 name）。
const HOT_NATIVE = [
  { path: 'HTMLCanvasElement.prototype.toDataURL', name: 'toDataURL', length: 0 },
  { path: 'CanvasRenderingContext2D.prototype.getImageData', name: 'getImageData', length: 4 },
  { path: 'CanvasRenderingContext2D.prototype.fillText', name: 'fillText', length: 3 },
  { path: 'WebGLRenderingContext.prototype.getParameter', name: 'getParameter', length: 1 },
  { path: 'RTCPeerConnection', name: '', length: 0 },
  { path: 'Function.prototype.toString', name: 'toString', length: 0 }
]

// ACCESSOR_TARGETS 目标 accessor：反检测工具最常改的就是这几个（[路径, 原型取值器, 属性名]）
const ACCESSOR_TARGETS = [
  ['navigator.webdriver', () => Navigator.prototype, 'webdriver'],
  ['navigator.plugins', () => Navigator.prototype, 'plugins'],
  ['navigator.languages', () => Navigator.prototype, 'languages'],
  ['navigator.hardwareConcurrency', () => Navigator.prototype, 'hardwareConcurrency'],
  ['navigator.userAgent', () => Navigator.prototype, 'userAgent'],
  ['navigator.platform', () => Navigator.prototype, 'platform'],
  ['navigator.permissions', () => Navigator.prototype, 'permissions'],
  ['screen.colorDepth', () => Screen.prototype, 'colorDepth'],
  ['screen.width', () => Screen.prototype, 'width']
]

// ---------- 判定（纯函数） ----------

/**
 * accessorTamper 挑出被 patch 的 accessor（原生 getter 一定带 [native code]）。
 * @param {Array<{path: string, exists: boolean, getter: boolean, native: boolean, nameOk: boolean, lengthOk: boolean}>} items
 * @returns {string[]}
 */
export function accessorTamper(items) {
  return items
    .filter(i => i.exists && i.getter && (!i.native || !i.nameOk || !i.lengthOk))
    .map(i => i.path)
}

/**
 * timingOutliers 在同类原生属性的耗时分布里挑出离群目标（弱信号，仅记录）。
 * @param {number} referenceMs 同类原生属性的中位耗时
 * @param {Array<{name: string, ms: number}>} samples
 * @returns {string[]}
 */
export function timingOutliers(referenceMs, samples) {
  const line = Math.max(TIMING_FLOOR_MS, referenceMs * TIMING_FACTOR)
  return samples.filter(s => s.ms > line).map(s => s.name)
}

/**
 * nativeTamper 检查热点函数是否仍是原生实现。
 * @param {Array<{path: string, native: boolean, nameOk: boolean, lengthOk: boolean, exists: boolean}>} items
 * @returns {string[]} 异常函数路径
 */
export function nativeTamper(items) {
  return items
    .filter(i => i.exists && (!i.native || !i.nameOk || !i.lengthOk))
    .map(i => i.path)
}

/**
 * stackEngine 从 Error.stack 文本判断 JS 引擎家族。
 * @returns {'v8'|'spidermonkey'|'jsc'|'unknown'}
 */
export function stackEngine(stack) {
  const s = String(stack || '')
  if (!s) return 'unknown'
  // V8: "    at fn (url:1:2)"（行首缩进 + at）；SpiderMonkey/JSC: "fn@url:1:2"
  if (/^\s*at\s/m.test(s)) return 'v8'
  if (/@\S+:\d+:\d+/m.test(s)) {
    return /^\s*\S+@/m.test(s) && /(?:^|\n)\s*[^\s@]+@/m.test(s) ? 'jsc' : 'spidermonkey'
  }
  return 'unknown'
}

// UA 声称的内核 → 期望的栈格式家族
export function expectedEngine(ua) {
  const s = String(ua || '').toLowerCase()
  if (s.includes('firefox')) return 'spidermonkey'
  if (s.includes('safari') && !s.includes('chrome')) return 'jsc'
  if (s.includes('chrome') || s.includes('chromium') || s.includes('edg/')) return 'v8'
  return ''
}

/**
 * forensicFlags 依据取证结果产出 flag。accessor 与时序都按属性细分 flag
 * （与 env-flag:<flag> 同一思路）：不同属性的命中是独立证据，服务端可按 flag 粒度互证。
 */
export function forensicFlags(r) {
  const flags = []
  for (const name of r.accessorTampered || []) flags.push('fpb_getter_patched:' + name)
  for (const name of r.timingOutliers || []) flags.push('fpb_getter_timing:' + name)
  if ((r.nativeTampered || []).length > 0) flags.push('fpb_native_fn_tamper')
  if (r.stackSupported === false) {
    flags.push('fpb_stack_unavailable')
  } else if (r.stackEngine && r.expectedEngine && r.stackEngine !== 'unknown' && r.stackEngine !== r.expectedEngine) {
    flags.push('fpb_stack_version_mismatch')
  }
  return flags
}

// ---------- DOM 探测 ----------

function median(a) {
  const b = [...a].sort((x, y) => x - y)
  const m = Math.floor(b.length / 2)
  return b.length % 2 ? b[m] : (b[m - 1] + b[m]) / 2
}

// timeRead 单属性 N 次读取摊销耗时（毫秒/次），3 轮取中位数。
function timeRead(read) {
  const rounds = []
  for (let r = 0; r < TIMING_ROUNDS; r++) {
    let acc = 0
    const t0 = performance.now()
    for (let i = 0; i < TIMING_ITER; i++) {
      try { acc += read() ? 1 : 0 } catch { /* 读取抛错按最慢计 */ acc += 2 }
    }
    rounds.push((performance.now() - t0) / TIMING_ITER)
  }
  // acc 随返回值交出，避免循环被优化掉
  return { ms: median(rounds), checksum: rounds }
}

// TIMING_TARGETS 目标属性：反检测工具最常 patch 的就是这几个（也是原生实现的 accessor）。
const TIMING_TARGETS = [
  ['navigator.userAgent', () => navigator.userAgent.length],
  ['navigator.plugins', () => navigator.plugins.length],
  ['navigator.languages', () => navigator.languages.length],
  ['screen.colorDepth', () => screen.colorDepth],
  ['navigator.hardwareConcurrency', () => navigator.hardwareConcurrency],
  ['navigator.webdriver', () => navigator.webdriver]
]

// TIMING_REFERENCES 同类原生属性（不常见被 patch），用作耗时基准分布。
const TIMING_REFERENCES = [
  ['navigator.appVersion', () => navigator.appVersion.length],
  ['navigator.productSub', () => navigator.productSub.length],
  ['screen.pixelDepth', () => screen.pixelDepth],
  ['screen.availWidth', () => screen.availWidth],
  ['navigator.maxTouchPoints', () => navigator.maxTouchPoints]
]

function timingProbe() {
  try {
    const refs = TIMING_REFERENCES.map(([name, read]) => ({ name, ms: timeRead(read).ms }))
    const samples = TIMING_TARGETS.map(([name, read]) => ({ name, ms: timeRead(read).ms }))
    const refMs = median(refs.map(r => r.ms))
    return { supported: true, referenceMs: refMs, refs, samples }
  } catch {
    return { supported: false, referenceMs: 0, refs: [], samples: [] }
  }
}

// isNativeAccessor getter 必须是原生实现：[native code] + 名字与形参个数符合原生签名。
// V8 里 accessor 的 getter 名字形如 "get webdriver"，部分实现为属性名或空串，故都接受。
function isNativeAccessor(prop, fn) {
  let src = ''
  try { src = Function.prototype.toString.call(fn) } catch { return false }
  if (!src.includes('[native code]')) return false
  const nameOk = fn.name === '' || fn.name === prop || fn.name === 'get ' + prop
  return nameOk && fn.length === 0
}

// accessorProbe 目标 accessor 的原生性取证（确定性信号，JS patch 必留痕迹）。
function accessorProbe() {
  const items = ACCESSOR_TARGETS.map(([path, protoOf, prop]) => {
    try {
      const proto = protoOf()
      if (!proto) return { path, exists: false, getter: false, native: true, nameOk: true, lengthOk: true }
      const d = Object.getOwnPropertyDescriptor(proto, prop)
      if (!d || typeof d.get !== 'function') {
        return { path, exists: false, getter: false, native: true, nameOk: true, lengthOk: true }
      }
      let src = ''
      try { src = Function.prototype.toString.call(d.get) } catch { src = '' }
      return {
        path,
        exists: true,
        getter: true,
        native: src.includes('[native code]'),
        nameOk: d.get.name === '' || d.get.name === prop || d.get.name === 'get ' + prop,
        lengthOk: d.get.length === 0
      }
    } catch {
      return { path, exists: false, getter: false, native: true, nameOk: true, lengthOk: true }
    }
  })
  return { supported: true, items }
}

function readPath(path) {
  try {
    return path.split('.').reduce((o, k) => (o ? o[k] : undefined), window)
  } catch {
    return undefined
  }
}

function nativeProbe() {
  const items = HOT_NATIVE.map(({ path, name, length }) => {
    const fn = readPath(path)
    if (typeof fn !== 'function') return { path, exists: false, native: false, nameOk: true, lengthOk: true }
    let src = ''
    try { src = Function.prototype.toString.call(fn) } catch { src = '' }
    const native = src.includes('[native code]')
    // name 为空（如 RTCPeerConnection 构造器）时跳过名字比对，只看 length 与 native 标记
    return {
      path,
      exists: true,
      native,
      nameOk: name === '' ? true : fn.name === name,
      lengthOk: fn.length === length
    }
  })
  return { supported: true, items }
}

function stackProbe() {
  try {
    const stack = new Error('fpb').stack
    if (!stack) return { supported: false, engine: 'unknown' }
    return { supported: true, engine: stackEngine(stack) }
  } catch {
    return { supported: false, engine: 'unknown' }
  }
}

// forensicsDetect 探测 + 判定一步到位。accessor 取证是确定性的，时序通道只记录。
export async function forensicsDetect() {
  const accessors = accessorProbe()
  const natives = nativeProbe()
  const stack = stackProbe()
  let timing = { supported: false, referenceMs: 0, refs: [], samples: [] }
  try {
    timing = timingProbe()
  } catch { /* 忽略 */ }
  const raw = {
    accessorTampered: accessorTamper(accessors.items),
    nativeTampered: nativeTamper(natives.items),
    timingOutliers: timing.supported ? timingOutliers(timing.referenceMs, timing.samples) : [],
    timingReferenceMs: timing.referenceMs,
    timingSamples: timing.samples,
    stackSupported: stack.supported,
    stackEngine: stack.engine,
    expectedEngine: expectedEngine(navigator.userAgent || '')
  }
  return { flags: forensicFlags(raw), raw }
}
