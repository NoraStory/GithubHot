<script setup>
// 分类/标签云（AnZhiYu card-tag-cloud 同构）：GitHub 语言 / 事件标签 / 期刊类型
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'

const route = useRoute()
const mode = route.path.startsWith('/tags') ? 'tags' : 'categories'
const view = ref({ github: [], news: [] })
const loading = ref(true)

const cloud = computed(() => {
  const counts = {}
  if (mode === 'categories') {
    counts['日报'] = (view.value.digestCount || 0)
    for (const p of view.value.github || []) {
      if (p.language) counts[p.language] = (counts[p.language] || 0) + 1
    }
  } else {
    for (const n of view.value.news || []) {
      for (const t of n.tags || []) counts[t] = (counts[t] || 0) + 1
      for (const b of n.badges || []) if (b !== '新' && b !== '上升') counts[b] = (counts[b] || 0) + 1
    }
  }
  const max = Math.max(...Object.values(counts), 1)
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .map(([name, count]) => ({ name, count, size: (0.85 + (count / max) * 0.9).toFixed(2) }))
})

function target(name) {
  if (mode === 'categories') {
    if (name === '日报') return '/digests'
    return '/github'
  }
  return { path: '/search', query: { q: name } }
}

onMounted(async () => {
  const [hot, dg] = await Promise.all([api.get('/api/v1/hot'), api.get('/api/v1/digests?pageSize=50')])
  view.value.github = hot.github || []
  view.value.news = hot.news || []
  view.value.digestCount = (dg.items || []).length
  loading.value = false
})
</script>

<template>
  <header class="post-bg post-bg--pink" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">{{ mode === 'tags' ? '标签' : '分类' }}</router-link></div></div>
      <h1 class="post-title">{{ mode === 'tags' ? '事件标签' : '内容分类' }}</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">点击标签查看对应内容</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!cloud.length" class="empty">暂无数据</div>
        <div v-else class="card-tag-cloud">
          <router-link
            v-for="c in cloud"
            :key="c.name"
            :to="target(c.name)"
            :style="{ fontSize: c.size + 'rem' }"
          >{{ c.name }}<sup>{{ c.count }}</sup></router-link>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.card-tag-cloud { text-align: center; }
.card-tag-cloud a { display: inline-block; margin: 8px 12px; color: var(--anzhiyu-fontcolor); }
.card-tag-cloud a:hover { color: var(--anzhiyu-hover); }
.card-tag-cloud sup { color: var(--anzhiyu-theme); font-size: .7em; }
@media (max-width: 768px) { .post-bg { height: 14rem; } }
</style>
