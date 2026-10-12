<script setup>
// 博客阅读页：站内原生阅读作者文章（服务端已把原文转成纯 Markdown），
// marked 渲染与期刊页共用 lib/md.js 与全局 .md-viewer 样式，审美同源。
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'
import { renderMarkdown } from '../lib/md'

const route = useRoute()
const det = ref(null)
const loading = ref(true)
const failed = ref(false)

const rendered = computed(() => renderMarkdown(det.value?.content || ''))
const wordCount = computed(() => (det.value?.content || det.value?.summary || '').replace(/\s/g, '').length)
const readMinutes = computed(() => Math.max(1, Math.round(wordCount.value / 400)))
const published = computed(() => (det.value?.publishedAt || '').replace('T', ' ').slice(0, 16))

onMounted(async () => {
  try {
    det.value = await api.get(`/api/v1/blog/${encodeURIComponent(route.params.id)}`)
  } catch (e) {
    failed.value = true
  }
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
          </span>
        </div>
      </div>
      <h1 class="post-title">{{ det?.title || '文章加载中' }}</h1>
      <div id="post-meta" v-if="det">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-calendar-days post-meta-icon"></i>
            <span class="post-meta-label">发表于</span>
            <time>{{ published }}</time>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-wordcount">
            <i class="anzhiyufont anzhiyu-icon-file-word post-meta-icon"></i>
            <span class="post-meta-label">字数总计:</span>
            <span class="word-count">{{ wordCount }}</span>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-wordcount">
            <i class="anzhiyufont anzhiyu-icon-clock post-meta-icon"></i>
            <span class="post-meta-label">阅读时长:</span>
            <span>{{ readMinutes }} 分钟</span>
          </span>
        </div>
      </div>
    </div>
  </header>

  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中</div>
        <div v-else-if="failed || !det" class="empty">文章不存在或已被移除</div>
        <template v-else-if="det.content">
          <!-- Markdown 查看器渲染区（与期刊页同一渲染器与排版） -->
          <div class="md-viewer" v-html="rendered"></div>
        </template>
        <template v-else>
          <!-- 兜底：正文抓取尚未完成，展示摘要并引导原文 -->
          <div class="fallback-card">
            <p class="fallback-title">站内正文还在路上</p>
            <p class="fallback-text">{{ det.summary || '这篇文章的站内存档尚未抓取完成。' }}</p>
            <a class="fallback-btn" :href="det.url" target="_blank" rel="noopener">去原文阅读全文 →</a>
          </div>
        </template>
        <div class="post-foot" v-if="det">
          <a class="origin-link" :href="det.url" target="_blank" rel="noopener">
            <i class="anzhiyufont anzhiyu-icon-link"></i> 阅读原文（CSDN）
          </a>
          <router-link class="back-link" to="/blogs">
            <i class="anzhiyufont anzhiyu-icon-arrow-left"></i> 返回博客列表
          </router-link>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 20rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; padding: 0 1rem; }
.post-title { font-size: 1.7rem; font-weight: 700; margin: 10px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); max-width: 46rem; }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.article-meta__tags { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 10px; font-size: .74rem; margin-left: 6px; }
#post-meta .meta-firstline { display: flex; gap: 12px; align-items: center; justify-content: center; flex-wrap: wrap; opacity: .92; font-size: .88rem; }
.post-meta-separator { opacity: .5; }
.post-foot { display: flex; justify-content: space-between; align-items: center; gap: 14px; margin-top: 1.8rem; flex-wrap: wrap; }
.origin-link, .back-link { color: var(--anzhiyu-blue); font-size: .9rem; text-decoration: none; }
.origin-link:hover, .back-link:hover { color: var(--anzhiyu-hover); }
/* 兜底卡片 */
.fallback-card { background: var(--anzhiyu-maskbg); border: 1px solid var(--anzhiyu-card-border); border-left: 4px solid var(--anzhiyu-theme); border-radius: 12px; padding: 1.4rem 1.6rem; }
.fallback-title { font-weight: 700; font-size: 1.05rem; color: var(--anzhiyu-fontcolor); margin: 0 0 .6rem; }
.fallback-text { color: var(--anzhiyu-secondary); line-height: 1.9; margin: 0 0 1rem; }
.fallback-btn { color: var(--anzhiyu-blue); font-weight: 700; text-decoration: none; }
.fallback-btn:hover { color: var(--anzhiyu-hover); }
@media (max-width: 768px) { .post-bg { height: 16rem; } .post-title { font-size: 1.3rem; } }
</style>
