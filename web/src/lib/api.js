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

export async function adminLogin(password, remember) {
  const res = await fetch('/api/v1/admin/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password, remember: !!remember })
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
