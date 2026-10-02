<script setup>
import { ref, computed, onMounted } from 'vue'
import Pagination from '../components/Pagination.vue'
import { api } from '../lib/api'

const items = ref([])
const total = ref(0)
const page = ref(1)
const kind = ref('all')
const q = ref('')
const pageSize = 8
const loading = ref(true)

const kinds = [
  { v: 'all', label: '全部' },
  { v: 'daily', label: '日报' },
  { v: 'weekly', label: '周报' },
  { v: 'monthly', label: '月报' }
]

function coverOf(title) {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  const a = `hsl(${h}, 42%, 62%)`
  const b = `hsl(${(h + 40) % 360}, 48%, 44%)`
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='600' height='336'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs><rect width='600' height='336' fill='url(#g)'/><circle cx='500' cy='70' r='110' fill='rgba(255,255,255,0.12)'/></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

async function load() {
  loading.value = true
  const k = kind.value === 'all' ? '' : `&kind=${kind.value}`
  let d = await api.get(`/api/v1/digests?page=${1}&pageSize=${500}${k}`)
  let list = d.items || []
  if (q.value.trim()) {
    const kw = q.value.trim().toLowerCase()
    list = list.filter((x) => x.date.toLowerCase().includes(kw))
  }
  total.value = list.length
  const start = (page.value - 1) * pageSize
  items.value = list.slice(start, start + pageSize)
  loading.value = false
}
function setKind(k) { kind.value = k; page.value = 1; load() }

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
function prevPage() { if (page.value > 1) { page.value--; load() } }
function nextPage() { if (page.value < pageCount.value) { page.value++; load() } }
function goPage(p) { if (p !== page.value) { page.value = p; load() } }
onMounted(load)
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">归档</router-link></div></div>
      <h1 class="post-title">全部期刊</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">日报每天 08:00 · 周报每周一 · 月报每月 1 日</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div class="toolbar">
          <input v-model="q" class="search-input" placeholder="搜索期号…" @input="page = 1">
        </div>
        <div id="categoryBar">
          <div class="category-bar" id="category-bar">
            <div id="catalog-bar">
              <div id="catalog-list">
                <div v-for="k in kinds" :key="k.v" class="catalog-list-item" :id="k.v">
                  <a href="javascript:void(0)" @click="setKind(k.v)">{{ k.label }}</a>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!items.length" class="empty">暂无期刊</div>
        <div v-for="d in items" :key="d.date" class="recent-post-item fade-up">
          <div class="post_cover left">
            <router-link :to="`/digest/${d.date}`">
              <img class="post_bg" :src="coverOf(d.date)" alt="cover" style="pointer-events: none">
            </router-link>
          </div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips">
                <div class="article-categories-original">{{ d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报' }}</div>
              </div>
              <router-link class="article-title" :to="`/digest/${d.date}`">{{ d.date }} 双热点报告</router-link>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date">
                <i class="anzhiyufont anzhiyu-icon-calendar-days" style="font-size: 15px"></i>
                <span class="article-meta-label">发表于</span>
                <time>{{ (d.createdAt || '').slice(0, 10) }}</time>
              </span>
              <span class="article-meta tags" v-if="d.stats">
                <a class="article-meta__tags"><span>🔥 {{ d.stats.githubItems }} 项目</span></a>
                <a class="article-meta__tags"><span>🤖 {{ d.stats.newsItems }} 资讯</span></a>
              </span>
            </div>
          </div>
        </div>
        <div id="pagination">
          <div class="pagination">
            <span class="page-item" :class="{ disabled: page === 1 }" @click="prevPage">‹</span>
            <span v-for="pn in pageCount" :key="pn" class="page-item" :class="{ active: pn === page }" @click="goPage(pn)">{{ pn }}</span>
            <span class="page-item" :class="{ disabled: page >= pageCount }" @click="nextPage">›</span>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 19rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.toolbar { display: flex; margin-bottom: 12px; }
.search-input { flex: 1; max-width: 340px; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius-full); padding: 8px 18px; font: inherit; font-size: .9rem; color: var(--anzhiyu-fontcolor); outline: none; }
.search-input:focus { border-color: var(--anzhiyu-theme); }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
@media (max-width: 768px) { .post-bg { height: 15rem; } }
</style>
