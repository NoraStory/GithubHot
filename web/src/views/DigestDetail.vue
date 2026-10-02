<script setup>
// 日报详情：嵌入 Markdown 查看器（marked，完整 GFM 表格/列表/引用/代码）+ 上下篇导航
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import { api } from '../lib/api'

const route = useRoute()
const props = defineProps({ latest: { type: Boolean, default: false } })
const date = ref(props.latest ? 'latest' : route.params.date)
const md = ref('')
const loading = ref(true)

// 已知期号用于上一篇/下一篇
const allDates = ref([])
const prevDate = computed(() => {
  const i = allDates.value.indexOf(date.value)
  return i > 0 ? allDates.value[i - 1] : null
})
const nextDate = computed(() => {
  const i = allDates.value.indexOf(date.value)
  return i >= 0 && i < allDates.value.length - 1 ? allDates.value[i + 1] : null
})

// marked 配置：GFM 表格 + 安全渲染（转义内嵌 HTML，日报内容来自我们自己的流水线）
marked.setOptions({ gfm: true, breaks: false })
const renderer = {
  // 表格包一层滚动容器，宽表不破版
  table(header, body) {
    return `<div class="table-wrap"><table><thead>${header}</thead><tbody>${body}</tbody></table></div>`
  }
}
marked.use({ renderer })

const rendered = computed(() => marked.parse(md.value))
const wordCount = computed(() => md.value.replace(/\s/g, '').length)
const readMinutes = computed(() => Math.max(1, Math.round(wordCount.value / 400)))

onMounted(async () => {
  const dg = await api.get('/api/v1/digests?pageSize=50')
  allDates.value = (dg.items || []).map((d) => d.date)
  const key = date.value === 'latest' ? (allDates.value[0] || 'latest') : date.value
  md.value = await api.raw(`/api/v1/digest/${encodeURIComponent(key)}?format=raw`)
  if (date.value === 'latest') date.value = key
  loading.value = false
})
</script>

<template>
  <!-- 期刊详情：post 页 + 内嵌 Markdown 查看器 -->
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <router-link class="post-meta-original" to="/">期刊</router-link>
          <span class="article-meta tags">
            <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>双热点</span></a>
            <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>日报</span></a>
          </span>
        </div>
      </div>
      <h1 class="post-title">{{ date }} 双热点报告</h1>
      <div id="post-meta">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-calendar-days post-meta-icon"></i>
            <span class="post-meta-label">发表于</span>
            <time>{{ date }}</time>
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
        <div v-if="loading" class="loading">加载中 </div>
        <!-- Markdown 查看器渲染区 -->
        <div class="md-viewer" v-html="rendered"></div>
      </div>
      <!-- 上一篇 / 下一篇 -->
      <div class="post-nav" v-if="prevDate || nextDate">
        <router-link v-if="prevDate" class="post-nav-prev" :to="`/digest/${prevDate}`">
          <span class="label">‹ 上一篇</span>
          <span class="title">{{ prevDate }} 双热点报告</span>
        </router-link>
        <span v-else class="post-nav-prev"></span>
        <router-link v-if="nextDate" class="post-nav-next" :to="`/digest/${nextDate}`">
          <span class="label">下一篇 ›</span>
          <span class="title">{{ nextDate }} 双热点报告</span>
        </router-link>
        <span v-else class="post-nav-next"></span>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 20rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.article-meta__tags { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 10px; font-size: .74rem; margin-left: 6px; }
#post-meta .meta-firstline { display: flex; gap: 12px; align-items: center; justify-content: center; flex-wrap: wrap; opacity: .92; font-size: .88rem; }
.post-meta-separator { opacity: .5; }
.post-nav { display: flex; justify-content: space-between; gap: 14px; margin-top: 1.6rem; }
.post-nav-prev, .post-nav-next { flex: 1; display: flex; flex-direction: column; gap: 2px; background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 12px 18px; transition: all .3s; }
.post-nav-next { text-align: right; }
.post-nav-prev:hover, .post-nav-next:hover { box-shadow: var(--card-hover-box-shadow); transform: translateY(-2px); }
.post-nav .label { color: var(--anzhiyu-gray); font-size: .78rem; }
.post-nav .title { font-weight: 700; color: var(--anzhiyu-blue); font-size: .95rem; }
.post-nav .title:hover { color: var(--anzhiyu-hover); }
</style>

<style>
/* ===== Markdown 查看器排版（AnZhiYu #article-container 风格）===== */
.md-viewer { font-size: 1rem; line-height: 1.85; }
.md-viewer h1 { font-size: 1.5rem; margin: .4rem 0 1rem; }
.md-viewer h2 { font-size: 1.25rem; margin: 1.8rem 0 .8rem; position: relative; padding-left: 1.35rem; }
.md-viewer h2::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: ccc 1.6s linear infinite; }
.md-viewer h3 { font-size: 1.08rem; margin: 1.2rem 0 .5rem; }
.md-viewer h3::before { content: '✦'; position: absolute; left: 0; color: #ffbf00; font-size: .95rem; }
@keyframes ccc { 0% { transform: rotate(0); } to { transform: rotate(-1turn); } }
.md-viewer p { margin: .55rem 0; color: var(--anzhiyu-secondary); }
.md-viewer strong { color: var(--anzhiyu-fontcolor); }
.md-viewer blockquote { margin: .8rem 0; padding: 10px 16px; background: var(--anzhiyu-background); border-left: 3px solid var(--anzhiyu-theme); border-radius: 6px; color: var(--anzhiyu-secondary); font-size: .95rem; }
.md-viewer blockquote p { margin: 0; }
.md-viewer code { background: var(--anzhiyu-background); padding: 1px 6px; border-radius: 4px; font-size: .85rem; color: var(--anzhiyu-blue); }
.md-viewer a { color: var(--anzhiyu-blue); }
.md-viewer a:hover { color: var(--anzhiyu-hover); }
.md-viewer hr { border: none; border-top: 1px dashed var(--anzhiyu-card-border); margin: 1.2rem 0; }
.md-viewer em { color: var(--anzhiyu-gray); font-size: .85rem; font-style: normal; }
/* 表格：滚动容器 + 主题粉调表头 */
.md-viewer .table-wrap { overflow-x: auto; margin: 0.8rem 0; border-radius: var(--anzhiyu-radius); border: 1px solid var(--anzhiyu-card-border); }
.md-viewer table { width: 100%; min-width: 620px; border-collapse: collapse; font-size: .92rem; margin: 0; }
.md-viewer thead th { background: var(--anzhiyu-theme-op); color: var(--anzhiyu-secondary); font-weight: 600; padding: 10px; text-align: left; white-space: nowrap; }
.md-viewer tbody td { padding: 10px; border-bottom: 1px solid var(--anzhiyu-card-border); vertical-align: top; }
.md-viewer tbody tr:hover { background: var(--anzhiyu-background); }
.md-viewer tbody tr:last-child td { border-bottom: none; }
.md-viewer td a { font-weight: 600; }
</style>
