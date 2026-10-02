<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const props = defineProps({ board: { type: String, default: 'github' } })
const view = ref({ github: [], news: [], generatedAt: '' })
const loading = ref(true)
const page = ref(1)
const pageSize = 6

const list = computed(() => (props.board === 'github' ? view.value.github : view.value.news))
const paged = computed(() => list.value.slice((page.value - 1) * pageSize, page.value * pageSize))

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
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <a class="post-meta-original">{{ board === 'github' ? 'GitHub 榜' : 'AI 榜' }}</a>
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
            <span class="word-count">{{ list.length }}</span>
            <span class="post-meta-label">条</span>
          </span>
        </div>
      </div>
    </div>
  </header>

  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!list.length" class="empty">暂无数据</div>
        <div v-for="p in paged" :key="p.fullName" class="recent-post-item fade-up">
          <div class="post_cover left">
            <a :href="p.url" target="_blank" rel="noopener" :title="p.fullName">
              <img class="post_bg" :src="coverOf(p.fullName)" alt="cover" style="pointer-events: none">
            </a>
          </div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips">
                <div class="article-categories-original">{{ board === 'github' ? '开源项目' : 'AI 资讯' }}</div>
                <span v-for="b in p.badges" :key="b" class="badge" :class="{ new: b === '新', rise: b === '上升', gh: b.startsWith('trending') || b === 'GitHub关联' }">{{ b }}</span>
              </div>
              <a class="article-title" :href="p.url" target="_blank" rel="noopener" :title="board === 'github' ? p.fullName : p.titleZh">
                {{ board === 'github' ? p.fullName : p.titleZh }}
              </a>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date" v-if="board === 'github'">
                <span class="article-meta-label">24h</span>
                <time class="gain">+{{ p.starsGained }} ★</time>
                <span class="article-meta-separator">·</span>
                <span class="hot">热度 {{ p.hotness.toFixed(1) }}</span>
              </span>
              <span class="post-meta-date" v-else>
                <span class="hot">热度 {{ p.hotness.toFixed(1) }}</span>
                <span class="article-meta-separator">·</span>
                <span class="desc">评分 {{ p.score.toFixed(1) }}</span>
              </span>
              <span class="article-meta tags">
                <span v-for="t in p.topics.slice(0, 3)" :key="t" class="article-meta__tags">{{ t }}</span>
              </span>
            </div>
            <div class="recent-post-desc" v-if="p.descriptionZh || p.description">{{ p.descriptionZh || p.description }}</div>
            <div class="recent-post-desc secondary" v-if="p.descriptionZh && p.description">{{ p.description }}</div>
          </div>
        </div>

        <div id="pagination">
          <div class="pagination">
            <span class="page-item" :class="{ disabled: page === 1 }" @click="page > 1 && page--">‹</span>
            <span v-for="pn in Math.max(1, Math.ceil(list.length / pageSize))" :key="pn" class="page-item" :class="{ active: pn === page }" @click="pn !== page && (page = pn)">{{ pn }}</span>
            <span class="page-item" :class="{ disabled: page >= Math.ceil(list.length / pageSize) }" @click="page < Math.ceil(list.length / pageSize) && page++">›</span>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 24rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.4), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 2.1rem; font-weight: 700; margin: 10px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { display: flex; gap: 10px; align-items: center; justify-content: center; opacity: .9; font-size: .88rem; }
.recent-post-desc { color: var(--anzhiyu-secondary); font-size: .9rem; margin-top: 6px; }
.recent-post-desc.secondary { color: var(--anzhiyu-gray); font-size: .82rem; }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
@media (max-width: 768px) { .post-bg { height: 19rem; } .post-title { font-size: 1.5rem; } }
</style>
