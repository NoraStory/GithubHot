// API 客户端：统一 fetch 封装。
// 管理端鉴权走服务端会话 Cookie（HttpOnly，登录后由 /api/v1/admin/login 下发），
// 这里不再存任何令牌；adminAuthed 供菜单/守卫做登录态响应式判断。
// 所有请求自动带 X-Device-Fp（设备指纹，第三层身份核验用）。
import { ref } from 'vue'
import { deviceFp } from './fingerprint'

export const adminAuthed = ref(false)

export async function checkAdminSession() {
  try {
    const res = await fetch('/api/v1/admin/session')
    adminAuthed.value = res.ok
    return res.ok
  } catch {
    adminAuthed.value = false
    return false
  }
}

// 登录用 fp：指纹采集是异步重型管线，直达登录页快速提交时可能尚未落盘 →
// 等待最多 8s；仍无则用会话级随机 fp（登录 PoW 是防爆破成本，不依赖身份绑定）
async function loginFp() {
  const { deviceFp } = await import('./fingerprint')
  let fp = deviceFp()
  const t0 = Date.now()
  while (!fp && Date.now() - t0 < 8000) {
    await new Promise((r) => setTimeout(r, 300))
    fp = deviceFp()
  }
  if (fp) return fp
  try {
    let fb = sessionStorage.getItem('gh_login_fp')
    if (!fb) {
      fb = 'login' + Math.random().toString(36).slice(2, 12) + Date.now().toString(36)
      sessionStorage.setItem('gh_login_fp', fb)
    }
    return fb
  } catch { return 'login' + Date.now().toString(36) }
}

export async function adminLogin(password, remember) {
  // P4-3 管理端 PoW 纵深：服务端启用（难度>0）时带解登录；未启用时 solveAltcha
  // 返回 null → 不带 altcha/fp 字段，行为与之前完全一致
  const { solveAltcha } = await import('./altcha')
  const fp = await loginFp()
  const altcha = await solveAltcha(fp)
  const res = await fetch('/api/v1/admin/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password, remember: !!remember, fp: fp || undefined, altcha: altcha || undefined })
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || '登录失败')
  adminAuthed.value = true
  return data
}

export async function adminLogout() {
  try {
    await fetch('/api/v1/admin/logout', { method: 'POST' })
  } finally {
    adminAuthed.value = false
  }
}

async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
  const fp = deviceFp()
  if (fp) headers['X-Device-Fp'] = fp
  const res = await fetch(path, { ...options, headers })
  const data = await res.json().catch(() => ({}))
  if (res.status === 401 && path.startsWith('/api/v1/admin')) adminAuthed.value = false
  if (!res.ok && data.error) throw new Error(data.error)
  return data
}

export const api = {
  get: (p) => request(p),
  post: (p, body) => request(p, { method: 'POST', body: JSON.stringify(body || {}) }),
  del: (p) => request(p, { method: 'DELETE' }),
  raw: async (p) => {
    const res = await fetch(p)
    return res.text()
  }
}

// 站点配置（歌单等）
export async function loadSiteConfig() {
  try {
    return await api.get('/api/v1/site/config')
  } catch {
    return { music: [] }
  }
}

// ---- P4-2 WebAuthn 通行密钥 ----
const b64uToBuf = (s) => {
  const pad = s + '='.repeat((4 - (s.length % 4)) % 4)
  const bin = atob(pad.replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(bin, c => c.charCodeAt(0)).buffer
}
// WebAuthn JSON 线格式 = base64url（无 padding）；go-webauthn 字段均为 URLEncodedBase64
const bufToB64u = (buf) => {
  const bin = String.fromCharCode(...new Uint8Array(buf))
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export const webAuthnAvailable = () => typeof window.PublicKeyCredential !== 'undefined'

// 注册（需已登录）：begin → navigator.credentials.create → finish
export async function passkeyBeginRegister() {
  const options = await api.post('/api/v1/admin/passkey/begin-register', {})
  const pk = options.publicKey
  pk.challenge = b64uToBuf(pk.challenge)
  pk.user.id = b64uToBuf(pk.user.id)
  ;(pk.excludeCredentials || []).forEach(c => { c.id = b64uToBuf(c.id) })
  const cred = await navigator.credentials.create({ publicKey: pk })
  const body = {
    id: cred.id, rawId: bufToB64u(cred.rawId), type: cred.type,
    response: {
      attestationObject: bufToB64u(cred.response.attestationObject),
      clientDataJSON: bufToB64u(cred.response.clientDataJSON),
    }
  }
  return api.post('/api/v1/admin/passkey/finish-register', body)
}

// 免密登录（守卫外）：begin → navigator.credentials.get → finish（成功即建会话）
export async function passkeyLogin() {
  const { token, options } = await api.post('/api/v1/admin/passkey/begin-login', {})
  const pk = options.publicKey
  pk.challenge = b64uToBuf(pk.challenge)
  ;(pk.allowCredentials || []).forEach(c => { c.id = b64uToBuf(c.id) })
  const assertion = await navigator.credentials.get({ publicKey: pk })
  const body = {
    token,
    id: assertion.id, rawId: bufToB64u(assertion.rawId), type: assertion.type,
    response: {
      clientDataJSON: bufToB64u(assertion.response.clientDataJSON),
      authenticatorData: bufToB64u(assertion.response.authenticatorData),
      signature: bufToB64u(assertion.response.signature),
      userHandle: assertion.response.userHandle ? bufToB64u(assertion.response.userHandle) : null,
    }
  }
  const res = await api.post('/api/v1/admin/passkey/finish-login', body)
  adminAuthed.value = true
  return res
}

export async function listPasskeys() {
  return api.get('/api/v1/admin/passkey/credentials')
}

export async function deletePasskey(id) {
  return api.del('/api/v1/admin/passkey/credentials/' + id)
}
