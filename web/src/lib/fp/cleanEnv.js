// P1-1 干净环境对照：把同一份渲染/读取指令放到"独立执行环境"里重放，
// 结果与主线程不一致 ⇒ 主线程的 DOM/Canvas/音频 API 被 patch（反检测浏览器的核心特征）。
//
// 三个对照通道：
//   1. Worker-Canvas：主线程 HTMLCanvasElement 渲染的像素 vs OffscreenCanvas（Worker 内）渲染的像素；
//   2. iframe 参照：about:blank 同源 iframe 有独立 realm 与原型链，其 navigator/screen 取值
//      应与主框架一致（patch 主框架原型不会影响 iframe）；
//   3. 音频双通道：同一 chirp 两次独立渲染必须逐样本一致（随机化 patch 会露馅），
//      且 getChannelData 与 copyFromChannel 两条读取路径必须给出同一份样本。
//
// 判定逻辑与 DOM 探测分离（cleanEnvFlags 是纯函数，可用 node --test 直接测）。

const CANVAS_W = 280
const CANVAS_H = 60
const AUDIO_SAMPLES = 4096
const PROBE_TIMEOUT_MS = 4000

// drawInstr 是唯一事实来源：主线程直接调用，同时通过 toString() 注入 Worker 源码，
// 保证两侧绘图指令逐字节一致（否则对照本身就失真）。
function drawInstr(x) {
  x.textBaseline = 'top'
  x.font = '16px Arial, sans-serif'
  x.fillStyle = '#f60'
  x.fillRect(10, 10, 120, 30)
  const g = x.createLinearGradient(0, 0, 280, 0)
  g.addColorStop(0, '#0af')
  g.addColorStop(1, '#a0f')
  x.fillStyle = g
  x.fillText('GithubHot 🚀 对照 CONTRAST 0123', 12, 12)
  x.fillStyle = 'rgba(0,0,0,0.35)'
  x.fillText('GithubHot 🚀 对照 CONTRAST 0123', 13.5, 14.5)
  x.strokeStyle = '#123'
  x.beginPath()
  x.arc(200, 30, 18, 0, Math.PI * 1.5)
  x.stroke()
}

// hashBytes FNV-1a 32 位（非安全用途：只需判断两组像素/样本是否同一份）。
export function hashBytes(bytes) {
  let h = 2166136261
  for (let i = 0; i < bytes.length; i++) {
    h ^= bytes[i]
    h = Math.imul(h, 16777619)
  }
  return (h >>> 0).toString(16).padStart(8, '0')
}

// ---------- 判定（纯函数，可单测） ----------

/**
 * cleanEnvFlags 依据各通道探测结果产出检测 flag。
 * @param {object} r
 * @param {boolean}  r.canvasSupported 主线程 canvas 与 Worker/OffscreenCanvas 是否都可用
 * @param {string}   r.canvasMain   主线程渲染像素哈希
 * @param {string}   r.canvasWorker Worker 内渲染像素哈希
 * @param {boolean}  r.iframeSupported about:blank iframe 参照是否可用
 * @param {string[]} r.iframeDiff   与 iframe 参照不一致的属性名
 * @param {boolean}  r.audioSupported 音频对照是否可用
 * @param {boolean}  r.audioDeterministic 同一 chirp 两次渲染是否一致
 * @param {boolean}  r.audioReadConsistent 两条读取路径是否一致
 * @returns {string[]} flags
 */
export function cleanEnvFlags(r) {
  const flags = []
  if (!r.canvasSupported) {
    flags.push('fpb_canvas_check_unsupported')
  } else if (r.canvasMain && r.canvasWorker && r.canvasMain !== r.canvasWorker) {
    flags.push('fpb_canvas_diverge')
  }
  if (!r.iframeSupported) {
    flags.push('fpb_iframe_check_unsupported')
  } else if ((r.iframeDiff || []).length > 0) {
    flags.push('fpb_iframe_diverge')
  }
  if (!r.audioSupported) {
    flags.push('fpb_audio_check_unsupported')
  } else if (!r.audioDeterministic || !r.audioReadConsistent) {
    flags.push('fpb_audio_diverge')
  }
  return flags
}

// ---------- DOM 探测 ----------

function withTimeout(promise, ms) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout')), ms)
    promise.then(v => { clearTimeout(timer); resolve(v) }, e => { clearTimeout(timer); reject(e) })
  })
}

// canvasProbe 主线程画布像素与 Worker 内 OffscreenCanvas 像素对照。
async function canvasProbe() {
  let mainBytes = ''
  try {
    const c = document.createElement('canvas')
    c.width = CANVAS_W
    c.height = CANVAS_H
    const x = c.getContext('2d')
    drawInstr(x)
    mainBytes = hashBytes(x.getImageData(0, 0, CANVAS_W, CANVAS_H).data)
  } catch {
    return { supported: false }
  }
  if (typeof Worker === 'undefined' || typeof OffscreenCanvas === 'undefined' ||
    typeof URL === 'undefined' || !URL.createObjectURL) {
    return { supported: false, main: mainBytes }
  }
  // 同一份绘图指令注入 Worker：直接复用 drawInstr 的源码，避免两侧漂移
  const src = `
    const DRAW = ${drawInstr.toString()};
    self.onmessage = () => {
      try {
        const c = new OffscreenCanvas(${CANVAS_W}, ${CANVAS_H});
        const x = c.getContext('2d');
        DRAW(x);
        const d = x.getImageData(0, 0, ${CANVAS_W}, ${CANVAS_H}).data;
        self.postMessage(d.buffer, [d.buffer]);
      } catch (e) {
        self.postMessage({ error: String(e) });
      }
    };`
  let url = ''
  let worker = null
  try {
    url = URL.createObjectURL(new Blob([src], { type: 'text/javascript' }))
    worker = new Worker(url)
    const data = await withTimeout(new Promise((resolve, reject) => {
      worker.onmessage = e => resolve(e.data)
      worker.onerror = e => reject(e)
      worker.postMessage(1)
    }), PROBE_TIMEOUT_MS)
    if (!data || data.error) return { supported: false, main: mainBytes }
    return { supported: true, main: mainBytes, worker: hashBytes(new Uint8Array(data)) }
  } catch {
    return { supported: false, main: mainBytes }
  } finally {
    try { worker && worker.terminate() } catch { /* 忽略 */ }
    try { url && URL.revokeObjectURL(url) } catch { /* 忽略 */ }
  }
}

// iframeProps about:blank iframe 应逐项一致的属性（独立 realm，不受主框架 patch 影响）。
const iframeProps = [
  ['webdriver', w => w.navigator.webdriver],
  ['plugins', w => (w.navigator.plugins || []).length],
  ['mimeTypes', w => (w.navigator.mimeTypes || []).length],
  ['hardwareConcurrency', w => w.navigator.hardwareConcurrency],
  ['deviceMemory', w => w.navigator.deviceMemory],
  ['languages', w => (w.navigator.languages || []).join(',')],
  ['userAgent', w => w.navigator.userAgent],
  ['platform', w => w.navigator.platform],
  ['colorDepth', w => (w.screen || {}).colorDepth]
]

function iframeProbe() {
  let frame = null
  try {
    frame = document.createElement('iframe')
    frame.style.display = 'none'
    frame.setAttribute('aria-hidden', 'true')
    frame.src = 'about:blank'
    document.body.appendChild(frame)
    const inner = frame.contentWindow
    if (!inner || !inner.navigator) return { supported: false, diff: [] }
    const diff = []
    for (const [name, read] of iframeProps) {
      let a
      let b
      try { a = String(read(window)) } catch { continue }
      try { b = String(read(inner)) } catch { continue }
      if (a !== b) diff.push(name)
    }
    return { supported: true, diff }
  } catch {
    return { supported: false, diff: [] }
  } finally {
    try { frame && frame.remove() } catch { /* 忽略 */ }
  }
}

function renderChirp(Ctx) {
  const ctx = new Ctx(1, 44100, 44100)
  const osc = ctx.createOscillator()
  osc.type = 'triangle'
  osc.frequency.value = 10000
  const comp = ctx.createDynamicsCompressor()
  comp.threshold.value = -50
  comp.ratio.value = 12
  osc.connect(comp)
  comp.connect(ctx.destination)
  osc.start(0)
  return ctx.startRendering()
}

function sampleSum(d) {
  let s = 0
  for (let i = 0; i < d.length; i++) s += Math.abs(d[i])
  return s
}

// audioProbe 音频对照：两次独立渲染一致 + 两条读取路径一致。
async function audioProbe() {
  const Ctx = window.OfflineAudioContext || window.webkitOfflineAudioContext
  if (!Ctx) return { supported: false }
  try {
    const buf1 = await renderChirp(Ctx)
    const buf2 = await renderChirp(Ctx)
    const n = Math.min(buf1.length, AUDIO_SAMPLES)
    const d1 = buf1.getChannelData(0).slice(0, n)
    const d2 = buf2.getChannelData(0).slice(0, n)
    const deterministic = hashBytes(new Uint8Array(d1.buffer)) === hashBytes(new Uint8Array(d2.buffer))
    let readConsistent = true
    if (typeof buf1.copyFromChannel === 'function') {
      const out = new Float32Array(n)
      buf1.copyFromChannel(out, 0)
      readConsistent = Math.abs(sampleSum(d1) - sampleSum(out)) < 1e-6
    }
    return { supported: true, deterministic, readConsistent }
  } catch {
    return { supported: false }
  }
}

// probeCleanEnv 汇总三通道探测（任一通道失败都不抛出，返回 supported=false）。
export async function probeCleanEnv() {
  const [canvas, iframe, audio] = await Promise.all([
    withTimeout(canvasProbe(), PROBE_TIMEOUT_MS).catch(() => ({ supported: false })),
    Promise.resolve().then(iframeProbe),
    withTimeout(audioProbe(), PROBE_TIMEOUT_MS).catch(() => ({ supported: false }))
  ])
  return {
    canvasSupported: !!canvas.supported,
    canvasMain: canvas.main || '',
    canvasWorker: canvas.worker || '',
    iframeSupported: !!iframe.supported,
    iframeDiff: iframe.diff || [],
    audioSupported: !!audio.supported,
    audioDeterministic: audio.deterministic !== false,
    audioReadConsistent: audio.readConsistent !== false
  }
}

// cleanEnvDetect 探测 + 判定一步到位。
export async function cleanEnvDetect() {
  const raw = await probeCleanEnv()
  return { flags: cleanEnvFlags(raw), raw }
}
