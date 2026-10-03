<script setup>
// 国内热榜：轻管道多源共振榜（无 LLM，实时计算）。
// 数据源 /api/v1/hot/domestic：rank/title/url/hotness/sources/sourceCount/memberCount/badges。
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const view = ref({ items: [], generatedAt: '', summary: '' })
const loading = ref(true)
const q = ref('')
const sort = ref('hot')
const page = ref(1)
const pageSize = ref(20)

const filtered = computed(() => {
  const kw = q.value.trim().toLowerCase()
  let list = view.value.items
  if (kw) list = list.filter((n) => (n.title + ' ' + (n.sources || []).join(' ')).toLowerCase().includes(kw))
  switch (sort.value) {
    case 'sources': return [...list].sort((a, b) => b.sourceCount - a.sourceCount || b.hotness - a.hotness)
    case 'fresh': return [...list].sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt))
    default: return [...list].sort((a, b) => b.hotness - a.hotness)
  }
})

const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const pageCount = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize.value)))
function resetPage() { page.value = 1 }
function fmtTime(s) { return (s || '').slice(11, 16) }

onMounted(async () => {
  view.value = await api.get('/api/v1/hot/domestic')
  loading.value = false
})
</script>

<template>
  <header class="post-bg post-bg--sunset" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <router-link class="post-meta-original" to="/">国内热榜</router-link>
        </div>
      </div>
      <h1 class="post-title">国内热点新闻</h1>
      <div id="post-meta">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-calendar-days post-meta-icon"></i>
            <span class="post-meta-label">更新于</span>
            <time>{{ (view.generatedAt || '').replace('T', ' ').slice(0, 16) }}</time>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-wordcount">
            <span class="post-meta-label">共</span>
            <span class="word-count">{{ filtered.length }}</span>
            <span class="post-meta-label">条</span>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-label">多源共振 · 无 AI 评分</span>
        </div>
      </div>
    </div>
  </header>

  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div class="toolbar">
          <input v-model="q" class="search-input" placeholder="搜索热点关键词 / 来源…" @input="resetPage">
          <select v-model="sort" class="select" @change="resetPage">
            <option value="hot">按热度</option>
            <option value="sources">按来源数</option>
            <option value="fresh">按最新更新</option>
          </select>
          <select v-model="pageSize" class="select narrow" @change="resetPage">
            <option :value="10">10 条/页</option>
            <option :value="20">20 条/页</option>
            <option :value="50">50 条/页</option>
          </select>
        </div>

        <div v-if="view.summary" class="summary-card">
          <div class="summary-title">📰 今日热点综述</div>
          <p class="summary-text">{{ view.summary }}</p>
        </div>

        <div v-if="loading" class="loading">加载中</div>
        <div v-else-if="!filtered.length" class="empty">暂无热榜数据，等待下一轮换抓取</div>

        <div class="feed-list">
          <div v-for="(p, i) in paged" :key="p.url || p.title" class="feed-row fade-up">
            <div class="row-body">
              <div class="row-top">
                <span class="row-rank" :class="'rank-' + ((page - 1) * pageSize + i + 1)">{{ (page - 1) * pageSize + i + 1 }}</span>
                <a class="row-title" :href="p.url" target="_blank" rel="noopener" :title="p.title">{{ p.title }}</a>
                <span v-for="b in p.badges" :key="b" class="badge" :class="{ hot: b === '沸' }">{{ b }}</span>
                <span v-if="p.sourceCount >= 2" class="badge multi">多源 ×{{ p.sourceCount }}</span>
              </div>
              <div class="row-meta">
                <span class="hot">热度 {{ Number(p.hotness).toFixed(1) }}</span>
                <span class="desc">{{ p.memberCount }} 家报道</span>
                <span v-for="s in p.sources" :key="s" class="row-tag">{{ s }}</span>
                <span class="desc">更新 {{ fmtTime(p.updatedAt) }}</span>
              </div>
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
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
@media (max-width: 768px) { .post-bg { height: 19rem; } .post-title { font-size: 1.5rem; } }

/* ===== 紧凑行式列表 ===== */
.feed-list { display: flex; flex-direction: column; gap: 10px; }
.feed-row { display: flex; gap: 14px; align-items: flex-start; background: var(--anzhiyu-maskbg); border: 1px solid var(--anzhiyu-card-border); border-radius: 12px; padding: 14px 16px; transition: border-color .2s, transform .2s; }
.feed-row:hover { border-color: var(--anzhiyu-theme); transform: translateY(-1px); }
.row-body { flex: 1; min-width: 0; }
.row-top { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.row-rank { font-weight: 800; color: var(--anzhiyu-gray); font-size: .88rem; min-width: 20px; }
.row-rank.rank-1, .row-rank.rank-2, .row-rank.rank-3 { color: var(--anzhiyu-red, #ff7242); font-size: 1rem; }
.row-title { font-weight: 700; font-size: 1.02rem; line-height: 1.5; color: var(--anzhiyu-fontcolor); }
.row-title:hover { color: var(--anzhiyu-hover); }
.row-meta { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-top: 4px; font-size: .8rem; color: var(--anzhiyu-gray); }
.row-meta .hot { color: var(--anzhiyu-hover); font-weight: 700; }
.row-tag { background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 7px; font-size: .72rem; }
.badge { display: inline-block; border-radius: 6px; padding: 0 7px; font-size: .72rem; }
.badge.hot { background: var(--anzhiyu-red, #ff7242); color: #fff; }
.badge.multi { background: var(--anzhiyu-theme-op); color: #a8766f; }

/* ===== 今日综述卡片 ===== */
.summary-card { background: var(--anzhiyu-maskbg); border: 1px solid var(--anzhiyu-card-border); border-left: 3px solid var(--anzhiyu-theme); border-radius: 12px; padding: 14px 18px; margin-bottom: 14px; }
.summary-title { font-weight: 700; font-size: .95rem; margin-bottom: 6px; color: var(--anzhiyu-fontcolor); }
.summary-text { color: var(--anzhiyu-secondary); font-size: .9rem; line-height: 1.9; white-space: pre-line; margin: 0; }
</style>
