<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../lib/api'

// 资源监测：进程 RSS / CPU / Go 运行时 / mlserve sidecar 状态
// CPU% 由前端差分两次轮询的 cpu_seconds_total 计算得出
const stats = ref(null)
const error = ref('')
const hist = ref([])          // { rss, cpu } 最近 90 个样本（3 分钟）
let timer = null
let prev = null               // { cpu, t } 上次采样

const MAX_PTS = 90
const numCPU = computed(() => stats.value?.num_cpu || 1)

const cpuPct = computed(() => {
  const last = hist.value[hist.value.length - 1]
  return last ? last.cpu : null
})

const sidecar = computed(() => stats.value?.sidecar || { enabled: false })

async function poll() {
  try {
    const d = await api.get('/api/v1/admin/system/stats')
    error.value = ''
    const now = performance.now()
    let cpu = null
    if (prev && d.cpu_seconds_total != null && d.cpu_seconds_total >= prev.cpu) {
      const wall = (now - prev.t) / 1000
      cpu = Math.min(100, Math.max(0, ((d.cpu_seconds_total - prev.cpu) / wall / (d.num_cpu || 1)) * 100))
    }
    prev = { cpu: d.cpu_seconds_total ?? 0, t: now }
    hist.value.push({ rss: d.rss_mb, cpu })
    if (hist.value.length > MAX_PTS) hist.value.shift()
    stats.value = d
  } catch (e) {
    error.value = e.message
  }
}

onMounted(() => { poll(); timer = setInterval(poll, 2000) })
onUnmounted(() => clearInterval(timer))

// sparkline：把数值序列映射到 SVG polyline 点
function sparkPoints(values, w = 240, h = 44) {
  const pts = values.filter(v => v != null)
  if (pts.length < 2) return ''
  const max = Math.max(...pts), min = Math.min(...pts)
  const span = Math.max(max - min, 0.001)
  const seq = values.filter(v => v != null) // 占位：与 hist 对齐由调用方保证
  return seq.map((v, i) =>
    `${(i / (seq.length - 1) * w).toFixed(1)},${(h - 4 - ((v - min) / span) * (h - 8)).toFixed(1)}`).join(' ')
}
const rssPts = computed(() => sparkPoints(hist.value.map(x => x.rss)))
const cpuPts = computed(() => sparkPoints(hist.value.map(x => x.cpu)))
const fmtUp = s => s == null ? '--' : s < 60 ? `${s}s` : s < 3600 ? `${(s / 60).toFixed(1)}min` : `${(s / 3600).toFixed(1)}h`
const fmtVer = v => (v || '').split('/').pop()
</script>

<template>
  <div>
    <div class="page-head">
      <h2>资源监测</h2>
      <span class="stat" v-if="stats">PID {{ stats.pid }} · Go {{ stats.go_version }} · {{ stats.num_cpu }} 核
        · 运行 {{ fmtUp(stats.uptime_s) }}</span>
    </div>
    <div v-if="error" class="empty">加载失败：{{ error }}</div>

    <div class="cards" v-if="stats">
      <div class="card">
        <div class="num">{{ stats.rss_mb ?? '—' }}<small> MB</small></div>
        <div class="label">进程内存 RSS</div>
        <svg viewBox="0 0 240 44" preserveAspectRatio="none">
          <polyline :points="rssPts" fill="none" stroke="var(--anzhiyu-theme)" stroke-width="1.5" />
        </svg>
      </div>
      <div class="card">
        <div class="num">{{ cpuPct == null ? '—' : cpuPct.toFixed(1) }}<small> %</small></div>
        <div class="label">CPU（占全部核心）</div>
        <svg viewBox="0 0 240 44" preserveAspectRatio="none">
          <polyline :points="cpuPts" fill="none" stroke="#3fb950" stroke-width="1.5" />
        </svg>
      </div>
      <div class="card"><div class="num">{{ stats.goroutines }}</div><div class="label">Goroutines</div></div>
      <div class="card"><div class="num">{{ stats.heap_alloc_mb }}<small> MB</small></div><div class="label">堆分配 HeapAlloc</div></div>
      <div class="card"><div class="num">{{ stats.sys_mb }}<small> MB</small></div><div class="label">进程向 OS 申请 Sys</div></div>
      <div class="card"><div class="num">{{ stats.num_gc }}</div><div class="label">GC 次数 · 占 CPU {{ ((stats.gc_cpu_percent || 0)).toFixed(2) }}%</div></div>
    </div>

    <h3 class="sec">GNN sidecar（mlserve，规格书 P6-3b）</h3>
    <div class="panel">
      <template v-if="!sidecar.enabled">
        <span class="dot gray"></span>未配置 <code>GNN_SIDECAR_URL</code>——纯离线模式，主服务功能不受影响
      </template>
      <template v-else-if="sidecar.status === 'ok'">
        <div class="sc-row"><span><span class="dot ok"></span>在线 · {{ sidecar.model_version }}</span>
          <span>RSS <b>{{ sidecar.rss_mb }}</b> MB</span>
          <span>累计打分 <b>{{ sidecar.scored_24h }}</b></span>
          <span>P99 <b>{{ sidecar.p99_ms }}</b>ms</span>
          <span>运行 {{ fmtUp(sidecar.uptime_s) }}</span></div>
      </template>
      <template v-else>
        <span class="dot bad"></span>sidecar {{ sidecar.status }}
        <span class="stat" v-if="sidecar.error">（{{ sidecar.error }}）</span>
        —— 打分回落 Louvain 社区均值，主服务不受影响
      </template>
    </div>
  </div>
</template>

<style scoped>
.page-head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 16px; flex-wrap: wrap; gap: 10px; }
.page-head h2 { margin: 0; }
.stat { font-size: 12px; opacity: 0.65; }
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; margin-bottom: 8px; }
.card { background: var(--anzhiyu-card-bg); border-radius: 12px; padding: 14px 16px; box-shadow: var(--anzhiyu-shadow-light); }
.card .num { font-size: 24px; font-weight: 700; font-variant-numeric: tabular-nums; }
.card .num small { font-size: 12px; opacity: 0.6; font-weight: 400; }
.card .label { font-size: 12px; opacity: 0.65; margin-bottom: 6px; }
.card svg { display: block; width: 100%; height: 44px; }
.sec { margin: 20px 0 10px; font-size: 15px; }
.panel { background: var(--anzhiyu-card-bg); border-radius: 12px; padding: 14px 18px; font-size: 13.5px; box-shadow: var(--anzhiyu-shadow-light); }
.panel code { background: var(--anzhiyu-background); padding: 1px 6px; border-radius: 6px; font-size: 12px; }
.sc-row { display: flex; gap: 18px; flex-wrap: wrap; align-items: center; }
.sc-row b { font-variant-numeric: tabular-nums; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; }
.dot.ok { background: #3fb950; }
.dot.bad { background: #e64545; }
.dot.gray { background: #8a8f9e; }
.empty { text-align: center; opacity: 0.55; padding: 20px; }
</style>
