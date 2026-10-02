<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const runs = ref([])
const loading = ref(true)
onMounted(async () => {
  const d = await api.get('/api/v1/admin/runs')
  runs.value = d.items || []
  loading.value = false
})
</script>

<template>
  <div class="page-enter">
    <h2>🖥 运行历史</h2>
    <div class="card">
      <div v-if="loading" class="loading">加载中 </div>
      <table v-if="!loading && runs.length">
        <thead><tr><th>开始时间(UTC)</th><th>状态</th><th>耗时</th><th>采集</th><th>写作</th><th>事件</th></tr></thead>
        <tbody>
          <tr v-for="(r, i) in runs" :key="i">
            <td>{{ r.startedAt.replace('T', ' ').slice(0, 19) }}</td>
            <td><span class="badge" :class="r.status === 'ok' ? 'okbadge' : 'errbadge'">{{ r.status }}</span></td>
            <td class="num">{{ r.durationSeconds.toFixed(0) }}s</td>
            <td class="num">{{ r.collected }}</td>
            <td class="num">{{ r.written }}</td>
            <td class="num">{{ r.stories }}</td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="empty">尚无运行记录</div>
    </div>
  </div>
</template>

<style scoped>
h2 { font-size: 1.2rem; margin: 1.4rem 0 .8rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; overflow-x: auto; }
.badge { padding: 1px 10px; border-radius: 50px; font-size: .74rem; }
.okbadge { background: #238636; color: #fff; }
.errbadge { background: #b62324; color: #fff; }
.num { font-variant-numeric: tabular-nums; }
</style>
