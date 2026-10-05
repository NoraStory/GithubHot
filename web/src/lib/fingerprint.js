// 设备指纹采集（第二层）：Canvas 渲染噪声 + WebGL 显卡信息 + 音频栈 + 字体枚举
// + WebRTC 真实 IP + 屏幕/环境参数，分量哈希后融合成设备指纹，每会话上报一次。
// 第三层环境核验：UA/platform/时区/语言一致性 + 无头浏览器与自动化框架痕迹检测，
// 命中项随上报传给服务端存档并计违规分。
// P1 检测族（fp/ 目录，结果全部走同一份 flags 上报，表结构零改动）：
//   - 干净环境对照 fpb_canvas_diverge / fpb_iframe_diverge / fpb_audio_diverge
//   - 能力—声明核验（P1-2）、JS 拦截取证（P1-3）、BotD（P1-4）
// 采集全程异步静默，失败任何一项都不影响站点功能。

import { cleanEnvDetect } from './fp/cleanEnv'
import { claimsDetect } from './fp/claims'
import { forensicsDetect } from './fp/forensics'
import { botdDetect } from './fp/botd'
import { phashFromImageData } from './phash'

const FP_KEY = 'gh_fp'
const REPORTED_KEY = 'gh_fp_reported'
const DETECT_TIMEOUT_MS = 5000
let reportRetry = 0

export function deviceFp() {
  try { return localStorage.getItem(FP_KEY) || '' } catch { return '' }
}

async function sha256(text) {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
  return btoa(String.fromCharCode(...new Uint8Array(buf).slice(0, 18)))
}

// Canvas 指纹：emoji + 渐变文本渲染的 GPU 驱动级差异。
// 同时算出 P2-1 感知哈希（pHash）：精确哈希会因驱动更新/抗指纹噪声漂移，
// pHash 对同一台设备的像素微改仍然稳定，服务端据此把"换了手指纹"关联回同一设备。
async function canvasFp() {
  try {
    const c = document.createElement('canvas')
    c.width = 280; c.height = 60
    const x = c.getContext('2d')
    x.textBaseline = 'top'
    x.font = '16px "Arial"'
    x.fillStyle = '#f60'
    x.fillRect(10, 10, 120, 30)
    const g = x.createLinearGradient(0, 0, 280, 0)
    g.addColorStop(0, '#0af'); g.addColorStop(1, '#a0f')
    x.fillStyle = g
    x.fillText('GithubHot 🚀 指纹 測試 0123', 12, 12)
    x.fillStyle = 'rgba(0,0,0,0.35)'
    x.fillText('GithubHot 🚀 指纹 測試 0123', 13.5, 14.5)
    const hash = await sha256(c.toDataURL())
    let phash = ''
    try {
      phash = phashFromImageData(x.getImageData(0, 0, c.width, c.height))
    } catch { phash = '' } // 像素读取失败（画布污染）不影响精确哈希
    return { hash, phash }
  } catch { return { hash: '', phash: '' } }
}

// WebGL 指纹：厂商/显卡（UNMASKED）、扩展列表、着色精度、最大纹理。
// 返回 {hash, renderer, exts}：renderer 原文入档案便于管理端辨认显卡型号；
// exts（P2-2）随 sets 上报，服务端算 MinHash 签名做组件集合关联。
function webglInfo() {
  try {
    const c = document.createElement('canvas')
    const gl = c.getContext('webgl') || c.getContext('experimental-webgl')
    if (!gl) return { hash: '', renderer: '', exts: [] }
    const dbg = gl.getExtension('WEBGL_debug_renderer_info')
    const vendor = dbg ? gl.getParameter(dbg.UNMASKED_VENDOR_WEBGL) : gl.getParameter(gl.VENDOR)
    const renderer = dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER)
    const exts = gl.getSupportedExtensions() || []
    const prec = gl.getShaderPrecisionFormat(gl.VERTEX_SHADER, gl.HIGH_FLOAT)
    const raw = [vendor, renderer, exts.join(','), gl.getParameter(gl.MAX_TEXTURE_SIZE),
      gl.getParameter(gl.MAX_VERTEX_ATTRIBS), prec ? `${prec.precision}/${prec.rangeMin}/${prec.rangeMax}` : ''].join('|')
    return { hash: raw, renderer: String(renderer || ''), exts: exts.map(String) }
  } catch { return { hash: '', renderer: '', exts: [] } }
}

async function webglFp() {
  const i = webglInfo()
  return { hash: i.hash ? await sha256(i.hash) : '', renderer: i.renderer }
}

// 音频指纹：OfflineAudioContext 生成正弦叠加音，各浏览器音频栈渲染存在驱动级差异
async function audioFp() {
  try {
    const Ctx = window.OfflineAudioContext || window.webkitOfflineAudioContext
    if (!Ctx) return ''
    const ctx = new Ctx(1, 44100, 44100)
    const osc = ctx.createOscillator()
    osc.type = 'triangle'
    osc.frequency.value = 10000
    const comp = ctx.createDynamicsCompressor()
    osc.connect(comp); comp.connect(ctx.destination)
    osc.start(0)
    const buf = await ctx.startRendering()
    const d = buf.getChannelData(0)
    let s = 0
    for (let i = 2000; i < 3000; i++) s += Math.abs(d[i])
    return await sha256('audio' + s.toFixed(6) + d.length)
  } catch { return '' }
}

// 字体枚举指纹：探测常见中英文字体的实际渲染宽度差异（像素级）。
// 返回 {hash, detected}：detected（P2-2）是实测存在的字体清单，随 sets 上报。
async function fontsFp() {
  try {
    const base = ['monospace', 'sans-serif', 'serif']
    const probe = ['Arial', 'Consolas', 'Segoe UI', 'Microsoft YaHei', 'PingFang SC',
      'Helvetica', 'Times New Roman', 'Courier New', 'SimSun', 'Noto Sans CJK SC']
    const c = document.createElement('canvas')
    const x = c.getContext('2d')
    const text = 'GithubHot 指纹测试 WiwW 0123'
    const baseW = base.map(f => {
      x.font = '16px ' + f
      return x.measureText(text).width
    })
    const detected = []
    for (const f of probe) {
      x.font = '16px "' + f + '", monospace'
      const w = x.measureText(text).width
      if (Math.abs(w - baseW[0]) > 0.5) detected.push(f)
    }
    return { hash: await sha256('fonts:' + detected.join(',')), detected }
  } catch { return { hash: '', detected: [] } }
}

// 环境核验（第三层）：返回命中项列表；空数组 = 环境自洽。
function envAudit() {
  const flags = []
  try {
    const ua = (navigator.userAgent || '').toLowerCase()
    const p = (navigator.platform || '').toLowerCase()
    // 1) UA ↔ platform 矛盾（改机工具常留此类痕迹）
    const saysWin = ua.includes('windows'), saysMac = ua.includes('mac os') || ua.includes('macintosh')
    const saysLinux = ua.includes('linux') && !ua.includes('android')
    const saysAndroid = ua.includes('android')
    if (p && ((p.startsWith('win') && !saysWin) || (p.startsWith('mac') && !saysMac)
      || (p.includes('android') && !saysAndroid) || (p.includes('linux') && !saysLinux && !saysAndroid))) {
      flags.push('ua-platform-mismatch')
    }
    // 2) 无头浏览器特征
    if (ua.includes('headless') || ua.includes('phantom') || ua.includes('selenium')) flags.push('headless-ua')
    // 3) 自动化框架注入的全局变量（Chrome 驱动会留 cdc_ 等痕迹）
    const w = window
    if (w.document && (w.document.$cdc_ || w.document.domAutomation || w.document.domAutomationController
      || w.document.__webdriver_script_fn || w.document.__selenium_unwrapped
      || w.document.__driver_evaluate || w.document.__webdriver_evaluate)) flags.push('automation-global')
    if (w.navigator.webdriver === true) flags.push('navigator-webdriver')
    if (w._phantom || w.__nightmare || w._selenium || w.callPhantom) flags.push('automation-global')
    if (w.external && w.external.toString && w.external.toString().includes('Sequentum')) flags.push('automation-global')
    // 4) 语言 ↔ 时区矛盾：中文语言环境却用美洲/欧洲时区（或反之），常见于伪造 header 的爬虫
    const langs = (navigator.languages || [navigator.language || '']).join(',').toLowerCase()
    const tz = -new Date().getTimezoneOffset() // 分钟，东八区 = +480
    const cnLang = langs.includes('zh') || langs.includes('cn')
    if (cnLang && (tz <= -180 || tz >= 600)) flags.push('lang-tz-mismatch')
    if (!cnLang && tz === 480 && langs.length > 0 && !langs.includes('en-us')) flags.push('lang-tz-mismatch')
    // 5) 桌面 Chrome 却没有插件接口（无头/精简环境的典型特征）
    if (ua.includes('chrome') && !saysAndroid && !ua.includes('edg/')
      && (!navigator.plugins || navigator.plugins.length === 0)) flags.push('no-plugins')
  } catch { /* 核验失败不阻塞 */ }
  return flags
}

// WebRTC IP（第二层深度特征）：host 候选 + STUN 反射（srflx）拿 NAT 后的真实公网 IP。
// 现代浏览器本地候选常给 mDNS 名（同样入指纹），srflx 候选暴露真实公网 IP。
function webrtcIPs() {
  return new Promise(resolve => {
    const ips = []
    if (!window.RTCPeerConnection) return resolve(ips)
    let done = false
    const finish = () => { if (!done) { done = true; try { pc.close() } catch {}; resolve(ips) } }
    const pc = new RTCPeerConnection({
      iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        { urls: 'stun:stun.cloudflare.com:3478' }
      ]
    })
    try { pc.createDataChannel('x') } catch { return resolve(ips) }
    const timer = setTimeout(finish, 2500)
    pc.onicecandidate = e => {
      if (!e.candidate) { clearTimeout(timer); finish(); return }
      // candidate:<foundation> <component> <protocol> <priority> <address> <port> typ <type>
      const m = /candidate:\S+ \d+ \S+ \d+ (\S+) \d+ typ (host|srflx)/.exec(e.candidate.candidate || '')
      if (m && ips.indexOf(m[1]) < 0) ips.push(m[1])
    }
    pc.createOffer().then(o => pc.setLocalDescription(o)).catch(() => { clearTimeout(timer); finish() })
  })
}

// 环境参数信号（构成身份哈希的稳定项）
//
// 注意：devicePixelRatio **刻意不参与身份哈希**——浏览器缩放（Ctrl+/-）、窗口拖到
// 不同缩放比的显示器都会改变它，若并入哈希会导致同一台设备产生新指纹，进而触发
// 服务端 device-mismatch（旧实现 +80 分，10 分钟内缩放两次即可致封）。
// DPR 作为软信号单独上报（components.dpr），仅供管理端排查，不影响身份判定。
function envSignals() {
  const n = navigator
  return [
    n.userAgent, n.platform || '', (n.languages || []).join(','),
    -new Date().getTimezoneOffset(), screen.width + 'x' + screen.height,
    screen.colorDepth, n.hardwareConcurrency || 0,
    n.deviceMemory || 0, n.maxTouchPoints || 0,
    'ontouchstart' in window ? 1 : 0
  ].join('|')
}

// 已安装插件清单（P2-2 sets 输入；无插件接口/被禁用时为空数组）。
function pluginsList() {
  try {
    const ps = navigator.plugins
    if (!ps || !ps.length) return []
    const out = []
    for (let i = 0; i < ps.length && i < 32; i++) out.push(String(ps[i].name || ''))
    return out.filter(Boolean)
  } catch { return [] }
}

// detectFlags 汇总 P1 检测族命中项（单项失败/超时都降级为空，绝不阻塞上报）。
// 灰度纪律：这些 flag 服务端默认只记录不计分（FP_SCORE_SHADOW），先看假阳性率再决定计分。
async function detectFlags() {
  const jobs = [
    cleanEnvDetect().then(r => r.flags).catch(() => []),
    claimsDetect().then(r => r.flags).catch(() => []),
    forensicsDetect().then(r => r.flags).catch(() => []),
    botdDetect().then(r => r.flags).catch(() => [])
  ]
  try {
    const results = await Promise.race([
      Promise.all(jobs),
      new Promise(resolve => setTimeout(() => resolve([]), DETECT_TIMEOUT_MS))
    ])
    return (Array.isArray(results) ? results : []).flat().filter(Boolean)
  } catch {
    return []
  }
}

// 采集 + 上报（每会话一次，成功后打标；失败 8 秒后重试一次）。
// 旧版本先打标再请求，遇到服务重启等瞬时失败会整会话不再上报——这里修正。
export async function reportFingerprint() {
  try {
    if (sessionStorage.getItem(REPORTED_KEY)) return
    const [canvas, webgl, rtc, audio, fonts] = await Promise.all(
      [canvasFp(), webglFp(), webrtcIPs(), audioFp(), fontsFp()])
    const fp = await sha256([canvas.hash, webgl.hash, audio, fonts.hash, envSignals()].join('~'))
    try { localStorage.setItem(FP_KEY, fp) } catch {}
    const flags = [...envAudit(), ...await detectFlags()]
    // P2-5 时区：IANA 名 + 偏移分钟（服务端与 IP 归属国做跨洲核验）
    let tz = ''
    try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || '' } catch {}
    const res = await fetch('/api/v1/fp/report', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        fp, canvas: canvas.hash, webgl: webgl.hash, audio, fonts: fonts.hash, webrtc: rtc,
        renderer: webgl.renderer, screen: screen.width + 'x' + screen.height,
        canvas_phash: canvas.phash,   // P2-1 感知哈希（可选字段，旧服务端忽略）
        sets: {                        // P2-2 原始清单：服务端算 MinHash 签名（可选字段）
          fonts: fonts.detected,
          webgl_exts: webgl.exts,
          plugins: pluginsList()
        },
        tz,                            // P2-5（可选字段，旧服务端忽略）
        tz_offset_min: -new Date().getTimezoneOffset(),
        components: {
          canvas: canvas.hash, webgl: webgl.hash, audio, fonts: fonts.hash,
          screen: screen.width + 'x' + screen.height,
          renderer: webgl.renderer,
          dpr: String(window.devicePixelRatio)   // 软信号：不参与身份哈希
        },
        flags
      })
    })
    if (!res.ok) throw new Error('http ' + res.status)
    sessionStorage.setItem(REPORTED_KEY, '1')
    const d = await res.json().catch(() => ({}))
    if (d.banned) location.reload()
  } catch {
    // 上报失败（如服务重启瞬间）不阻塞站点，稍后重试（最多 3 次）
    if (!sessionStorage.getItem(REPORTED_KEY) && reportRetry++ < 3) {
      setTimeout(() => reportFingerprint(), 8000)
    }
  }
}
