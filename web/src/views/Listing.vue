<script setup>
// 分类/标签/归档详情页（参考站 article-sort 过滤列表结构）
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'

const route = useRoute()
const name = computed(() => decodeURIComponent(route.params.name || ''))
const month = computed(() => (route.params.month ? `${route.params.year}-${route.params.month}` : ''))
const view = ref({ github: [], news: [], digests: [] })
const loading = ref(true)
const page = ref(1)
const pageSize = 10

// 依据路由来源组装条目
const entries = computed(() => {
  const list = []
  const isCategory = route.path.startsWith('/categories')
  const isTag = route.path.startsWith('/tags')
  const isArchive = route.path.startsWith('/archives')
  if (isArchive && month.value) {
    for (const d of view.value.digests) {
      if (d.date.startsWith(month.value)) {
        list.push({ kind: d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报', title: `${d.date} 双热点报告`, url: `/digest/${d.date}`, date: d.date, tags: [] })
      }
    }
  } else if (isCategory) {
    if (name.value === '日报' || name.value === '周报' || name.value === '月报') {
      for (const d of view.value.digests) {
        const k = d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报'
        if (k === name.value) list.push({ kind: k, title: `${d.date} 双热点报告`, url: `/digest/${d.date}`, date: d.date, tags: [] })
      }
    } else {
      for (const p of view.value.github) {
        if (p.language === name.value) {
          list.push({ kind: '项目', title: p.fullName, url: p.url, date: '', tags: p.topics || [], hotness: p.hotness, gained: p.starsGained })
        }
      }
    }
  } else if (isTag) {
    for (const n of view.value.news) {
      if ((n.tags || []).includes(name.value) || (n.badges || []).includes(name.value)) {
        list.push({ kind: '事件', title: n.titleZh, url: `/story/${n.storyId}`, date: (n.firstSeenAt || '').slice(0, 10), tags: n.tags || [], hotness: n.hotness })
      }
    }
  }
  list.sort((a, b) => (b.hotness || 0) - (a.hotness || 0) || (a.date < b.date ? 1 : -1))
  return list
})

const paged = computed(() => entries.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const pageCount = computed(() => Math.max(1, Math.ceil(entries.value.length / pageSize)))

function coverOf(title, idx) {
  let h = (idx * 47) % 360
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='600' height='336'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='hsl(${h},42%,62%)'/><stop offset='1' stop-color='hsl(${(h + 40) % 360},46%,44%)'/></linearGradient></defs><rect width='600' height='336' fill='url(#g)'/><circle cx='500' cy='70' r='110' fill='rgba(255,255,255,0.12)'/></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

onMounted(async () => {
  const [hot, dg] = await Promise.all([api.get('/api/v1/hot'), api.get('/api/v1/digests?pageSize=500')])
  view.value.github = hot.github || []
  view.value.news = hot.news || []
  view.value.digests = dg.items || []
  loading.value = false
})
</script>

<template>
  <header class="post-bg post-bg--blue" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">{{ route.path.startsWith('/archives') ? '归档' : route.path.startsWith('/categories') ? '分类' : '标签' }}</router-link></div></div>
      <h1 class="post-title">{{ route.path.startsWith('/archives') ? month : name }}</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">共 {{ entries.length }} 篇</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!entries.length" class="empty">该{{ route.path.startsWith('/archives') ? '月' : route.path.startsWith('/categories') ? '分类' : '标签' }}暂无内容</div>
        <div v-for="(e, i) in paged" :key="e.url + i" class="recent-post-item fade-up">
          <div class="post_cover left">
            <router-link v-if="e.url.startsWith('/')" :to="e.url">
              <img class="post_bg" :src="coverOf(e.title, i)" alt="cover" style="pointer-events: none">
            </router-link>
            <a v-else :href="e.url" target="_blank" rel="noopener">
              <img class="post_bg" :src="coverOf(e.title, i)" alt="cover" style="pointer-events: none">
            </a>
          </div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips">
                <div class="article-categories-original">{{ e.kind }}</div>
              </div>
              <router-link v-if="e.url.startsWith('/')" class="article-title" :to="e.url">{{ e.title }}</router-link>
              <a v-else class="article-title" :href="e.url" target="_blank" rel="noopener">{{ e.title }}</a>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date" v-if="e.hotness !== undefined">
                <span class="hot">热度 {{ e.hotness.toFixed(1) }}</span>
                <span v-if="e.gained !== undefined" class="article-meta-separator">·</span>
                <span v-if="e.gained !== undefined" class="gain">24h +{{ e.gained }} ★</span>
              </span>
              <span class="article-meta tags">
                <span v-for="t in e.tags.slice(0, 3)" :key="t" class="article-meta__tags">{{ t }}</span>
              </span>
            </div>
          </div>
        </div>
        <div id="pagination">
          <div class="pagination">
            <span class="page-item" :class="{ disabled: page === 1 }" @click="page > 1 && page--">‹</span>
            <span v-for="pn in pageCount" :key="pn" class="page-item" :class="{ active: pn === page }" @click="pn !== page && (page = pn)">{{ pn }}</span>
            <span class="page-item" :class="{ disabled: page >= pageCount }" @click="page < pageCount && page++">›</span>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 17rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
@media (max-width: 768px) { .post-bg { height: 13rem; } }
</style>
