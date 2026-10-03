<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../lib/api'

const latest = ref([])
const loading = ref(false)
const running = ref(false)
const expanded = ref('') // 展开历史的目标名
const history = ref([])
const historyLoading = ref(false)
let pollTimer = null

async function load() {
  loading.value = true
  try {
    const d = await api.get('/api/v1/admin/probes')
    latest.value = d.latest || []
  } finally {
    loading.value = false
  }
}

async function runNow() {
  running.value = true
  try {
    await api.post('/api/v1/admin/probes/run', {})
    // 轮询等待新一轮结果落库
    let tries = 0
    clearInterval(pollTimer)
    pollTimer = setInterval(async () => {
      tries++
      await load()
      if (tries >= 20) clearInterval(pollTimer)
    }, 3000)
    setTimeout(() => { running.value = false; clearInterval(pollTimer) }, 60000)
  } catch (e) {
    running.value = false
  }
}

async function toggleHistory(target) {
  if (expanded.value === target) {
    expanded.value = ''
    return
  }
  expanded.value = target
  history.value = []
  historyLoading.value = true
  try {
    const d = await api.get('/api/v1/admin/probes?target=' + encodeURIComponent(target))
    history.value = d.history || []
  } finally {
    historyLoading.value = false
  }
}

const sources = computed(() => latest.value.filter(r => r.kind === 'source'))
const endpoints = computed(() => latest.value.filter(r => r.kind === 'endpoint'))
const okCount = list => list.filter(r => r.ok).length
const lastRunAt = computed(() =>
  latest.value.length ? latest.value.map(r => r.checkedAt).sort().pop() : '')

function fmtTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  return isNaN(d) ? iso : d.toLocaleString('zh-CN', { hour12: false })
}

function statusClass(ok) {
  return ok ? 'ok' : 'bad'
}

onMounted(load)
onUnmounted(() => clearInterval(pollTimer))
</script>

<template>
  <div class="probes-page">
    <div class="page-head">
      <h2>🩺 健康探针</h2>
      <div class="head-actions">
        <span class="last-run">上次探测：{{ fmtTime(lastRunAt) }}</span>
        <button class="btn" :disabled="running" @click="runNow">
          {{ running ? '探测中…' : '立即探测' }}
        </button>
        <button class="btn ghost" :disabled="loading" @click="load">刷新</button>
      </div>
    </div>

    <div class="cards">
      <div class="card">
        <div class="num">{{ okCount(sources) }}/{{ sources.length }}</div>
        <div class="label">信源在线</div>
      </div>
      <div class="card">
        <div class="num">{{ okCount(endpoints) }}/{{ endpoints.length }}</div>
        <div class="label">端点在线</div>
      </div>
      <div class="card" :class="{ warn: endpoints.some(e => !e.ok) || sources.some(s => !s.ok) }">
        <div class="num">{{ endpoints.some(e => !e.ok) || sources.some(s => !s.ok) ? '⚠' : '✓' }}</div>
        <div class="label">整体状态</div>
      </div>
    </div>

    <h3 class="sec">端点服务</h3>
    <table class="tbl">
      <thead>
        <tr><th>目标</th><th>状态</th><th>耗时</th><th>详情</th><th>探测时间</th></tr>
      </thead>
      <tbody>
        <tr v-for="r in endpoints" :key="r.target" class="row" @click="toggleHistory(r.target)">
          <td>{{ r.target }}</td>
          <td><span class="dot" :class="statusClass(r.ok)"></span>{{ r.ok ? '正常' : '异常' }}</td>
          <td>{{ r.latencyMs }}ms</td>
          <td class="detail">{{ r.detail || '—' }}</td>
          <td>{{ fmtTime(r.checkedAt) }}</td>
        </tr>
        <tr v-if="!endpoints.length"><td colspan="5" class="empty">暂无数据（首轮探测于服务启动 15 秒后自动进行）</td></tr>
      </tbody>
    </table>

    <h3 class="sec">信源（{{ sources.length }} 个启用）</h3>
    <table class="tbl">
      <thead>
        <tr><th>信源</th><th>状态</th><th>耗时</th><th>详情</th><th>探测时间</th></tr>
      </thead>
      <tbody>
        <template v-for="r in sources" :key="r.target">
          <tr class="row" @click="toggleHistory(r.target)">
            <td>{{ r.target }}</td>
            <td><span class="dot" :class="statusClass(r.ok)"></span>{{ r.ok ? '正常' : '异常' }}</td>
            <td>{{ r.latencyMs }}ms</td>
            <td class="detail">{{ r.detail || '—' }}</td>
            <td>{{ fmtTime(r.checkedAt) }}</td>
          </tr>
          <tr v-if="expanded === r.target" class="hist-row">
            <td colspan="5">
              <div v-if="historyLoading" class="empty">加载历史…</div>
              <div v-else class="hist">
                <div v-for="h in history" :key="h.checkedAt + h.latencyMs" class="hist-line">
                  <span class="dot" :class="statusClass(h.ok)"></span>
                  <span class="t">{{ fmtTime(h.checkedAt) }}</span>
                  <span class="ms">{{ h.latencyMs }}ms</span>
                  <span class="d">{{ h.detail }}</span>
                </div>
                <div v-if="!history.length" class="empty">无历史记录</div>
              </div>
            </td>
          </tr>
        </template>
        <tr v-if="!sources.length"><td colspan="5" class="empty">暂无数据</td></tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.page-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; flex-wrap: wrap; gap: 10px; }
.page-head h2 { margin: 0; }
.head-actions { display: flex; align-items: center; gap: 10px; }
.last-run { font-size: 12px; opacity: 0.65; }
.btn { padding: 6px 16px; border-radius: 8px; border: none; background: var(--anzhiyu-theme); color: #fff; cursor: pointer; font-size: 13px; }
.btn:disabled { opacity: 0.5; cursor: default; }
.btn.ghost { background: var(--anzhiyu-theme-op); color: var(--anzhiyu-fontcolor); }
.cards { display: flex; gap: 14px; margin-bottom: 20px; flex-wrap: wrap; }
.card { flex: 1; min-width: 140px; background: var(--anzhiyu-card-bg); border-radius: 12px; padding: 16px 20px; text-align: center; box-shadow: var(--anzhiyu-shadow-light); }
.card.warn { outline: 2px solid #e64545; }
.card .num { font-size: 26px; font-weight: 700; }
.card .label { font-size: 13px; opacity: 0.7; margin-top: 4px; }
.sec { margin: 22px 0 10px; font-size: 15px; }
.tbl { width: 100%; border-collapse: collapse; font-size: 13px; }
.tbl th { text-align: left; padding: 8px 10px; opacity: 0.6; font-weight: 500; border-bottom: 1px solid var(--anzhiyu-theme-op); }
.tbl td { padding: 9px 10px; border-bottom: 1px solid var(--anzhiyu-theme-op); }
.row { cursor: pointer; }
.row:hover { background: var(--anzhiyu-theme-op); }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; }
.dot.ok { background: #3fb950; }
.dot.bad { background: #e64545; }
.detail { max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty { text-align: center; opacity: 0.55; padding: 12px; }
.hist-row td { background: var(--anzhiyu-theme-op); }
.hist-line { display: flex; gap: 14px; padding: 4px 8px; font-size: 12px; align-items: center; }
.hist-line .t { min-width: 150px; }
.hist-line .ms { min-width: 60px; opacity: 0.7; }
.hist-line .d { opacity: 0.75; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
