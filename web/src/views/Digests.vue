<script setup>
import { ref, onMounted } from 'vue'
import Pagination from '../components/Pagination.vue'
import { api } from '../lib/api'

const items = ref([])
const total = ref(0)
const page = ref(1)
const kind = ref('all')
const pageSize = 10
const loading = ref(true)

const kinds = [
  { v: 'all', label: '全部' },
  { v: 'daily', label: '日报' },
  { v: 'weekly', label: '周报' },
  { v: 'monthly', label: '月报' }
]

async function load() {
  loading.value = true
  const k = kind.value === 'all' ? '' : `&kind=${kind.value}`
  const d = await api.get(`/api/v1/digests?page=${page.value}&pageSize=${pageSize}${k}`)
  items.value = d.items || []
  total.value = d.total || 0
  loading.value = false
}
function setKind(k) { kind.value = k; page.value = 1; load() }
onMounted(load)
</script>

<template>
  <div class="layout page-enter">
    <main id="article-container">
      <div class="card article">
        <h2 class="first-title">📰 期刊</h2>
        <div class="meta-line">每日 08:00 日报 · 每周一 周报 · 每月 1 日 月报</div>
        <div class="kinds">
          <button v-for="k in kinds" :key="k.v" class="tab" :class="{ active: kind === k.v }" @click="setKind(k.v)">{{ k.label }}</button>
        </div>
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!items.length" class="empty">暂无期刊</div>
        <div v-for="d in items" :key="d.date" class="digest-row fade-up">
          <span class="chip">{{ d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报' }}</span>
          <router-link class="date-link" :to="`/digest/${d.date}`">{{ d.date }}</router-link>
          <span class="date-stats" v-if="d.stats">{{ d.stats.githubItems }} 项目 · {{ d.stats.newsItems }} 资讯</span>
        </div>
        <Pagination :total="total" :page="page" :page-size="pageSize" @change="page = $event; load()" />
      </div>
    </main>
  </div>
</template>

<style scoped>
.meta-line { color: var(--anzhiyu-gray); font-size: .8rem; margin: 8px 0 12px; }
.kinds { display: flex; gap: 8px; margin-bottom: 12px; }
.tab { border: none; background: var(--anzhiyu-background); padding: 7px 18px; border-radius: var(--anzhiyu-radius-full); cursor: pointer; font: inherit; font-size: .88rem; }
.tab.active { background: var(--anzhiyu-theme); color: #fff; }
.digest-row { display: flex; align-items: center; gap: 12px; padding: 11px 4px; border-bottom: 1px dashed var(--anzhiyu-card-border); }
.digest-row:last-child { border-bottom: none; }
.date-link { font-weight: 700; color: var(--anzhiyu-blue); }
.date-link:hover { color: var(--anzhiyu-hover); }
.date-stats { color: var(--anzhiyu-gray); font-size: .8rem; margin-left: auto; }
</style>
