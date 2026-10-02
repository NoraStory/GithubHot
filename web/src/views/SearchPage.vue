<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'

const route = useRoute()
const q = ref(route.query.q || '')
const results = ref(null)
const took = ref('')
const loading = ref(false)

async function doSearch() {
  if (!q.value.trim()) return
  loading.value = true
  const started = Date.now()
  const d = await api.get(`/api/v1/search?q=${encodeURIComponent(q.value)}`)
  results.value = d.results || []
  took.value = `${Date.now() - started}ms`
  loading.value = false
}
onMounted(() => { if (q.value) doSearch() })
</script>

<template>
  <div class="layout page-enter">
    <main id="article-container">
      <div class="card article">
        <h2 class="first-title">🔍 搜索</h2>
        <div class="meta-line">检索已精选写作的资讯与事件{{ took ? ` · 耗时 ${took}` : '' }}</div>
        <form class="searchform" @submit.prevent="doSearch">
          <input v-model="q" placeholder="搜索标题 / 摘要，如：Claude、agent、向量数据库">
          <button>搜索</button>
        </form>
        <div v-if="loading" class="loading">搜索中 </div>
        <div v-else-if="results !== null && !results.length" class="empty">没有匹配结果</div>
        <div v-for="r in results || []" :key="r.url" class="story fade-up">
          <div class="story-head">
            <span class="chip">{{ r.kind === 'story' ? '事件' : 'AI 资讯' }}</span>
            <a class="title" :href="r.url" target="_blank" rel="noopener">{{ r.titleZh }}</a>
          </div>
          <div class="summary">{{ r.summaryZh }}</div>
          <div class="story-meta">{{ r.sourceNames }} · 热度 {{ r.hotness.toFixed(1) }}</div>
        </div>
      </div>
    </main>
  </div>
</template>

<style scoped>
.meta-line { color: var(--anzhiyu-gray); font-size: .8rem; margin: 8px 0 12px; }
.searchform { display: flex; gap: 10px; margin-bottom: 14px; }
.searchform input { flex: 1; background: var(--anzhiyu-card-bg); color: var(--anzhiyu-fontcolor); border: 1px solid var(--anzhiyu-card-border); border-radius: 50px; padding: 10px 20px; font: inherit; outline: none; }
.searchform input:focus { border-color: var(--anzhiyu-theme); }
.searchform button { background: var(--anzhiyu-theme); color: #fff; border: none; border-radius: 50px; padding: 10px 26px; cursor: pointer; font: inherit; }
.searchform button:hover { background: var(--anzhiyu-hover); }
.story { padding: 13px 4px; border-bottom: 1px dashed var(--anzhiyu-card-border); }
.story:last-child { border-bottom: none; }
.story-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.story-head .title { font-weight: 700; }
.story-head .title:hover { color: var(--anzhiyu-hover); }
.summary { color: var(--anzhiyu-secondary); margin-top: 5px; font-size: .95rem; }
.story-meta { color: var(--anzhiyu-gray); font-size: .8rem; margin-top: 5px; }
</style>
