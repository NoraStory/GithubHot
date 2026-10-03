// 设备指纹采集（第二层）：Canvas 渲染噪声 + WebGL 显卡信息 + 音频栈 + 字体枚举
// + WebRTC 真实 IP + 屏幕/环境参数，分量哈希后融合成设备指纹，每会话上报一次。
// 第三层环境核验：UA/platform/时区/语言一致性 + 无头浏览器与自动化框架痕迹检测，
// 命中项随上报传给服务端存档并计违规分。
// 采集全程异步静默，失败任何一项都不影响站点功能。

const FP_KEY = 'gh_fp'
const REPORTED_KEY = 'gh_fp_reported'
let reportRetry = 0

export function deviceFp() {
  try { return localStorage.getItem(FP_KEY) || '' } catch { return '' }
}

async function sha256(text) {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
  return btoa(String.fromCharCode(...new Uint8Array(buf).slice(0, 18)))
}

// Canvas 指纹：emoji + 渐变文本渲染的 GPU 驱动级差异
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
    return await sha256(c.toDataURL())
  } catch { return '' }
}

// WebGL 指纹：厂商/显卡（UNMASKED）、扩展列表、着色精度、最大纹理。
// 返回 {hash, renderer}：renderer 原文入档案，便于管理端直接辨认显卡型号。
function webglInfo() {
  try {
    const c = document.createElement('canvas')
    const gl = c.getContext('webgl') || c.getContext('experimental-webgl')
    if (!gl) return { hash: '', renderer: '' }
    const dbg = gl.getExtension('WEBGL_debug_renderer_info')
    const vendor = dbg ? gl.getParameter(dbg.UNMASKED_VENDOR_WEBGL) : gl.getParameter(gl.VENDOR)
    const renderer = dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER)
    const exts = gl.getSupportedExtensions() || []
    const prec = gl.getShaderPrecisionFormat(gl.VERTEX_SHADER, gl.HIGH_FLOAT)
    const raw = [vendor, renderer, exts.join(','), gl.getParameter(gl.MAX_TEXTURE_SIZE),
      gl.getParameter(gl.MAX_VERTEX_ATTRIBS), prec ? `${prec.precision}/${prec.rangeMin}/${prec.rangeMax}` : ''].join('|')
    return { hash: raw, renderer: String(renderer || '') }
  } catch { return { hash: '', renderer: '' } }
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

// 字体枚举指纹：探测常见中英文字体的实际渲染宽度差异（像素级）
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
    return await sha256('fonts:' + detected.join(','))
  } catch { return '' }
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

// 环境参数信号
function envSignals() {
  const n = navigator
  return [
    n.userAgent, n.platform || '', (n.languages || []).join(','),
    -new Date().getTimezoneOffset(), screen.width + 'x' + screen.height,
    screen.colorDepth, window.devicePixelRatio, n.hardwareConcurrency || 0,
    n.deviceMemory || 0, n.maxTouchPoints || 0,
    'ontouchstart' in window ? 1 : 0
  ].join('|')
}

// 采集 + 上报（每会话一次，成功后打标；失败 8 秒后重试一次）。
// 旧版本先打标再请求，遇到服务重启等瞬时失败会整会话不再上报——这里修正。
export async function reportFingerprint() {
  try {
    if (sessionStorage.getItem(REPORTED_KEY)) return
    const [canvas, webgl, rtc, audio, fonts] = await Promise.all(
      [canvasFp(), webglFp(), webrtcIPs(), audioFp(), fontsFp()])
    const fp = await sha256([canvas, webgl.hash, audio, fonts, envSignals()].join('~'))
    try { localStorage.setItem(FP_KEY, fp) } catch {}
    const flags = envAudit()
    const res = await fetch('/api/v1/fp/report', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        fp, canvas, webgl: webgl.hash, audio, fonts, webrtc: rtc,
        renderer: webgl.renderer, screen: screen.width + 'x' + screen.height,
        components: {
          canvas, webgl: webgl.hash, audio, fonts,
          screen: screen.width + 'x' + screen.height,
          renderer: webgl.renderer
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
