<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const data = ref(null)
const error = ref('')
const banIp = ref('')
const banHours = ref(24)
const banReason = ref('')
const ipFilter = ref('')

// IP 下钻详情
const detail = ref(null)
const detailLoading = ref(false)
const detailError = ref('')

const match = (ip) => !ipFilter.value || (ip || '').includes(ipFilter.value.trim())

const talkers = computed(() => (data.value?.talkers || []).filter(t => match(t.ip)))
const bansFiltered = computed(() => (data.value?.bans || []).filter(b => match(b.IP)))
const eventsFiltered = computed(() => (data.value?.events || []).filter(e => match(e.IP)))
const fpsFiltered = computed(() => (data.value?.fingerprints || []).filter(f => match(f.IPs.join(' '))))

async function load() {
  error.value = ''
  try {
    data.value = await api.get('/api/v1/admin/ipguard/summary')
  } catch (e) {
    error.value = e.message
  }
}

async function showIP(ip) {
  if (!ip) return
  detailError.value = ''
  detailLoading.value = true
  detail.value = null
  try {
    const d = await api.get('/api/v1/admin/ipguard/ip?ip=' + encodeURIComponent(ip))
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
const levelName = n => ['', '30 分钟', '24 小时', '7 天', '30 天'][Math.min(n, 4)] || (n + ' 级')

onMounted(load)
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
        <button v-if="ipFilter" class="btn small" @click="ipFilter = ''">清除</button>
        <span class="hint">点击任意 IP 可下钻查看完整档案与指纹</span>
      </div>

      <!-- IP 下钻详情 -->
      <div v-if="detail || detailLoading || detailError" class="card detail-panel">
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

          <!-- 当前封禁 -->
          <template v-if="detail.ban">
            <h4>当前封禁</h4>
            <div class="kv">
              <div><span>违规次数</span><b class="num">{{ detail.ban.Strikes }}</b></div>
              <div><span>档位</span><b>{{ levelName(detail.ban.Level) }}</b></div>
              <div><span>原因</span><b>{{ detail.ban.Reason }}</b></div>
              <div><span>解禁时间</span><b>{{ fmt(detail.ban.ExpiresAt) }}</b></div>
            </div>
          </template>

          <!-- 关联指纹 -->
          <h4>关联设备指纹（{{ detail.fingerprints.length }}）</h4>
          <table v-if="detail.fingerprints.length">
            <thead><tr><th>指纹</th><th>关联 IP 数</th><th>最近使用</th><th>上报次数</th></tr></thead>
            <tbody>
              <tr v-for="f in detail.fingerprints" :key="f.Fingerprint">
                <td class="mono" :title="f.Fingerprint + '\n' + f.UA">{{ short(f.Fingerprint) }}</td>
                <td class="num" :class="{ hot: f.IPs.length > 8 }">{{ f.IPs.length }}</td>
                <td>{{ fmt(f.LastSeen) }}</td>
                <td class="num">{{ f.Hits }}</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty">无关联指纹</div>

          <!-- 违规事件 -->
          <h4>违规事件（近 {{ detail.events.length }} 条）</h4>
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
      <h3>违规事件（近 50 条{{ ipFilter ? '，已筛选' : '' }}）</h3>
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
        <table v-else>
          <thead><tr><th>指纹</th><th>关联 IP 数</th><th>最近使用</th><th>上报次数</th></tr></thead>
          <tbody>
            <tr v-for="f in fpsFiltered" :key="f.Fingerprint">
              <td class="mono" :title="f.Fingerprint + '\n' + f.UA">{{ short(f.Fingerprint) }}</td>
              <td class="num" :class="{ hot: f.IPs.length > 8 }">
                <a class="ip-link" @click="ipFilter = f.IPs[0] || ''">{{ f.IPs.length }}</a>
              </td>
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
.ip-link { color: var(--anzhiyu-theme); cursor: pointer; text-decoration: none; }
.ip-link:hover { text-decoration: underline; }
.detail-panel { border: 1px solid var(--anzhiyu-theme); }
.detail-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: .6rem; }
.detail-head h3 { margin: 0; }
.detail-head .btn { margin-left: 8px; }
.detail-panel h4 { margin: 1rem 0 .5rem; font-size: .9rem; color: var(--anzhiyu-secondary); }
.kv { display: flex; gap: 24px; flex-wrap: wrap; }
.kv > div { display: flex; flex-direction: column; gap: 2px; }
.kv span { color: var(--anzhiyu-gray); font-size: .76rem; }
.kv b { font-weight: 600; font-size: .88rem; }
.kv .ua { max-width: 420px; white-space: normal; word-break: break-all; font-weight: 400; }
</style>
