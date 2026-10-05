<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { api } from '../lib/api'
import FlagStatsCard from './components/FlagStatsCard.vue'
import { flagInfo } from './flagLabels'

const data = ref(null)
const error = ref('')
const banIp = ref('')
const banHours = ref(24)
const banReason = ref('')
const ipFilter = ref('')
// 检测 flag 筛选（点击「检测命中统计」条目联动）
const flagFilter = ref('')
// 时间筛选：违规事件只看近 N 小时（0 = 全部）
const eventsHours = ref(0)
const RANGES = [[0, '全部时间'], [1, '近 1 小时'], [24, '近 24 小时'], [168, '近 7 天']]

// IP 下钻详情
const detail = ref(null)
const detailLoading = ref(false)
const detailPanel = ref(null)   // 详情面板 DOM 引用（点击 IP 后滚动定位）
const detailError = ref('')

const match = (ip) => !ipFilter.value || (ip || '').includes(ipFilter.value.trim())

const talkers = computed(() => (data.value?.talkers || []).filter(t => match(t.ip)))
const bansFiltered = computed(() => (data.value?.bans || []).filter(b => match(b.IP)))
const eventsFiltered = computed(() => (data.value?.events || []).filter(e => match(e.IP)))
// 指纹筛选：除 IP 外同时匹配型号/显卡/UA/指纹串；flag 筛选为并集（命中该检测项）
const fpsFiltered = computed(() => (data.value?.fingerprints || []).filter(f => {
  if (flagFilter.value && !(f.Flags || []).includes(flagFilter.value)) return false
  const q = ipFilter.value.trim()
  if (!q) return true
  const hay = [...f.IPs, f.UA, f.Components?.model || '', f.Components?.renderer || '', f.Fingerprint].join(' ')
  return hay.includes(q)
}))

// ---------- 设备识别与统计 ----------
// APP 行的 UA 形如 "GithubHot-APP (vivo V2505A)"、components.client=android；
// 网页行解析浏览器名 + 系统。
function clientInfo(f) {
  const ua = f.UA || ''
  const comp = f.Components || {}
  if (ua.startsWith('GithubHot-APP') || comp.client === 'android') {
    const model = comp.model || ua.match(/GithubHot-APP \((.+)\)/)?.[1] || 'Android 设备'
    return { kind: 'app', icon: '📱', label: model }
  }
  const m = ua.match(/(Edg|Chrome|Firefox|Safari)\/([\d.]+)/)
  const browser = m ? ({ Edg: 'Edge', Chrome: 'Chrome', Firefox: 'Firefox', Safari: 'Safari' })[m[1]] : '浏览器'
  const os = ua.includes('Windows') ? 'Windows' : ua.includes('Mac OS') ? 'macOS'
    : ua.includes('iPhone') || ua.includes('iPad') ? 'iOS'
    : ua.includes('Android') ? 'Android' : ua.includes('Linux') ? 'Linux' : ''
  return { kind: 'web', icon: '🌐', label: browser + (os ? ' · ' + os : '') }
}

const stats = computed(() => {
  const fps = data.value?.fingerprints || []
  let app = 0
  for (const f of fps) if (clientInfo(f).kind === 'app') app++
  return {
    total: fps.length,
    app,
    web: fps.length - app,
    multiIp: fps.filter(f => f.IPs.length > 1).length,
    hot: fps.filter(f => f.IPs.length > 8).length,
  }
})

// 风险行：高频换 IP 或命中 APP 威胁
const isRisk = f => f.IPs.length > 8 || (f.Flags || []).some(x => x.startsWith('app-'))

async function load() {
  error.value = ''
  try {
    const q = eventsHours.value > 0 ? '?hours=' + eventsHours.value : ''
    data.value = await api.get('/api/v1/admin/ipguard/summary' + q)
  } catch (e) {
    error.value = e.message
  }
}

function onHoursChange() {
  load()
  if (detail.value) showIP(detail.value.ip)
}

async function showIP(ip) {
  if (!ip) return
  detailError.value = ''
  detailLoading.value = true
  detail.value = null
  // 面板位于页面上部，而 IP 链接分布在下方多个列表：加载后滚动到面板，
  // 否则「点了没反应」（内容在视口外）
  await nextTick()
  detailPanel.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  try {
    const q = eventsHours.value > 0 ? '&hours=' + eventsHours.value : ''
    const d = await api.get('/api/v1/admin/ipguard/ip?ip=' + encodeURIComponent(ip) + q)
    if (d && d.enabled === false) {
      detailError.value = 'IP 防护未启用'
    } else {
      detail.value = d
    }
  } catch (e) {
    detailError.value = e.message
  } finally {
    detailLoading.value = false
  }
}

function closeDetail() {
  detail.value = null
}

// 详情面板：该 IP 所有指纹的环境核验命中并集
const detailFlags = computed(() => {
  const set = []
  for (const f of detail.value?.fingerprints || []) {
    for (const x of f.Flags || []) if (!set.includes(x)) set.push(x)
  }
  return set
})

async function unban(ip) {
  if (!confirm(`解除对 ${ip} 的封禁？`)) return
  try {
    await api.post('/api/v1/admin/ipguard/unban', { ip })
    await load()
    if (detail.value?.ip === ip) await showIP(ip)
  } catch (e) { alert(e.message) }
}

async function ban() {
  if (!banIp.value.trim()) return
  try {
    await api.post('/api/v1/admin/ipguard/ban', { ip: banIp.value.trim(), hours: banHours.value, reason: banReason.value || '手动封禁' })
    banIp.value = ''; banReason.value = ''
    await load()
  } catch (e) { alert(e.message) }
}

const fmt = t => t ? new Date(t).toLocaleString('zh-CN', { hour12: false }) : '-'
const short = s => s ? s.slice(0, 12) + '…' : '-'
const comp = s => s ? s.slice(0, 8) : '-'
const levelName = n => ['', '30 分钟', '24 小时', '7 天', '30 天'][Math.min(n, 4)] || (n + ' 级')

// 环境核验命中项的中文说明见 ./flagLabels.js（与「检测命中统计」卡共用同一份文案）

// 显卡名截断：ANGLE 长串取括号内首段（如 "ANGLE (AMD, AMD Radeon ...)" → "AMD Radeon ..." 截断）
const shortGpu = s => {
  if (!s) return '-'
  const m = s.match(/^ANGLE \(([^,]+), (.+?)\)/)
  const t = m ? m[2] : s
  return t.length > 26 ? t.slice(0, 26) + '…' : t
}

onMounted(() => {
  load()
  tickTimer = setInterval(() => { nowTs.value = Date.now() }, 1000)
})
onUnmounted(() => clearInterval(tickTimer))

// ---------- 下钻增强：全部由下钻响应聚合，无新增接口 ----------
const expandedFp = ref('')      // 展开的指纹行
const nowTs = ref(Date.now())   // 封禁倒计时用（秒级刷新）
let tickTimer = null

const detailStats = computed(() => {
  const d = detail.value
  if (!d) return null
  const fps = d.fingerprints || []
  const evs = d.events || []
  const times = [...fps.flatMap(f => [f.FirstSeen, f.LastSeen]),
                 d.profile?.FirstSeen, d.profile?.LastSeen]
    .filter(Boolean).map(t => new Date(t).getTime())
  const first = times.length ? Math.min(...times) : null
  const last = times.length ? Math.max(...times) : null
  const spanDays = first && last ? (last - first) / 86400000 : 0
  const reqs = d.profile?.Reqs || 0
  return {
    fps: fps.length,
    multiIp: fps.filter(f => (f.IPs || []).length > 1).length,
    riskFps: fps.filter(isRisk).length,
    leaks: fps.filter(f => (f.Webrtc || []).length > 0).length,
    events: evs.length,
    score: evs.reduce((s, e) => s + (e.Score || 0), 0),
    spanDays,
    reqs,
    rpm: spanDays > 0 ? reqs / (spanDays * 1440) : null,
  }
})

// 违规事件按类型聚合（积分降序）
const eventKinds = computed(() => {
  const m = new Map()
  for (const e of detail.value?.events || []) {
    const k = m.get(e.Kind) || { kind: e.Kind, count: 0, score: 0 }
    k.count++
    k.score += e.Score || 0
    m.set(e.Kind, k)
  }
  return [...m.values()].sort((a, b) => b.score - a.score)
})

// 同一 UA 被多个指纹使用 → 指纹漂移/多开线索
const sameUaCount = computed(() => {
  const m = new Map()
  for (const f of detail.value?.fingerprints || []) {
    const ua = f.UA || ''
    m.set(ua, (m.get(ua) || 0) + 1)
  }
  let n = 0
  for (const c of m.values()) if (c > 1) n += c
  return n
})

// 封禁剩余倒计时
const banRemain = computed(() => {
  const b = detail.value?.ban
  if (!b) return null
  const ms = new Date(b.ExpiresAt).getTime() - nowTs.value
  if (ms <= 0) return { text: '已到期', sec: 0 }
  const s = Math.floor(ms / 1000)
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60)
  return { text: (h ? h + ' 小时 ' : '') + m + ' 分 ' + (s % 60) + ' 秒', sec: s }
})

// 封禁总时长（小时）
const banSpanText = computed(() => {
  const b = detail.value?.ban
  if (!b) return '-'
  const h = (new Date(b.ExpiresAt).getTime() - new Date(b.BannedAt).getTime()) / 3600000
  return h >= 24 ? (h / 24).toFixed(1) + ' 天' : h.toFixed(1) + ' 小时'
})

// WebRTC 泄漏的 IP 与当前访问 IP 不一致 → 代理/VPN 线索
const rtcLeak = f => (f.Webrtc || []).some(x => x && x !== detail.value?.ip)
</script>

<template>
  <div class="page-enter">
    <div class="head">
      <h2>🛡 IP 防护（三层身份识别）</h2>
      <button class="btn" @click="load">刷新</button>
    </div>
    <div v-if="error" class="err">{{ error }}</div>
    <div v-if="!data" class="loading">加载中</div>
    <template v-else>
      <!-- 手动封禁 -->
      <div class="card ban-form">
        <input v-model="banIp" placeholder="要封禁的 IP">
        <input v-model.number="banHours" type="number" min="1" title="封禁时长（小时）">
        <span class="unit">小时</span>
        <input v-model="banReason" placeholder="原因（可选）" class="reason">
        <button class="btn danger" @click="ban">封禁</button>
      </div>

      <!-- 筛选 -->
      <div class="card filter-bar">
        <label>🔍 IP 筛选</label>
        <input v-model="ipFilter" placeholder="输入 IP 片段，同时过滤下方所有列表">
        <label>⏱ 时间</label>
        <select v-model.number="eventsHours" class="range-select" @change="onHoursChange">
          <option v-for="[h, label] in RANGES" :key="h" :value="h">{{ label }}</option>
        </select>
        <button v-if="ipFilter" class="btn small" @click="ipFilter = ''">清除</button>
        <span v-if="flagFilter" class="chip flag-chip">
          🧪 {{ flagInfo(flagFilter)[0] }}
          <button class="x" title="取消 flag 筛选" @click="flagFilter = ''">×</button>
        </span>
        <span class="hint">点击任意 IP 可下钻查看完整档案与指纹；筛选同时匹配型号/显卡/UA；时间筛选作用于违规事件</span>
      </div>

      <!-- 统计概览 -->
      <div v-if="data" class="stat-row">
        <div class="stat"><b class="num">{{ stats.total }}</b><span>活跃指纹</span></div>
        <div class="stat"><b>📱 {{ stats.app }}</b><span>APP 设备</span></div>
        <div class="stat"><b>🌐 {{ stats.web }}</b><span>浏览器访客</span></div>
        <div class="stat" :class="{ warn: stats.multiIp }"><b class="num">⚠ {{ stats.multiIp }}</b><span>多 IP 指纹</span></div>
        <div class="stat" :class="{ hot: stats.hot }"><b class="num">🔥 {{ stats.hot }}</b><span>高频换 IP</span></div>
      </div>

      <!-- 检测命中统计（P0-4：灰度观察各检测 flag 的假阳性率） -->
      <FlagStatsCard
        v-if="data"
        :stats="data.flag_stats || []"
        :active="flagFilter"
        @select="k => flagFilter = k"
      />

      <!-- IP 下钻详情 -->
      <div v-if="detail || detailLoading || detailError" ref="detailPanel" :key="detail?.ip || 'pending'" class="card detail-panel">
        <div class="detail-head">
          <h3>🔎 IP 详情：{{ detail?.ip || ipFilter }}</h3>
          <div>
            <button v-if="detail?.ban" class="btn small" @click="unban(detail.ip)">解封</button>
            <button class="btn small" @click="closeDetail">关闭</button>
          </div>
        </div>
        <div v-if="detailLoading" class="empty">加载中…</div>
        <div v-else-if="detailError" class="err">{{ detailError }}</div>
        <template v-else-if="detail">
          <!-- 档案 -->
          <div v-if="detail.profile" class="kv">
            <div><span>首次出现</span><b>{{ fmt(detail.profile.FirstSeen) }}</b></div>
            <div><span>最近活跃</span><b>{{ fmt(detail.profile.LastSeen) }}</b></div>
            <div><span>总请求</span><b class="num">{{ detail.profile.Reqs }}</b></div>
            <div><span>最近 UA</span><b class="ua">{{ detail.profile.UALast || '-' }}</b></div>
          </div>
          <div v-else class="empty">该 IP 暂无档案记录</div>
          <details v-if="detail.profile && (detail.profile.UASet || []).length > 1" class="ua-set">
            <summary>历史 UA 集合（{{ detail.profile.UASet.length }} 个{{ detail.profile.UASet.length > 3 ? '，UA 频繁变化是代理池特征' : '' }}）</summary>
            <ul><li v-for="(u, i) in detail.profile.UASet" :key="i">{{ u }}</li></ul>
          </details>

          <!-- 下钻概览（由本次响应聚合） -->
          <div v-if="detailStats" class="stat-row mini">
            <div class="stat"><b class="num">{{ detailStats.fps }}</b><span>关联指纹</span></div>
            <div class="stat" :class="{ warn: detailStats.multiIp }"><b class="num">{{ detailStats.multiIp }}</b><span>多 IP 指纹</span></div>
            <div class="stat" :class="{ warn: detailStats.riskFps }"><b class="num">{{ detailStats.riskFps }}</b><span>风险指纹</span></div>
            <div class="stat" :class="{ warn: detailStats.leaks }"><b class="num">{{ detailStats.leaks }}</b><span>WebRTC 泄漏</span></div>
            <div class="stat"><b class="num">{{ detailStats.events }}</b><span>违规事件</span></div>
            <div class="stat" :class="{ hot: detailStats.score >= 100 }"><b class="num">{{ detailStats.score }}</b><span>违规积分合计</span></div>
            <div class="stat"><b class="num">{{ detailStats.spanDays >= 1 ? detailStats.spanDays.toFixed(1) + ' 天' : '不足 1 天' }}</b><span>活跃跨度</span></div>
            <div class="stat"><b class="num">{{ detailStats.rpm == null ? '-' : detailStats.rpm.toFixed(2) }}</b><span>平均请求/分</span></div>
          </div>
          <div v-if="sameUaCount" class="hint-line">
            ⚠ 有 {{ sameUaCount }} 个指纹共用同一 UA —— 可能是同设备的指纹漂移或多开
          </div>

          <!-- 第三层环境核验 -->
          <h4>环境核验（第三层）
            <span v-if="!detailFlags.length" class="ok-chip">✓ 通过</span>
            <span v-else class="bad-chip">{{ detailFlags.length }} 项命中</span>
          </h4>
          <div v-if="detailFlags.length" class="flag-list">
            <div v-for="f in detailFlags" :key="f" class="flag-item">
              <span class="flag-name">{{ flagInfo(f)[0] }}</span>
              <span class="flag-desc">{{ flagInfo(f)[1] }}</span>
              <code class="flag-code">{{ f }}</code>
            </div>
          </div>
          <div v-else class="empty">UA / Client Hints / 时区语言 / 无头与自动化检测均未发现异常</div>

          <!-- 当前封禁 -->
          <template v-if="detail.ban">
            <h4>当前封禁</h4>
            <div class="kv">
              <div><span>违规次数</span><b class="num">{{ detail.ban.Strikes }}</b></div>
              <div><span>档位</span><b>{{ levelName(detail.ban.Level) }}</b></div>
              <div><span>原因</span><b>{{ detail.ban.Reason }}</b></div>
              <div><span>封禁时长</span><b>{{ banSpanText }}</b></div>
              <div><span>剩余</span><b :class="{ warn: banRemain && banRemain.sec > 0 }">{{ banRemain?.text }}</b></div>
              <div><span>封禁时间</span><b>{{ fmt(detail.ban.BannedAt) }}</b></div>
              <div><span>解禁时间</span><b>{{ fmt(detail.ban.ExpiresAt) }}</b></div>
            </div>
          </template>

          <!-- 关联指纹 -->
          <h4>关联设备指纹（{{ detail.fingerprints.length }}）</h4>
          <table v-if="detail.fingerprints.length" class="fp-table">
            <thead><tr>
              <th>融合指纹</th><th>设备</th><th>屏幕</th><th>显卡</th>
              <th>Canvas</th><th>WebGL</th><th>音频</th><th>字体</th>
              <th>关联 IP 数</th><th>WebRTC 真实 IP</th><th>最近使用</th><th>上报次数</th>
            </tr></thead>
            <tbody>
              <template v-for="f in detail.fingerprints" :key="f.Fingerprint">
              <tr class="fp-row" :class="{ 'row-risk': isRisk(f) }"
                  @click="expandedFp = expandedFp === f.Fingerprint ? '' : f.Fingerprint">
                <td class="mono" :title="f.Fingerprint + '\n' + f.UA">
                  <span class="caret">{{ expandedFp === f.Fingerprint ? '▾' : '▸' }}</span>
                  {{ short(f.Fingerprint) }}<span v-if="(f.Flags || []).length" class="warn" :title="'核验命中: ' + f.Flags.map(x => flagInfo(x)[0]).join('、')">⚠</span>
                </td>
                <td><span class="dev" :class="clientInfo(f).kind" :title="f.UA"><span class="dev-icon">{{ clientInfo(f).icon }}</span>{{ clientInfo(f).label }}</span></td>
                <td class="mono comp">{{ f.Components?.screen || '-' }}</td>
                <td class="mono comp" :title="f.Components?.renderer">{{ shortGpu(f.Components?.renderer) }}</td>
                <td class="mono comp" :title="f.Components?.canvas">{{ comp(f.Components?.canvas) }}</td>
                <td class="mono comp" :title="'WebGL 分量: ' + (f.Components?.webgl || '')">{{ comp(f.Components?.webgl) }}</td>
                <td class="mono comp" :title="f.Components?.audio">{{ comp(f.Components?.audio) }}</td>
                <td class="mono comp" :title="f.Components?.fonts">{{ comp(f.Components?.fonts) }}</td>
                <td class="num" :class="{ hot: f.IPs.length > 8 }">{{ f.IPs.length }}</td>
                <td class="mono rtc" :class="{ warn: rtcLeak(f) }" :title="rtcLeak(f) ? '与访问 IP 不一致，可能经代理/VPN' : (f.Webrtc || []).join('\n')">{{ (f.Webrtc || []).join(', ') || '-' }}</td>
                <td>{{ fmt(f.LastSeen) }}</td>
                <td class="num">{{ f.Hits }}</td>
              </tr>
              <tr v-if="expandedFp === f.Fingerprint" class="fp-expand">
                <td colspan="12">
                  <div class="exp-grid">
                    <div class="exp-col">
                      <h5>完整环境分量（后端原始值）</h5>
                      <div class="kv tight">
                        <div v-for="(v, k) in (f.Components || {})" :key="k">
                          <span>{{ k }}</span><b class="mono break">{{ v }}</b>
                        </div>
                      </div>
                      <h5>WebRTC 真实 IP（STUN 探测）</h5>
                      <div v-if="(f.Webrtc || []).length" class="chip-row">
                        <span v-for="x in f.Webrtc" :key="x" class="chip" :class="{ bad: x !== detail.ip }">
                          {{ x }}{{ x !== detail.ip ? ' ≠ 访问 IP' : ' ✓ 与访问 IP 一致' }}
                        </span>
                      </div>
                      <div v-else class="empty">未泄漏（或浏览器已屏蔽）</div>
                    </div>
                    <div class="exp-col">
                      <h5>该指纹出现过的 IP（{{ (f.IPs || []).length }} 个，点击切换下钻）</h5>
                      <div class="chip-row">
                        <a v-for="x in f.IPs" :key="x" class="ip-link mono" @click.stop="showIP(x)">{{ x }}</a>
                      </div>
                      <h5>时间与身份</h5>
                      <div class="kv tight">
                        <div><span>首见</span><b>{{ fmt(f.FirstSeen) }}</b></div>
                        <div><span>末次</span><b>{{ fmt(f.LastSeen) }}</b></div>
                        <div><span>上报次数</span><b class="num">{{ f.Hits }}</b></div>
                      </div>
                      <div class="kv tight">
                        <div><span>UA</span><b class="ua">{{ f.UA || '-' }}</b></div>
                        <div><span>融合指纹</span><b class="mono break">{{ f.Fingerprint }}</b></div>
                      </div>
                      <template v-if="(f.Flags || []).length">
                        <h5>环境核验命中（{{ f.Flags.length }} 项）</h5>
                        <div class="flag-list">
                          <div v-for="fl in f.Flags" :key="fl" class="flag-item small">
                            <span class="flag-name">{{ flagInfo(fl)[0] }}</span>
                            <span class="flag-desc">{{ flagInfo(fl)[1] }}</span>
                            <code class="flag-code">{{ fl }}</code>
                          </div>
                        </div>
                      </template>
                      <div v-else class="empty">该指纹环境核验全部通过</div>
                    </div>
                  </div>
                </td>
              </tr>
              </template>
            </tbody>
          </table>
          <div v-else class="empty">无关联指纹</div>

          <!-- 违规事件 -->
          <h4>违规事件（近 {{ detail.events.length }} 条）</h4>
          <div v-if="eventKinds.length" class="chip-row kind-row">
            <span v-for="k in eventKinds" :key="k.kind" class="kind-chip">
              <span class="chip">{{ k.kind }}</span> ×{{ k.count }} · 计 +{{ k.score }}
            </span>
          </div>
          <table v-if="detail.events.length">
            <thead><tr><th>时间</th><th>类型</th><th>详情</th><th>积分</th></tr></thead>
            <tbody>
              <tr v-for="e in detail.events" :key="e.ID">
                <td>{{ fmt(e.At) }}</td>
                <td><span class="chip">{{ e.Kind }}</span></td>
                <td>{{ e.Detail }}</td>
                <td class="num">{{ e.Score }}</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty">无违规事件</div>
        </template>
      </div>

      <!-- 封禁列表 -->
      <h3>封禁中（{{ bansFiltered.length }}）</h3>
      <div class="card">
        <div v-if="!bansFiltered.length" class="empty">当前没有封禁中的 IP</div>
        <table v-else>
          <thead><tr><th>IP</th><th>次数</th><th>档位</th><th>原因</th><th>封禁时间</th><th>解禁时间</th><th></th></tr></thead>
          <tbody>
            <tr v-for="b in bansFiltered" :key="b.IP">
              <td class="mono"><a class="ip-link" @click="showIP(b.IP)">{{ b.IP }}</a></td>
              <td class="num">{{ b.Strikes }}</td>
              <td>{{ levelName(b.Level) }}</td>
              <td>{{ b.Reason }}</td>
              <td>{{ fmt(b.BannedAt) }}</td>
              <td>{{ fmt(b.ExpiresAt) }}</td>
              <td><button class="btn small" @click="unban(b.IP)">解封</button></td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Top 访客（第一层 IP 记忆） -->
      <h3>Top 访客（近 1 小时{{ ipFilter ? '，已筛选' : '' }}）</h3>
      <div class="card">
        <div v-if="!talkers.length" class="empty">暂无记录</div>
        <table v-else>
          <thead><tr><th>IP</th><th>请求数</th></tr></thead>
          <tbody>
            <tr v-for="t in talkers" :key="t.ip">
              <td class="mono"><a class="ip-link" @click="showIP(t.ip)">{{ t.ip }}</a></td>
              <td class="num">{{ t.reqs }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 违规事件 -->
      <h3>违规事件（{{ eventsHours ? RANGES.find(r => r[0] === eventsHours)?.[1] : '近 50 条' }}{{ ipFilter ? '，已筛选' : '' }}）</h3>
      <div class="card">
        <div v-if="!eventsFiltered.length" class="empty">暂无违规事件</div>
        <table v-else>
          <thead><tr><th>时间</th><th>IP</th><th>类型</th><th>详情</th><th>积分</th></tr></thead>
          <tbody>
            <tr v-for="e in eventsFiltered" :key="e.ID">
              <td>{{ fmt(e.At) }}</td>
              <td class="mono"><a class="ip-link" @click="showIP(e.IP)">{{ e.IP }}</a></td>
              <td><span class="chip">{{ e.Kind }}</span></td>
              <td>{{ e.Detail }}</td>
              <td class="num">{{ e.Score }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 指纹 ↔ IP 关联（第二层） -->
      <h3>设备指纹 ↔ IP 关联（近 20 个活跃指纹{{ ipFilter ? '，已筛选' : '' }}）</h3>
      <div class="card">
        <div v-if="!fpsFiltered.length" class="empty">暂无指纹记录（访客上报后出现）</div>
        <table v-else class="fp-table">
          <thead><tr>
            <th>融合指纹</th><th>设备</th><th>屏幕</th><th>显卡</th>
            <th>Canvas</th><th>WebGL</th><th>音频</th><th>字体</th>
            <th>关联 IP 数</th><th>WebRTC 真实 IP</th><th>最近使用</th><th>上报次数</th>
          </tr></thead>
          <tbody>
            <tr v-for="f in fpsFiltered" :key="f.Fingerprint" :class="{ 'row-risk': isRisk(f) }">
              <td class="mono" :title="f.Fingerprint + '\n' + f.UA">
                {{ short(f.Fingerprint) }}<span v-if="(f.Flags || []).length" class="warn" :title="'核验命中: ' + f.Flags.map(x => flagInfo(x)[0]).join('、')">⚠</span>
              </td>
              <td><span class="dev" :class="clientInfo(f).kind" :title="f.UA"><span class="dev-icon">{{ clientInfo(f).icon }}</span>{{ clientInfo(f).label }}</span></td>
              <td class="mono comp">{{ f.Components?.screen || '-' }}</td>
              <td class="mono comp" :title="f.Components?.renderer">{{ shortGpu(f.Components?.renderer) }}</td>
              <td class="mono comp" :title="f.Components?.canvas">{{ comp(f.Components?.canvas) }}</td>
              <td class="mono comp" :title="'WebGL 分量: ' + (f.Components?.webgl || '')">{{ comp(f.Components?.webgl) }}</td>
              <td class="mono comp" :title="f.Components?.audio">{{ comp(f.Components?.audio) }}</td>
              <td class="mono comp" :title="f.Components?.fonts">{{ comp(f.Components?.fonts) }}</td>
              <td class="num" :class="{ hot: f.IPs.length > 8 }">
                <a class="ip-link" @click="ipFilter = f.IPs[0] || ''">{{ f.IPs.length }}</a>
              </td>
              <td class="mono rtc" :title="(f.Webrtc || []).join('\n')">{{ (f.Webrtc || []).join(', ') || '-' }}</td>
              <td>{{ fmt(f.LastSeen) }}</td>
              <td class="num">{{ f.Hits }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; }
h2 { font-size: 1.2rem; margin: 1.4rem 0 .8rem; }
h3 { font-size: 1rem; margin: 1.2rem 0 .6rem; color: var(--anzhiyu-secondary); }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1rem 1.3rem; margin-bottom: 1rem; overflow-x: auto; }
table { width: 100%; border-collapse: collapse; font-size: .86rem; }
th, td { text-align: left; padding: 7px 10px; border-bottom: 1px solid var(--anzhiyu-card-border); white-space: nowrap; }
th { color: var(--anzhiyu-gray); font-weight: 500; }
.mono { font-family: Consolas, monospace; font-size: .8rem; }
.num { font-variant-numeric: tabular-nums; text-align: right; }
.hot { color: var(--anzhiyu-red); font-weight: 700; }
.chip { background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 9px; font-size: .75rem; }
.flag-chip { display: inline-flex; align-items: center; gap: 4px; }
.flag-chip .x { border: none; background: transparent; color: inherit; cursor: pointer; font-size: .9rem; line-height: 1; padding: 0 2px; }
.empty { color: var(--anzhiyu-gray); padding: 14px 4px; }
.err { color: var(--anzhiyu-red); margin: 8px 0; }
.ban-form { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.ban-form input { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 8px 12px; font: inherit; outline: none; width: 180px; }
.ban-form input:focus { border-color: var(--anzhiyu-theme); }
.ban-form .reason { flex: 1; min-width: 160px; }
.ban-form .unit { color: var(--anzhiyu-gray); font-size: .84rem; }
.btn { background: var(--anzhiyu-theme); color: #fff; border: none; border-radius: var(--anzhiyu-radius); padding: 8px 16px; font: inherit; cursor: pointer; }
.btn:hover { background: var(--anzhiyu-hover); }
.btn.small { padding: 4px 10px; font-size: .8rem; }
.btn.danger { background: var(--anzhiyu-red); }
.filter-bar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.filter-bar label { font-size: .88rem; color: var(--anzhiyu-secondary); }
.filter-bar input { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 8px 12px; font: inherit; outline: none; flex: 1; min-width: 200px; }
.filter-bar input:focus { border-color: var(--anzhiyu-theme); }
.filter-bar .hint { color: var(--anzhiyu-gray); font-size: .78rem; }
.range-select { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 8px 10px; font: inherit; outline: none; color: inherit; cursor: pointer; }
.range-select:focus { border-color: var(--anzhiyu-theme); }
.rtc { font-size: .76rem; max-width: 260px; overflow: hidden; text-overflow: ellipsis; }
.fp-table th, .fp-table td { white-space: nowrap; }
.comp { font-size: .74rem; color: var(--anzhiyu-secondary); }
.warn { color: var(--anzhiyu-red); margin-left: 4px; cursor: help; }
.ok-chip { background: rgba(46, 160, 67, .15); color: #2ea043; border-radius: 50px; padding: 1px 10px; font-size: .72rem; margin-left: 8px; }
.bad-chip { background: rgba(248, 81, 73, .15); color: var(--anzhiyu-red); border-radius: 50px; padding: 1px 10px; font-size: .72rem; margin-left: 8px; }
.flag-list { display: flex; flex-direction: column; gap: 6px; }
.flag-item { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; font-size: .84rem; }
.flag-name { color: var(--anzhiyu-red); font-weight: 600; min-width: 180px; }
.flag-desc { color: var(--anzhiyu-secondary); flex: 1; }
.flag-code { background: var(--anzhiyu-background); border-radius: 6px; padding: 1px 8px; font-size: .74rem; color: var(--anzhiyu-gray); }
.ua-set { margin: 8px 0; font-size: .82rem; color: var(--anzhiyu-secondary); }
.ua-set summary { cursor: pointer; }
.ua-set ul { margin: 6px 0 0; padding-left: 18px; word-break: break-all; white-space: normal; }
.ip-link { color: var(--anzhiyu-theme); cursor: pointer; text-decoration: none; }
.ip-link:hover { text-decoration: underline; }
.stat-row { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 1rem; }
.stat { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 10px 20px; display: flex; flex-direction: column; align-items: center; gap: 2px; min-width: 96px; }
.stat b { font-size: 1.15rem; font-weight: 700; }
.stat span { color: var(--anzhiyu-gray); font-size: .76rem; }
.stat.warn b { color: #d48806; }
.stat.hot b { color: var(--anzhiyu-red); }
.dev { display: inline-flex; align-items: center; gap: 5px; font-size: .84rem; }
.dev-icon { font-size: .95rem; }
.dev.app { color: var(--anzhiyu-theme); font-weight: 600; }
.dev.web { color: var(--anzhiyu-secondary); }
.row-risk { box-shadow: inset 3px 0 0 var(--anzhiyu-red); }
.detail-panel { border: 1px solid var(--anzhiyu-theme); scroll-margin-top: 12px; animation: panelIn .45s ease; }
@keyframes panelIn {
  0% { box-shadow: 0 0 0 4px var(--anzhiyu-theme-op); }
  100% { box-shadow: var(--card-box-shadow); }
}
.detail-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: .6rem; }
.detail-head h3 { margin: 0; }
.detail-head .btn { margin-left: 8px; }
.detail-panel h4 { margin: 1rem 0 .5rem; font-size: .9rem; color: var(--anzhiyu-secondary); }
.kv { display: flex; gap: 24px; flex-wrap: wrap; }
.kv > div { display: flex; flex-direction: column; gap: 2px; }
.kv span { color: var(--anzhiyu-gray); font-size: .76rem; }
.kv b { font-weight: 600; font-size: .88rem; }
.kv .ua { max-width: 420px; white-space: normal; word-break: break-all; font-weight: 400; }
/* ---- 下钻增强 ---- */
.stat-row.mini { gap: 8px; margin: .6rem 0 1rem; }
.stat-row.mini .stat { padding: 6px 14px; min-width: 84px; }
.stat-row.mini .stat b { font-size: 1rem; }
.hint-line { font-size: .82rem; color: #d48806; margin: 4px 0 10px; }
.fp-row { cursor: pointer; }
.fp-row:hover { background: var(--anzhiyu-theme-op); }
.caret { color: var(--anzhiyu-gray); margin-right: 4px; font-size: .72rem; }
.fp-expand td { background: var(--anzhiyu-background); white-space: normal; padding: 12px 16px; }
.exp-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
@media (max-width: 900px) { .exp-grid { grid-template-columns: 1fr; } }
.exp-col h5 { margin: 10px 0 6px; font-size: .82rem; color: var(--anzhiyu-secondary); font-weight: 600; }
.exp-col h5:first-child { margin-top: 0; }
.kv.tight { gap: 16px; }
.kv.tight > div { padding: 1px 0; }
.kv.tight b { font-size: .8rem; font-weight: 500; }
.break { white-space: normal; word-break: break-all; }
.chip-row { display: flex; flex-wrap: wrap; gap: 8px; margin: 4px 0 8px; align-items: center; }
.chip.bad { background: rgba(248, 81, 73, .16); color: var(--anzhiyu-red); }
.kind-row { margin-bottom: 8px; }
.kind-chip { font-size: .78rem; color: var(--anzhiyu-secondary); }
.flag-item.small { font-size: .8rem; }
.flag-item.small .flag-name { min-width: 150px; }
</style>
