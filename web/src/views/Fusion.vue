<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const view = ref({ fusion: [], generatedAt: '' })
const loading = ref(true)
onMounted(async () => {
  view.value = await api.get('/api/v1/hot/fusion')
  loading.value = false
})
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">融合观察</router-link></div></div>
      <h1 class="post-title">资讯 × 项目互相印证</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">AI 资讯事件与 GitHub 项目配对成功时，双方热度获得 ×1.25 加成</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中 </div>
        <div v-if="!loading && !view.fusion.length" class="empty">本轮未发现资讯与项目的直接对应</div>
        <div v-for="(f, i) in view.fusion" :key="i" class="recent-post-item fade-up">
          <div class="post_cover left">
            <a :href="f.project.url" target="_blank" rel="noopener">
              <img class="post_bg" :src="'data:image/svg+xml;utf8,' + encodeURIComponent(`<svg xmlns='http://www.w3.org/2000/svg' width='600' height='336'><rect width='600' height='336' fill='%23232832'/><text x='50%25' y='50%25' fill='%23eabcbd' font-size='34' text-anchor='middle' font-family='monospace'>${'{ AI × GH }'}</text></svg>`)" alt="cover">
            </a>
          </div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips">
                <div class="article-categories-original">融合配对</div>
                <span class="chip">热度 ×1.25</span>
              </div>
              <div class="article-title-line">
                <router-link v-if="f.news.storyId" class="article-title" :to="`/story/${f.news.storyId}`">{{ f.news.titleZh }}</router-link>
                <span v-else class="article-title">{{ f.news.titleZh }}</span>
                <span class="x-mark">×</span>
                <a class="article-title proj" :href="f.project.url" target="_blank" rel="noopener">{{ f.project.fullName }}</a>
              </div>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date">
                <span class="hot">GitHub 热度 {{ f.project.hotness.toFixed(1) }}</span>
                <span class="article-meta-separator">·</span>
                <span class="gain">24h +{{ f.project.starsGained }} ★</span>
              </span>
            </div>
            <div class="recent-post-desc">{{ f.news.summaryZh }}</div>
            <div class="recent-post-desc secondary">{{ f.project.descriptionZh || f.project.description }}</div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 20rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.38), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.9rem; font-weight: 700; margin: 10px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.article-title-line { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; }
.article-title { font-weight: 700; color: var(--anzhiyu-blue); }
.article-title:hover { color: var(--anzhiyu-hover); }
.article-title.proj { color: var(--anzhiyu-fontcolor); }
.x-mark { color: var(--anzhiyu-hover); font-weight: 700; font-size: 1.1rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.chip { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 10px; font-size: .74rem; }
.recent-post-desc { color: var(--anzhiyu-secondary); font-size: .9rem; margin-top: 6px; }
.recent-post-desc.secondary { color: var(--anzhiyu-gray); font-size: .82rem; }
@media (max-width: 768px) { .post-bg { height: 17rem; } .post-title { font-size: 1.4rem; } }
</style>
