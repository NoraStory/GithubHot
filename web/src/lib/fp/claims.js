// P1-2 能力—声明核验：不看客户端"自报了什么"，而是看它**报出的声明**与**实测能力**是否自洽。
// 这条路对内核级 patch 也有效——patch 能改声明（UNMASKED_RENDERER / hardwareConcurrency），
// 但改不了真实算力与真实纹理上限。
//
//   1. GPU 能力集：MAX_TEXTURE_SIZE 实测值 vs UNMASKED_RENDERER 声称的档位；
//   2. 字体实测：Canvas measureText 与 DOM 布局两条独立度量路径必须给出一致结论；
//   3. 核数基准：hardwareConcurrency 声明值 vs 固定微基准耗时档位（低置信，只记录）。
//
// 判定逻辑与 DOM 探测分离（claimFlags / fontAvailability 是纯函数，可用 node --test 直接测）。

import gpuTable from './gpu-capabilities.json' with { type: 'json' }

// FONT_SAMPLE 字体实测抽样（覆盖中英 + 各系统常见字体，10 个）
const FONT_SAMPLE = ['Arial', 'Consolas', 'Segoe UI', 'Microsoft YaHei', 'PingFang SC',
  'Times New Roman', 'Courier New', 'SimSun', 'Helvetica', 'Noto Sans CJK SC']

const CORES_BENCH_ITER = 1e7

// ---------- 判定（纯函数） ----------

/**
 * fontAvailability 单字体的可用性判定：两条独立度量路径各给出"与回退字体的宽度差（px）"，
 * 差值落在模糊带（0.5–2px，亚像素/字距舍入）内的样本归为 ambiguous 直接跳过——
 * 避免把舍入噪声当成矛盾。两条路径结论不一致才是 conflict。
 *
 * 为什么不用规格里的 document.fonts.check：实测 Chrome 的 check() 不做字体族匹配，
 * 对**未安装**的系统字体同样返回 true（它只回答"有没有待加载的 webfont"），
 * 与 measureText 的可用性结论天然不一致 → 在无头/精简字体环境会产生 100% 假阳性。
 * 因此改为 Canvas measureText 与 DOM 布局（offsetWidth）两条真实度量路径交叉。
 */
export function fontAvailability(canvasDiffPx, domDiffPx) {
  const cls = d => {
    const v = Math.abs(Number(d) || 0)
    if (v <= 0.5) return 'absent'
    if (v >= 2) return 'installed'
    return 'ambiguous'
  }
  const a = cls(canvasDiffPx)
  const b = cls(domDiffPx)
  if (a === 'ambiguous' || b === 'ambiguous') return 'ambiguous'
  return a === b ? a : 'conflict'
}

// countFontConflicts 统计两条路径结论冲突的字体数。
export function countFontConflicts(pairs) {
  return (pairs || []).filter(p => fontAvailability(p.canvas, p.dom) === 'conflict').length
}

/**
 * gpuClassFor 按 renderer 声明串匹配能力档位。
 * @param {string} renderer UNMASKED_RENDERER_WEBGL
 * @returns {{vendor: string, minMaxTextureSize: number}|null}
 */
export function gpuClassFor(renderer, table = gpuTable) {
  const r = String(renderer || '').toLowerCase()
  if (!r) return null
  for (const cls of table.classes) {
    if (cls.match.some(m => r.includes(m))) return cls
  }
  return null
}

/**
 * coresBucket 按声明核数与基准耗时分档，返回两者是否"同一档"。
 * 阈值刻意放宽（CPU 节流/后台标签会让耗时长几十倍），只抓极端矛盾。
 */
export function coresBucket(cores, ms) {
  const claimed = cores >= 12 ? 'high' : cores >= 4 ? 'mid' : 'low'
  const measured = ms <= 12 ? 'fast' : ms <= 150 ? 'mid' : 'slow'
  // 只有"高档声明 + 极慢实测"或"低档声明 + 极快实测"才算矛盾
  const mismatch = (claimed === 'high' && measured === 'slow') || (claimed === 'low' && measured === 'fast')
  return { claimed, measured, mismatch }
}

/**
 * claimFlags 依据探测结果产出 flag。
 * @param {object} r
 * @param {string} [r.renderer] UNMASKED_RENDERER_WEBGL
 * @param {number} [r.maxTextureSize]
 * @param {boolean} r.gpuSupported
 * @param {boolean} r.fontsSupported
 * @param {number} r.fontDisagreements 两条独立度量路径（Canvas / DOM 布局）结论冲突的字体数
 * @param {number} r.fontSampled 实际抽样数
 * @param {number} [r.cores] hardwareConcurrency
 * @param {number} [r.benchMs] 基准耗时（毫秒）
 * @returns {string[]}
 */
export function claimFlags(r) {
  const flags = []
  if (!r.gpuSupported) {
    flags.push('fpb_gpu_check_unsupported')
  } else if (r.softwareGPU) {
    // 软件光栅化（SwiftShader / llvmpipe）：无头环境与虚拟机的典型形态，
    // 但部分 Linux 桌面/远程桌面也合法存在 → 只记录不计分
    flags.push('fpb_gpu_software')
  } else if (r.renderer) {
    const cls = gpuClassFor(r.renderer)
    if (cls && cls.minMaxTextureSize > 0 && r.maxTextureSize > 0 &&
      r.maxTextureSize < cls.minMaxTextureSize) {
      flags.push('fpb_gpu_claim_mismatch')
    }
  }
  if (!r.fontsSupported) {
    flags.push('fpb_font_check_unsupported')
  } else if (r.fontDisagreements >= 3) {
    flags.push('fpb_font_claim_mismatch')
  }
  if (r.cores > 0 && r.benchMs > 0 && coresBucket(r.cores, r.benchMs).mismatch) {
    flags.push('fpb_cores_claim_mismatch')
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

function gpuProbe() {
  try {
    const c = document.createElement('canvas')
    const gl = c.getContext('webgl') || c.getContext('experimental-webgl')
    if (!gl) return { supported: false }
    const dbg = gl.getExtension('WEBGL_debug_renderer_info')
    const renderer = String((dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER)) || '')
    const maxTex = Number(gl.getParameter(gl.MAX_TEXTURE_SIZE)) || 0
    const software = /swiftshader|llvmpipe|software|basic render|mesa offscreen/i.test(renderer)
    return { supported: true, renderer, maxTextureSize: maxTex, software }
  } catch {
    return { supported: false }
  }
}

// fontProbe 字体实测：Canvas measureText 与 DOM 布局（offsetWidth）是两条独立的文本度量
// 实现，对"某字体是否真的装了"必须给出一致结论；不一致说明其中一条被 patch 或伪造。
function fontProbe() {
  let span = null
  try {
    const c = document.createElement('canvas')
    const x = c.getContext('2d')
    if (!x) return { supported: false }
    const text = 'GithubHot 指纹测试 WiwW 0123'
    const stack = f => '16px ' + (f ? '"' + f + '", ' : '') + 'monospace'
    x.font = stack('')
    const baseCanvas = x.measureText(text).width
    span = document.createElement('span')
    span.style.cssText = 'position:absolute;left:-9999px;top:0;white-space:pre;visibility:hidden'
    span.textContent = text
    span.style.font = stack('')
    document.body.appendChild(span)
    const baseDom = span.getBoundingClientRect().width
    if (!baseCanvas || !baseDom) return { supported: false }
    const pairs = []
    for (const f of FONT_SAMPLE) {
      x.font = stack(f)
      const canvasDiff = x.measureText(text).width - baseCanvas
      span.style.font = stack(f)
      const domDiff = span.getBoundingClientRect().width - baseDom
      pairs.push({ font: f, canvas: canvasDiff, dom: domDiff })
    }
    return {
      supported: true,
      sampled: pairs.filter(p => fontAvailability(p.canvas, p.dom) !== 'ambiguous').length,
      disagreements: countFontConflicts(pairs),
      pairs
    }
  } catch {
    return { supported: false }
  } finally {
    try { span && span.remove() } catch { /* 忽略 */ }
  }
}

// coresProbe 固定整数微基准（1e7 次运算），与 hardwareConcurrency 声明档位对照。
function coresProbe() {
  const cores = Number(navigator.hardwareConcurrency) || 0
  if (!cores) return { cores: 0, benchMs: 0, checksum: 0 }
  try {
    const t0 = performance.now()
    let acc = 0
    for (let i = 0; i < CORES_BENCH_ITER; i++) acc = (acc + i) ^ (i >>> 3)
    const ms = performance.now() - t0
    // checksum 随返回值一起交出：累加结果被真正使用，循环才不会被 JIT 消除
    return { cores, benchMs: ms, checksum: acc }
  } catch {
    return { cores, benchMs: 0, checksum: 0 }
  }
}

// claimsDetect 探测 + 判定一步到位。
export async function claimsDetect() {
  const gpu = gpuProbe()
  const fonts = fontProbe()
  // 微基准放到最后单独跑，避免与其它采集相互干扰计时
  const cores = await withTimeout(Promise.resolve().then(coresProbe), 3000)
    .catch(() => ({ cores: 0, benchMs: 0 }))
  const raw = {
    gpuSupported: !!gpu.supported,
    renderer: gpu.renderer || '',
    maxTextureSize: gpu.maxTextureSize || 0,
    softwareGPU: !!gpu.software,
    fontsSupported: !!fonts.supported,
    fontSampled: fonts.sampled || 0,
    fontDisagreements: fonts.disagreements || 0,
    fontPairs: fonts.pairs || [],
    cores: cores.cores,
    benchMs: cores.benchMs
  }
  return { flags: claimFlags(raw), raw }
}

// GPU_CLASSES 导出给单测核对表结构（避免测试文件再引一次 JSON 模块）。
export const GPU_CLASSES = gpuTable.classes
