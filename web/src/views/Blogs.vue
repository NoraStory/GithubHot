<script setup>
// 博客列表：作者 CSDN 文章（站内阅读 + 原文外链）。
// 数据源 /api/v1/blogs：id/title/url/summary/publishedAt/hasContent。
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const view = ref({ items: [], generatedAt: '' })
const loading = ref(true)
const q = ref('')

const filtered = computed(() => {
  const kw = q.value.trim().toLowerCase()
  if (!kw) return view.value.items
  return view.value.items.filter((n) => (n.title + ' ' + (n.summary || '')).toLowerCase().includes(kw))
})

function fmtDate(s) { return (s || '').slice(0, 10) }
function fmtMonth(s) { return (s || '').slice(5, 7) }
function fmtDay(s) { return (s || '').slice(8, 10) }

onMounted(async () => {
  try { view.value = await api.get('/api/v1/blogs') } catch (e) { /* 空态兜底 */ }
  loading.value = false
})
</script>

<template>
  <header class="post-bg post-bg--warm" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <router-link class="post-meta-original" to="/blogs">博客</router-link>
          <span class="article-meta tags">
            <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>原创</span></a>
            <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>站内阅读</span></a>
          </span>
        </div>
      </div>
      <h1 class="post-title">作者的技术博客</h1>
      <div id="post-meta">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-calendar-days post-meta-icon"></i>
            <span class="post-meta-label">共</span>
            <span class="word-count">{{ view.items.length }}</span>
            <span class="post-meta-label">篇 · Markdown 站内原生阅读</span>
          </span>
        </div>
      </div>
    </div>
  </header>

  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div class="toolbar">
          <input v-model="q" class="search-input" placeholder="搜索文章标题 / 摘要…">
        </div>

        <div v-if="loading" class="loading">加载中</div>
        <div v-else-if="!filtered.length" class="empty">暂无文章，下一轮采集后这里会出现博客更新</div>

        <div class="feed-list">
          <div v-for="p in filtered" :key="p.id" class="feed-row fade-up">
            <div class="date-tile">
              <span class="d">{{ fmtDay(p.publishedAt) }}</span>
              <span class="m">{{ Number(fmtMonth(p.publishedAt)) || '?' }} 月</span>
            </div>
            <div class="row-body">
              <div class="row-top">
                <router-link class="row-title" :to="`/blog/${p.id}`" :title="p.title">{{ p.title }}</router-link>
                <span v-if="p.hasContent" class="badge read">站内可读</span>
              </div>
              <div v-if="p.summary" class="row-summary">{{ p.summary }}</div>
              <div class="row-meta">
                <span class="desc">{{ fmtDate(p.publishedAt) }}</span>
                <span class="row-tag">CSDN</span>
                <a class="ext-link" :href="p.url" target="_blank" rel="noopener" title="在 CSDN 打开原文">
                  <i class="anzhiyufont anzhiyu-icon-link"></i> 原文
                </a>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
#post { max-width: 100%; margin: 0; }
.post-bg { height: 20rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; margin: 10px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.article-meta__tags { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 10px; font-size: .74rem; margin-left: 6px; }
#post-meta .meta-firstline { display: flex; gap: 12px; align-items: center; justify-content: center; flex-wrap: wrap; opacity: .92; font-size: .88rem; }
.toolbar { display: flex; gap: 10px; margin-bottom: 12px; }
.search-input { flex: 1; min-width: 200px; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius-full); padding: 8px 18px; font: inherit; font-size: .9rem; color: var(--anzhiyu-fontcolor); outline: none; }
.search-input:focus { border-color: var(--anzhiyu-theme); }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }

/* ===== 行式列表（与国内热榜同构） ===== */
.feed-list { display: flex; flex-direction: column; gap: 10px; }
.feed-row { display: flex; gap: 14px; align-items: flex-start; background: var(--anzhiyu-maskbg); border: 1px solid var(--anzhiyu-card-border); border-radius: 12px; padding: 14px 16px; transition: border-color .2s, transform .2s; }
.feed-row:hover { border-color: var(--anzhiyu-theme); transform: translateY(-1px); }
.date-tile { display: flex; flex-direction: column; align-items: center; justify-content: center; min-width: 52px; height: 52px; border-radius: 10px; background: var(--anzhiyu-theme-op); color: var(--anzhiyu-theme); flex-shrink: 0; }
.date-tile .d { font-size: 1.25rem; font-weight: 800; line-height: 1.1; }
.date-tile .m { font-size: .68rem; opacity: .8; }
.row-body { flex: 1; min-width: 0; }
.row-top { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.row-title { font-weight: 700; font-size: 1.02rem; line-height: 1.5; color: var(--anzhiyu-fontcolor); }
.row-title:hover { color: var(--anzhiyu-hover); }
.badge { display: inline-block; border-radius: 6px; padding: 0 7px; font-size: .72rem; }
.badge.read { background: var(--anzhiyu-theme-op); color: var(--anzhiyu-theme); }
.row-summary { margin-top: 4px; font-size: .84rem; color: var(--anzhiyu-gray); line-height: 1.7; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.row-meta { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-top: 6px; font-size: .8rem; color: var(--anzhiyu-gray); }
.row-tag { background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 7px; font-size: .72rem; }
.ext-link { color: var(--anzhiyu-blue); font-size: .78rem; text-decoration: none; }
.ext-link:hover { color: var(--anzhiyu-hover); }
@media (max-width: 768px) { .post-bg { height: 16rem; } .post-title { font-size: 1.4rem; } }
</style>
