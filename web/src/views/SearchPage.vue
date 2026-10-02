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
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">搜索</router-link></div></div>
      <h1 class="post-title">站内搜索</h1>
      <form id="post-meta" class="searchform" @submit.prevent="doSearch">
        <input v-model="q" placeholder="搜索标题 / 摘要，如：Claude、agent、向量数据库">
        <button>搜索</button>
      </form>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div class="meta-line" v-if="results !== null">耗时 {{ took }} · {{ results.length }} 条结果</div>
        <div v-if="loading" class="loading">搜索中 </div>
        <div v-else-if="results !== null && !results.length" class="empty">没有匹配结果</div>
        <div v-for="r in results || []" :key="r.url" class="story-line fade-up">
          <a class="member-link" :href="r.url" target="_blank" rel="noopener">
            <span class="chip">{{ r.kind === 'story' ? '事件' : '资讯' }}</span> {{ r.titleZh }}
          </a>
          <span class="desc">{{ r.summaryZh }}</span>
          <span class="desc">{{ r.sourceNames }} · 热度 {{ r.hotness.toFixed(1) }}</span>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; padding: 0 1.5rem; }
.post-title { font-size: 1.7rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.searchform { display: flex; gap: 10px; margin-top: 16px; width: min(520px, 90vw); }
.searchform input { flex: 1; border: none; outline: none; background: rgba(255,255,255,.96); border-radius: 50px; padding: 10px 20px; font: inherit; color: #363636; }
.searchform button { border: none; background: var(--anzhiyu-theme); color: #fff; border-radius: 50px; padding: 10px 24px; cursor: pointer; font: inherit; }
.searchform button:hover { background: var(--anzhiyu-hover); }
.meta-line { color: var(--anzhiyu-gray); font-size: .8rem; margin: 8px 0; }
.story-line { padding: 11px 4px; border-bottom: 1px dashed var(--anzhiyu-card-border); display: flex; flex-direction: column; gap: 3px; }
.story-line:last-child { border-bottom: none; }
.member-link { font-weight: 600; color: var(--anzhiyu-fontcolor); }
.member-link:hover { color: var(--anzhiyu-hover); }
.chip { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 8px; font-size: .78rem; margin-right: 6px; }
.desc { color: var(--anzhiyu-gray); font-size: .82rem; }
@media (max-width: 768px) { .post-bg { height: 15rem; } }
</style>
