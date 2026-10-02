// API 客户端：统一 fetch 封装 + 管理端令牌注入
const TOKEN_KEY = 'githubhot_admin_token'

export function getToken() { return localStorage.getItem(TOKEN_KEY) || '' }
export function setToken(t) { localStorage.setItem(TOKEN_KEY, t || '') }

async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
  const t = getToken()
  if (t) headers['X-Admin-Token'] = t
  const res = await fetch(path, { ...options, headers })
  const data = await res.json().catch(() => ({}))
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
