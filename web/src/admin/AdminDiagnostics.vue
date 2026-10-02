<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const stages = ['written', 'clustered', 'scored', 'rejected', 'filtered', 'prefiltered', 'dropped', 'new']
const labels = { written: '已写作', clustered: '已聚簇', scored: '评分通过', rejected: '评分淘汰', filtered: '预筛通过', prefiltered: '已预筛', dropped: '预筛淘汰', new: '新采集' }
const stage = ref('written')
const items = ref([])
const loading = ref(false)

async function load() {
  loading.value = true
  const d = await api.get(`/api/v1/admin/diagnostics?stage=${stage.value}`)
  items.value = d.items || []
  loading.value = false
}
onMounted(load)
</script>

<template>
  <div class="page-enter">
    <h2>🩺 内容诊断</h2>
    <div class="card">
      <div class="kinds">
        <button v-for="s in stages" :key="s" class="tab" :class="{ active: stage === s }" @click="stage = s; load()">{{ labels[s] }}</button>
      </div>
      <div v-if="loading" class="loading">加载中 </div>
      <table v-if="!loading && items.length">
        <thead><tr><th>阶段</th><th>标题 / 来源</th><th>评分 A/B</th><th>理由</th></tr></thead>
        <tbody>
          <tr v-for="it in items" :key="it.id">
            <td><span class="chip">{{ labels[it.stage] || it.stage }}</span></td>
            <td>
              <a class="title" :href="it.url" target="_blank" rel="noopener">{{ it.titleZh || it.title }}</a>
              <div class="desc">{{ it.source }}</div>
            </td>
            <td class="num">{{ it.scoreA.toFixed(1) }} / {{ it.scoreB.toFixed(1) }}</td>
            <td class="desc">{{ it.reason }}</td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="empty">该阶段暂无条目</div>
    </div>
  </div>
</template>

<style scoped>
h2 { font-size: 1.2rem; margin: 1.4rem 0 .8rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; overflow-x: auto; }
.kinds { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 12px; }
.tab { border: none; background: var(--anzhiyu-background); padding: 6px 14px; border-radius: var(--anzhiyu-radius-full); cursor: pointer; font: inherit; font-size: .85rem; }
.tab.active { background: var(--anzhiyu-theme); color: #fff; }
.title { font-weight: 600; }
.title:hover { color: var(--anzhiyu-hover); }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
.num { font-variant-numeric: tabular-nums; }
</style>
