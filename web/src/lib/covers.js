// 封面/头像生成器：
// - GitHub 项目 → 真实仓库所有者头像（github.com/{owner}.png），失败回退渐变字标
// - AI 资讯 → 按分类 emoji 生成的有内容封面（不再是无字纯渐变）
const TAG_EMOJI = [
  ['模型发布', '🚀'], ['安全', '🛡️'], ['政策监管', '⚖️'], ['融资并购', '💰'],
  ['硬件', '🔧'], ['开发者工具', '🧰'], ['行业动态', '📈']
]

export function emojiForTags(tags) {
  for (const [t, e] of TAG_EMOJI) if ((tags || []).includes(t)) return e
  return '🤖'
}

function hashCode(s) {
  let h = 0
  for (const c of String(s)) h = (h * 31 + c.charCodeAt(0)) % 360
  return h
}

function gradientSvg(title, emoji, w = 600, h2 = 336) {
  const h = hashCode(title)
  const a = `hsl(${h}, 45%, 58%)`
  const b = `hsl(${(h + 45) % 360}, 50%, 40%)`
  const label = String(title || '').slice(0, 14)
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='${w}' height='${h2}'>` +
    `<defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'>` +
    `<stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs>` +
    `<rect width='${w}' height='${h2}' fill='url(#g)'/>` +
    `<circle cx='${w * 0.82}' cy='${h2 * 0.2}' r='${h2 * 0.32}' fill='rgba(255,255,255,0.10)'/>` +
    `<circle cx='${w * 0.15}' cy='${h2 * 0.9}' r='${h2 * 0.26}' fill='rgba(0,0,0,0.10)'/>` +
    `<text x='50%' y='52%' font-size='${Math.round(h2 * 0.36)}' text-anchor='middle'>${emoji}</text>` +
    `<text x='50%' y='84%' font-size='${Math.round(h2 * 0.09)}' fill='rgba(255,255,255,0.85)' text-anchor='middle' font-family='monospace'>${label}</text>` +
    `</svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

// AI 资讯封面：分类 emoji + 渐变 + 标题摘录
export function newsCover(title = '', tags = []) {
  return gradientSvg(title, emojiForTags(tags))
}

// GitHub 仓库字标封面（头像加载失败时的回退）
export function repoCover(fullName = '') {
  const name = String(fullName || '')
  const initial = (name.split('/')[1] || name || '?').slice(0, 1).toUpperCase()
  return gradientSvg(name, initial)
}

// GitHub 仓库所有者真实头像
export function repoAvatar(fullName = '') {
  const owner = String(fullName || '').split('/')[0]
  return owner ? `https://github.com/${owner}.png?size=96` : ''
}

// 期刊封面：📰 + 期号
export function digestCover(date = '') {
  return gradientSvg(date + ' 双热点报告', '📰')
}
