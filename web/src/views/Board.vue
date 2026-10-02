<script setup>
// 榜单页：搜索 / 筛选 / 排序 / 分页（数据全量拉取后客户端处理）
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const props = defineProps({ board: { type: String, default: 'github' } })
const view = ref({ github: [], news: [], generatedAt: '' })
const loading = ref(true)
const q = ref('')
const lang = ref('')
const tag = ref('')
const sort = ref('hot')
const page = ref(1)
const pageSize = ref(20)

// 语言/标签候选
const langs = computed(() => {
  const s = new Set()
  for (const p of view.value.github) if (p.language) s.add(p.language)
  return [...s].sort()
})
const tags = computed(() => {
  const s = new Set()
  for (const n of view.value.news) for (const t of n.tags || []) s.add(t)
  return [...s].sort()
})

const filtered = computed(() => {
  const kw = q.value.trim().toLowerCase()
  if (props.board === 'github') {
    let list = view.value.github
    if (lang.value) list = list.filter((p) => p.language === lang.value)
    if (kw) list = list.filter((p) => (p.fullName + ' ' + (p.descriptionZh || '') + ' ' + (p.description || '')).toLowerCase().includes(kw))
    return [...list].sort(sortGithub)
  }
  let list = view.value.news
  if (tag.value) list = list.filter((n) => (n.tags || []).includes(tag.value))
  if (kw) list = list.filter((n) => (n.titleZh + ' ' + n.summaryZh).toLowerCase().includes(kw))
  return [...list].sort(sortNews)
})

function sortGithub(a, b) {
  switch (sort.value) {
    case 'gained': return b.starsGained - a.starsGained
    case 'stars': return b.stars - a.stars
    default: return b.hotness - a.hotness
  }
}
function sortNews(a, b) {
  switch (sort.value) {
    case 'score': return b.score - a.score
    case 'sources': return b.sourceCount - a.sourceCount
    default: return b.hotness - a.hotness
  }
}

const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const pageCount = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize.value)))

function resetPage() { page.value = 1 }

function coverOf(title) {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  const a = `hsl(${h}, 42%, 62%)`
  const b = `hsl(${(h + 40) % 360}, 48%, 44%)`
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='600' height='336'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs><rect width='600' height='336' fill='url(#g)'/><circle cx='500' cy='70' r='110' fill='rgba(255,255,255,0.12)'/></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

onMounted(async () => {
  view.value = await api.get('/api/v1/hot')
  loading.value = false
})
</script>

<template>
  <!-- 榜单页：post-bg 页头（AnZhiYu 文章页同构） -->
  <header class="post-bg" :class="'post-bg--' + (board === 'github' ? 'emerald' : 'azure')" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <router-link class="post-meta-original" to="/">{{ board === 'github' ? 'GitHub 榜' : 'AI 榜' }}</router-link>
        </div>
      </div>
      <h1 class="post-title">{{ board === 'github' ? 'GitHub 项目热点' : 'AI 资讯热点' }}</h1>
      <div id="post-meta">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-calendar-days post-meta-icon"></i>
            <span class="post-meta-label">生成于</span>
            <time>{{ (view.generatedAt || '').slice(0, 10) }}</time>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-wordcount">
            <span class="post-meta-label">共</span>
            <span class="word-count">{{ filtered.length }}</span>
            <span class="post-meta-label">条</span>
          </span>
        </div>
      </div>
    </div>
  </header>

  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <!-- 工具栏：搜索 + 筛选 + 排序 + 每页条数 -->
        <div class="toolbar">
          <input v-model="q" class="search-input" :placeholder="board === 'github' ? '搜索项目名 / 描述…' : '搜索标题 / 摘要…'" @input="resetPage">
          <select v-model="sort" class="select" @change="resetPage">
            <template v-if="board === 'github'">
              <option value="hot">按热度</option>
              <option value="gained">按 24h 增长</option>
              <option value="stars">按总 star</option>
            </template>
            <template v-else>
              <option value="hot">按热度</option>
              <option value="score">按评分</option>
              <option value="sources">按来源数</option>
            </template>
          </select>
          <select v-model="pageSize" class="select narrow" @change="resetPage">
            <option :value="10">10 条/页</option>
            <option :value="20">20 条/页</option>
            <option :value="50">50 条/页</option>
          </select>
        </div>
        <!-- 筛选 chips -->
        <div class="chips-row">
          <span class="chip" :class="{ on: lang === '' && tag === '' }" @click="lang = ''; tag = ''; resetPage()">全部</span>
          <template v-if="board === 'github'">
            <span v-for="l in langs" :key="l" class="chip" :class="{ on: lang === l }" @click="lang = lang === l ? '' : l; resetPage()">{{ l }}</span>
          </template>
          <template v-else>
            <span v-for="t in tags" :key="t" class="chip" :class="{ on: tag === t }" @click="tag = tag === t ? '' : t; resetPage()">{{ t }}</span>
          </template>
        </div>

        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!filtered.length" class="empty">没有匹配的内容</div>
        <div v-for="p in paged" :key="p.fullName || p.storyId" class="recent-post-item fade-up">
          <div class="post_cover left">
            <router-link v-if="board === 'news' && p.storyId" :to="`/story/${p.storyId}`">
              <img class="post_bg" :src="coverOf(p.titleZh)" alt="cover" style="pointer-events: none">
            </router-link>
            <a v-else :href="p.url" target="_blank" rel="noopener" :title="p.fullName">
              <img class="post_bg" :src="coverOf(p.fullName || p.titleZh)" alt="cover" style="pointer-events: none">
            </a>
          </div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips">
                <div class="article-categories-original">{{ board === 'github' ? '开源项目' : 'AI 资讯' }}</div>
                <span v-for="b in p.badges" :key="b" class="badge" :class="{ new: b === '新', rise: b === '上升', gh: b.startsWith('trending') || b === 'GitHub关联' }">{{ b }}</span>
              </div>
              <router-link v-if="board === 'news' && p.storyId" class="article-title" :to="`/story/${p.storyId}`">{{ p.titleZh }}</router-link>
              <a v-else class="article-title" :href="p.url" target="_blank" rel="noopener" :title="p.fullName || p.titleZh">
                {{ board === 'github' ? p.fullName : p.titleZh }}
              </a>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date" v-if="board === 'github'">
                <span class="article-meta-label">24h</span>
                <time class="gain">+{{ p.starsGained }} ★</time>
                <span class="article-meta-separator">·</span>
                <span class="hot">热度 {{ p.hotness.toFixed(1) }}</span>
                <span class="article-meta-separator">·</span>
                <span class="desc">总计 {{ p.stars.toLocaleString() }} ★</span>
              </span>
              <span class="post-meta-date" v-else>
                <span class="hot">热度 {{ p.hotness.toFixed(1) }}</span>
                <span class="article-meta-separator">·</span>
                <span class="desc">评分 {{ p.score.toFixed(1) }}</span>
                <span class="article-meta-separator">·</span>
                <span class="desc">{{ p.sourceCount }} 个来源</span>
              </span>
              <span class="article-meta tags">
                <span v-for="t in (p.topics || p.tags || []).slice(0, 3)" :key="t" class="article-meta__tags">{{ t }}</span>
              </span>
            </div>
            <div class="recent-post-desc" v-if="p.descriptionZh || p.summaryZh || p.overview">{{ p.descriptionZh || p.summaryZh || p.overview }}</div>
            <div class="recent-post-desc secondary" v-if="p.descriptionZh && p.description">{{ p.description }}</div>
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
#post { max-width: 100%; margin: 0; }
.post-bg { height: 24rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 2.1rem; font-weight: 700; margin: 10px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { display: flex; gap: 10px; align-items: center; justify-content: center; opacity: .9; font-size: .88rem; }
.toolbar { display: flex; gap: 10px; margin-bottom: 12px; flex-wrap: wrap; }
.search-input { flex: 1; min-width: 200px; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius-full); padding: 8px 18px; font: inherit; font-size: .9rem; color: var(--anzhiyu-fontcolor); outline: none; }
.search-input:focus { border-color: var(--anzhiyu-theme); }
.select { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 8px 12px; font: inherit; font-size: .88rem; color: var(--anzhiyu-fontcolor); }
.select.narrow { width: 110px; }
.chips-row { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 14px; }
.chip { display: inline-block; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius-full); padding: 3px 14px; font-size: .82rem; cursor: pointer; transition: all .2s; }
.chip:hover { border-color: var(--anzhiyu-theme); color: var(--anzhiyu-hover); }
.chip.on { background: var(--anzhiyu-theme); border-color: var(--anzhiyu-theme); color: #fff; }
.recent-post-desc { color: var(--anzhiyu-secondary); font-size: .9rem; margin-top: 6px; }
.recent-post-desc.secondary { color: var(--anzhiyu-gray); font-size: .82rem; }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
@media (max-width: 768px) { .post-bg { height: 19rem; } .post-title { font-size: 1.5rem; } }
</style>
