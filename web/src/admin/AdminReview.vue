<script setup>
// §10.2 Review 队列页：异常超标指纹的批量处理（标注 / 忽略）。
import { ref, onMounted } from 'vue'
import { api, passkeyLogin } from '../lib/api'
import { deviceFp } from '../lib/fingerprint'

const items = ref([])
const loading = ref(true)
const error = ref('')
const selected = ref(new Set())
const busy = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const d = await api.get('/api/v1/admin/review/queue?status=pending&limit=50')
    items.value = d.items || []
    selected.value = new Set()
  } catch (e) { error.value = e.message }
  loading.value = false
}

function toggle(id) {
  if (selected.value.has(id)) selected.value.delete(id)
  else selected.value.add(id)
}

async function resolve(action) {
  if (selected.value.size === 0) return
  busy.value = true
  try {
    await api.post('/api/v1/admin/review/resolve', { ids: [...selected.value], action })
    await load()
  } catch (e) { error.value = e.message }
  busy.value = false
}

async function labelFp(fp, label) {
  busy.value = true
  try {
    await api.post('/api/v1/admin/fp/label', { fp, label, notes: 'review queue' })
    await load()
  } catch (e) { error.value = e.message }
  busy.value = false
}

const reasonLabels = {
  anomaly_p999: 'iForest 异常',
  cluster_risk: '簇风险',
  midas_5sigma: 'MIDAS 5σ',
  ml_shadow_high: 'ML shadow 高',
}
const tagClass = (tag) => {
  if (tag.includes('anomaly')) return 'tag-anomaly'
  if (tag.includes('midas')) return 'tag-midas'
  if (tag.includes('cluster')) return 'tag-cluster'
  return 'tag-ml'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <h2>📋 Review 队列</h2>
    <div v-if="error" class="err">{{ error }}</div>
    <div v-if="loading" class="hint">加载中…</div>
    <template v-else>
      <div class="actions" v-if="selected.size > 0">
        <button class="primary" :disabled="busy" @click="resolve('done')">✓ 标记已处理</button>
        <button class="ghost" :disabled="busy" @click="resolve('ignored')">忽略</button>
      </div>
      <div v-else class="hint">勾选待处理项后可批量操作。</div>
      <table v-if="items.length">
        <thead><tr><th></th><th>指纹</th><th>原因</th><th>分数</th><th>IP</th><th>标注</th></tr></thead>
        <tbody>
          <tr v-for="item in items" :key="item.id">
            <td><input type="checkbox" :checked="selected.has(item.id)" @change="toggle(item.id)"></td>
            <td class="mono">{{ item.fp.slice(0, 16) }}…</td>
            <td><span v-for="tag in item.reason_tags" :key="tag" class="tag" :class="tagClass(tag)">{{ reasonLabels[tag] || tag }}</span></td>
            <td><span v-for="(v, k) in item.scores" :key="k" class="score">{{ k }}: {{ v.toFixed(3) }}</span></td>
            <td>{{ item.ip_sample }}</td>
            <td>
              <button class="mini" @click="labelFp(item.fp, 'bot')">bot</button>
              <button class="mini" @click="labelFp(item.fp, 'human')">human</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="hint">队列为空。</div>
    </template>
  </div>
</template>

<style scoped>
.page h2 { margin: 0 0 8px; }
.hint { color: var(--grey9, #888); font-size: 13px; }
.err { color: #c0392b; margin: 10px 0; }
.actions { margin: 10px 0; display: flex; gap: 8px; }
.mono { font-family: monospace; font-size: 12px; }
.primary { background: var(--main, #425066); color: #fff; border: none; padding: 6px 14px; border-radius: 6px; cursor: pointer; }
.ghost { background: transparent; border: 1px solid var(--main, #425066); color: var(--main, #425066); border-radius: 6px; cursor: pointer; }
.mini { background: transparent; border: 1px solid var(--main, #425066); color: var(--main, #425066); border-radius: 4px; cursor: pointer; font-size: 12px; margin: 0 2px; }
.mini:hover { background: var(--main, #425066); color: #fff; }
table { width: 100%; margin-top: 14px; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid var(--light-border, #eee); }
.tag { display: inline-block; padding: 1px 8px; border-radius: 4px; font-size: 12px; margin: 0 2px; }
.tag-anomaly { background: #e74c3c22; color: #e74c3c; }
.tag-midas { background: #e67e2222; color: #e67e22; }
.tag-cluster { background: #3498db22; color: #3498db; }
.tag-ml { background: #9b59b622; color: #9b59b6; }
.score { font-size: 12px; margin: 0 4px; }
button:disabled { opacity: 0.5; cursor: default; }
</style>
