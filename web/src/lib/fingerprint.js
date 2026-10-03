// 设备指纹采集（第二层）：Canvas 渲染噪声 + WebGL 显卡信息 + WebRTC 本地 IP
// + 屏幕/环境参数，分量哈希后融合成设备指纹，每会话上报一次。
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

// WebGL 指纹：厂商/显卡（UNMASKED）、扩展列表、着色精度、最大纹理
async function webglFp() {
  try {
    const c = document.createElement('canvas')
    const gl = c.getContext('webgl') || c.getContext('experimental-webgl')
    if (!gl) return ''
    const dbg = gl.getExtension('WEBGL_debug_renderer_info')
    const vendor = dbg ? gl.getParameter(dbg.UNMASKED_VENDOR_WEBGL) : gl.getParameter(gl.VENDOR)
    const renderer = dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER)
    const exts = gl.getSupportedExtensions() || []
    const prec = gl.getShaderPrecisionFormat(gl.VERTEX_SHADER, gl.HIGH_FLOAT)
    const raw = [vendor, renderer, exts.join(','), gl.getParameter(gl.MAX_TEXTURE_SIZE),
      gl.getParameter(gl.MAX_VERTEX_ATTRIBS), prec ? `${prec.precision}/${prec.rangeMin}/${prec.rangeMax}` : ''].join('|')
    return await sha256(raw)
  } catch { return '' }
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

// 环境自洽（第三层核验辅助）：navigator.platform 与 UA 声明的系统应一致
function coherent() {
  try {
    const ua = navigator.userAgent.toLowerCase()
    const p = (navigator.platform || '').toLowerCase()
    if (!p) return true
    const saysWin = ua.includes('windows'), saysMac = ua.includes('mac os') || ua.includes('macintosh')
    const saysLinux = ua.includes('linux') && !ua.includes('android')
    const saysAndroid = ua.includes('android')
    if (p.startsWith('win')) return saysWin
    if (p.startsWith('mac')) return saysMac
    if (p.includes('android')) return saysAndroid
    if (p.includes('linux')) return saysLinux
    return true
  } catch { return true }
}

// 采集 + 上报（每会话一次，成功后打标；失败 8 秒后重试一次）。
// 旧版本先打标再请求，遇到服务重启等瞬时失败会整会话不再上报——这里修正。
export async function reportFingerprint() {
  try {
    if (sessionStorage.getItem(REPORTED_KEY)) return
    const [canvas, webgl, rtc] = await Promise.all([canvasFp(), webglFp(), webrtcIPs()])
    const fp = await sha256([canvas, webgl, envSignals()].join('~'))
    try { localStorage.setItem(FP_KEY, fp) } catch {}
    const res = await fetch('/api/v1/fp/report', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        fp, canvas, webgl, webrtc: rtc,
        renderer: '', screen: screen.width + 'x' + screen.height,
        coherent: coherent()
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
