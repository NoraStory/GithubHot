<script setup>
// 统计/工具页（AnZhiYu charts + toolbox 合并）：Token 用量图表 + API 工具箱
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const usage = ref(null)
const overview = ref(null)
const tools = [
  { name: '三榜 JSON', desc: 'GET /api/v1/hot', url: '/api/v1/hot' },
  { name: 'GitHub 项目榜', desc: 'GET /api/v1/hot/github', url: '/api/v1/hot/github' },
  { name: 'AI 资讯榜', desc: 'GET /api/v1/hot/news', url: '/api/v1/hot/news' },
  { name: '融合配对', desc: 'GET /api/v1/hot/fusion', url: '/api/v1/hot/fusion' },
  { name: '期刊列表', desc: 'GET /api/v1/digests?page=1', url: '/api/v1/digests?page=1&pageSize=10' },
  { name: '事件详情', desc: 'GET /api/v1/story/{id}', url: '' },
  { name: '站内搜索', desc: 'GET /api/v1/search?q=', url: '' },
  { name: '日报 Markdown', desc: 'GET /api/v1/digest/latest?format=raw', url: '/api/v1/digest/latest?format=raw' },
  { name: 'Agent 报告', desc: 'GET /api/v1/agent/hot.md', url: '/api/v1/agent/hot.md' },
  { name: 'llms.txt', desc: 'GET /llms.txt', url: '/llms.txt' }
]

onMounted(async () => {
  const [hot, dg, srcs] = await Promise.all([
    api.get('/api/v1/hot'), api.get('/api/v1/digests?pageSize=500'), api.get('/api/v1/sources')
  ])
  overview.value = {
    projects: (hot.github || []).length,
    stories: (hot.news || []).length,
    fusion: (hot.fusion || []).length,
    digests: (dg.items || []).length,
    sources: (srcs.items || []).length
  }
  usage.value = await api.get('/api/v1/admin/usage').catch(() => null)
})

function maxDay(days) {
  return Math.max(...(days || []).map((d) => d.promptTokens + d.completionTokens), 1)
}
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">统计与工具</router-link></div></div>
      <h1 class="post-title">站点统计 & API 工具箱</h1>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <h2 class="first-title">站点概览</h2>
        <div class="overview" v-if="overview">
          <div class="ov-card"><div class="v">{{ overview.projects }}</div><div class="k">追踪项目</div></div>
          <div class="ov-card"><div class="v">{{ overview.stories }}</div><div class="k">活跃事件</div></div>
          <div class="ov-card"><div class="v">{{ overview.fusion }}</div><div class="k">融合配对</div></div>
          <div class="ov-card"><div class="v">{{ overview.digests }}</div><div class="k">期刊</div></div>
          <div class="ov-card"><div class="v">{{ overview.sources }}</div><div class="k">信源</div></div>
        </div>

        <h2>Token 用量（近 7 日）</h2>
        <div v-if="usage && usage.days && usage.days.length" class="chart">
          <div v-for="d in usage.days" :key="d.day" class="chart-row">
            <span class="chart-day">{{ d.day }}</span>
            <div class="chart-bar">
              <div class="fill" :style="{ width: ((d.promptTokens + d.completionTokens) / maxDay(usage.days) * 100) + '%' }"></div>
            </div>
            <span class="num chart-v">{{ (d.promptTokens + d.completionTokens).toLocaleString() }}</span>
          </div>
        </div>
        <div v-else class="empty">暂无用量数据</div>

        <h2>API 工具箱</h2>
        <div class="toolbox">
          <a v-for="t in tools.filter(t => t.url)" :key="t.name" class="tool-item" :href="t.url" target="_blank" rel="noopener">
            <div class="tool-name">{{ t.name }}</div>
            <div class="tool-desc">{{ t.desc }}</div>
          </a>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.overview { display: flex; gap: 12px; flex-wrap: wrap; }
.ov-card { flex: 1; min-width: 120px; background: var(--anzhiyu-background); border-radius: var(--anzhiyu-radius); padding: 14px 10px; text-align: center; }
.ov-card .v { font-weight: 700; font-size: 1.35rem; color: var(--anzhiyu-hover); }
.ov-card .k { color: var(--anzhiyu-gray); font-size: .78rem; }
h2 { font-size: 1.2rem; margin: 1.6rem 0 .8rem; position: relative; padding-left: 1.35rem; }
h2::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: ccc 1.6s linear infinite; }
@keyframes ccc { 0% { transform: rotate(0); } to { transform: rotate(-1turn); } }
.chart-row { display: flex; align-items: center; gap: 10px; padding: 5px 0; }
.chart-day { width: 90px; color: var(--anzhiyu-gray); font-size: .84rem; }
.chart-bar { flex: 1; height: 16px; background: var(--anzhiyu-background); border-radius: 8px; overflow: hidden; }
.chart-bar .fill { height: 100%; background: linear-gradient(90deg, var(--anzhiyu-theme), var(--anzhiyu-hover)); border-radius: 8px; transition: width .6s; }
.chart-v { width: 100px; text-align: right; font-size: .84rem; color: var(--anzhiyu-secondary); }
.num { font-variant-numeric: tabular-nums; }
.toolbox { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
.tool-item { background: var(--anzhiyu-card-bg); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 12px 16px; transition: all .25s; }
.tool-item:hover { border-color: var(--anzhiyu-theme); transform: translateY(-2px); box-shadow: var(--card-box-shadow); }
.tool-name { font-weight: 700; font-size: .95rem; }
.tool-desc { color: var(--anzhiyu-gray); font-size: .8rem; margin-top: 2px; }
</style>
