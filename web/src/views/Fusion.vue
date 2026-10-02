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
  <div class="layout page-enter">
    <main id="article-container">
      <div class="card article">
        <h2 class="first-title">🔗 融合观察：资讯 × 项目互相印证</h2>
        <div class="meta-line">一条 AI 资讯与一个 GitHub 项目互相印证时，两个事件的热度都会获得加成——这是"双重热点"的交汇点。</div>
        <div v-if="loading" class="loading">加载中 </div>
        <div v-if="!loading && !view.fusion.length" class="empty">本轮未发现资讯与项目的直接对应</div>
        <div v-for="(f, i) in view.fusion" :key="i" class="fusion fade-up">
          <div class="news-side">
            <span class="chip">AI 资讯</span>
            <router-link v-if="f.news.storyId" class="news-title" :to="`/story/${f.news.storyId}`">{{ f.news.titleZh }}</router-link>
            <span v-else class="news-title">{{ f.news.titleZh }}</span>
            <div class="desc">{{ f.news.summaryZh }}</div>
          </div>
          <div class="x">×</div>
          <div class="proj-side">
            <span class="chip blue">GitHub 项目</span>
            <a class="proj-name" :href="f.project.url" target="_blank" rel="noopener">{{ f.project.fullName }}</a>
            <div class="desc">{{ f.project.descriptionZh || f.project.description }}</div>
            <div class="desc">24h +{{ f.project.starsGained }}★ · 热度 {{ f.project.hotness.toFixed(1) }}</div>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<style scoped>
.meta-line { color: var(--anzhiyu-gray); font-size: .8rem; margin: 8px 0 14px; }
.fusion { display: flex; align-items: center; gap: 14px; padding: 14px 6px; border-bottom: 1px dashed var(--anzhiyu-card-border); }
.fusion:last-child { border-bottom: none; }
.fusion .news-side, .fusion .proj-side { flex: 1; min-width: 0; }
.fusion .x { color: var(--anzhiyu-hover); font-weight: 700; font-size: 1.3rem; }
.news-title, .proj-name { font-weight: 700; color: var(--anzhiyu-blue); }
.news-title:hover, .proj-name:hover { color: var(--anzhiyu-hover); }
.desc { color: var(--anzhiyu-gray); font-size: .82rem; margin-top: 3px; }
</style>
