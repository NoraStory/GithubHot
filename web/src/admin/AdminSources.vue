<script setup>
// 信源管理 + 脚本推送（消费 /api/v1/admin/sources 与 /api/v1/admin/push）
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const sources = ref([])
const msg = ref('')
const loading = ref(true)
const form = ref({ id: '', name: '', kind: 'rss', tier: 'T2', url: '', intervalMinutes: 120 })
const pushForm = ref({ sourceId: 'script-push', url: '', title: '', summary: '' })
const kinds = ['rss', 'json_api', 'web_list', 'hacker_news', 'github_search', 'github_trending', 'hot_board', 'script']

const scriptSources = computed(() => sources.value.filter((s) => s.kind === 'script'))

// 国内热榜 Top10 综述开关
const summaryCfg = ref({ enabled: true, hasSummary: false, date: '' })
async function loadSummaryCfg() {
  try {
    summaryCfg.value = await api.get('/api/v1/admin/domestic-summary')
  } catch { /* 忽略，展示默认 */ }
}
async function toggleSummary() {
  const next = !summaryCfg.value.enabled
  try {
    await api.post('/api/v1/admin/domestic-summary', { enabled: next })
    summaryCfg.value.enabled = next
    msg.value = `国内热榜综述已${next ? '开启' : '关闭'}`
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}

async function load() {
  loading.value = true
  const d = await api.get('/api/v1/sources')
  sources.value = d.items || []
  if (scriptSources.value.length && !scriptSources.value.some((s) => s.id === pushForm.value.sourceId)) {
    pushForm.value.sourceId = scriptSources.value[0].id
  }
  loading.value = false
}
onMounted(() => { load(); loadSummaryCfg() })

async function add() {
  msg.value = '保存中…'
  try {
    await api.post('/api/v1/admin/sources', form.value)
    msg.value = `已保存：${form.value.id}`
    form.value = { id: '', name: '', kind: 'rss', tier: 'T2', url: '', intervalMinutes: 120 }
    await load()
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
async function del(id) {
  if (!confirm(`删除信源 ${id}？`)) return
  try {
    await api.del(`/api/v1/admin/sources/${encodeURIComponent(id)}`)
    msg.value = `已删除 ${id}`
    await load()
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
async function test(id) {
  const s = sources.value.find((x) => x.id === id) || {}
  msg.value = `试抓 ${id} …`
  try {
    const d = await api.post('/api/v1/admin/sources/test', {
      id, name: s.name, kind: s.kind, tier: s.tier, url: (s.config && s.config.url) || '', config: s.config || {}
    })
    msg.value = d.ok
      ? `试抓成功，共 ${d.count} 条，预览: ${(d.preview || []).map((p) => p.title).join(' / ')}`
      : `试抓失败: ${d.error}`
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
async function push() {
  const f = pushForm.value
  if (!f.url || !f.title) { msg.value = 'URL 与标题必填'; return }
  msg.value = '推送中…'
  try {
    const d = await api.post('/api/v1/admin/push', f)
    msg.value = `已推送（id=${d.id.slice(0, 12)}…）`
    pushForm.value.url = ''
    pushForm.value.title = ''
    pushForm.value.summary = ''
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
function displayInterval(s) {
  const cur = s.currentIntervalMinutes || 0
  return cur > 0 ? `${cur} 分钟（自适应）` : '—'
}
</script>

<template>
  <div class="page-enter">
    <h2>🛠 信源管理</h2>
    <div v-if="msg" class="msg card">{{ msg }}</div>

    <div class="card">
      <h3>新增 / 更新信源</h3>
      <div class="form-grid">
        <input v-model="form.id" placeholder="id（如 rss-xxx）">
        <input v-model="form.name" placeholder="名称">
        <select v-model="form.kind">
          <option v-for="k in kinds" :key="k" :value="k">{{ k }}</option>
        </select>
        <select v-model="form.tier"><option value="T1">T1 官方</option><option value="T2">T2 媒体</option></select>
        <input v-model="form.url" placeholder="URL（feed / 接口 / 页面）" class="wide">
        <button class="primary" @click="add">保存</button>
      </div>
      <div class="desc">config 可配 fulltext=1 开启全文抓取；抓取间隔按产出自适应（连空退避，上限 24h）。</div>
    </div>

    <div class="card">
      <h3>国内热榜 Top10 综述</h3>
      <div class="summary-toggle-row">
        <span class="desc">
          每天流水线为<a href="/domestic" target="_blank">国内热榜</a>Top10 生成一段 AI 综述（单次调用，预算很小）。
          今日（{{ summaryCfg.date || '—' }}）状态：<b>{{ summaryCfg.hasSummary ? '已生成' : '未生成' }}</b>
        </span>
        <button class="primary" @click="toggleSummary">{{ summaryCfg.enabled ? '关闭综述' : '开启综述' }}</button>
      </div>
    </div>

    <div class="card">
      <div v-if="loading" class="loading">加载中 </div>
      <table v-if="!loading && sources.length">
        <thead><tr><th>ID</th><th>名称</th><th>种类</th><th>当前间隔</th><th>连空</th><th>最近抓取</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="s in sources" :key="s.id">
            <td><code>{{ s.id }}</code></td>
            <td>{{ s.name }}</td>
            <td><span class="chip">{{ s.kind }}</span> {{ s.tier }}</td>
            <td class="num">{{ displayInterval(s) }}</td>
            <td class="num">{{ s.emptyStreak || 0 }}</td>
            <td class="desc">{{ s.lastFetchedAt || '从未' }}</td>
            <td><span class="badge" :class="s.enabled ? 'okbadge' : 'offbadge'">{{ s.enabled ? '启用' : '停用' }}</span></td>
            <td>
              <button class="op" @click="test(s.id)">试抓</button>
              <button class="op danger" @click="del(s.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="empty">无信源</div>
    </div>

    <h2>📨 脚本推送</h2>
    <div class="card">
      <div class="desc" style="margin-bottom:10px">
        外部脚本也可以走 CLI：<code>githubhot push --source script-push --url ... --title ...</code>；
        或 HTTP：<code>POST /api/v1/admin/push</code>。URL 判重，kind=script 的信源才会出现在下方。
      </div>
      <div v-if="!scriptSources.length" class="empty">暂无 script 类信源——先在上方新增一个 kind=script 的信源</div>
      <div v-else class="form-grid">
        <select v-model="pushForm.sourceId">
          <option v-for="s in scriptSources" :key="s.id" :value="s.id">{{ s.name }}（{{ s.id }}）</option>
        </select>
        <input v-model="pushForm.url" placeholder="https:// 资料链接" class="wide">
        <input v-model="pushForm.title" placeholder="标题" class="wide">
        <input v-model="pushForm.summary" placeholder="摘要（可选）" class="wide">
        <button class="primary" @click="push">推送到流水线</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
h2, h3 { font-size: 1.15rem; margin: 1.3rem 0 .7rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; margin-bottom: 1rem; overflow-x: auto; }
.form-grid { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
input, select { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 8px 12px; font: inherit; font-size: .9rem; color: var(--anzhiyu-fontcolor); }
.wide { flex: 2; min-width: 220px; }
button { border: 1px solid var(--anzhiyu-card-border); background: var(--anzhiyu-background); border-radius: var(--anzhiyu-radius); padding: 8px 16px; font: inherit; font-size: .88rem; cursor: pointer; }
button.primary { background: var(--anzhiyu-theme); color: #fff; border: none; }
button:hover { border-color: var(--anzhiyu-hover); color: var(--anzhiyu-hover); }
button.primary:hover { color: #fff; }
.op { padding: 4px 14px; font-size: .82rem; }
.op.danger:hover { border-color: var(--anzhiyu-red); color: var(--anzhiyu-red); }
.msg { padding: 9px 14px; font-size: .88rem; color: var(--anzhiyu-secondary); }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
.chip { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 8px; font-size: .78rem; }
.badge { padding: 1px 10px; border-radius: 50px; font-size: .74rem; }
.okbadge { background: #238636; color: #fff; }
.offbadge { background: var(--anzhiyu-theme-op); color: #a8766f; }
.summary-toggle-row { display: flex; align-items: center; gap: 14px; justify-content: space-between; flex-wrap: wrap; }
.summary-toggle-row .desc { flex: 1; min-width: 260px; line-height: 1.7; }
.summary-toggle-row a { color: var(--anzhiyu-hover); }
</style>
