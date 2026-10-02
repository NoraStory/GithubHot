<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const digests = ref([])
const loading = ref(true)

onMounted(async () => {
  const d = await api.get('/api/v1/digests?pageSize=50')
  digests.value = d.items || []
  loading.value = false
})
</script>

<template>
  <div class="page-enter">
    <h2>📰 期刊</h2>
    <div class="card">
      <div v-if="loading" class="loading">加载中 </div>
      <table v-if="!loading && digests.length">
        <thead><tr><th>类型</th><th>期号</th><th>项目 / 资讯 / 融合</th><th>创建时间</th><th>查看</th></tr></thead>
        <tbody>
          <tr v-for="d in digests" :key="d.date">
            <td><span class="chip">{{ d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报' }}</span></td>
            <td>{{ d.date }}</td>
            <td class="num">{{ d.stats.githubItems }} / {{ d.stats.newsItems }} / {{ d.stats.fusion }}</td>
            <td class="desc">{{ (d.createdAt || '').replace('T', ' ').slice(0, 16) }}</td>
            <td><a class="view" :href="`/api/v1/digest/${d.date}?format=raw`" target="_blank" rel="noopener">Markdown ↗</a></td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="empty">暂无期刊</div>
    </div>
  </div>
</template>

<style scoped>
h2 { font-size: 1.2rem; margin: 1.4rem 0 .8rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; overflow-x: auto; }
.chip { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 10px; font-size: .74rem; }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
.num { font-variant-numeric: tabular-nums; }
.view { color: var(--anzhiyu-blue); font-size: .88rem; }
.view:hover { color: var(--anzhiyu-hover); }
</style>
