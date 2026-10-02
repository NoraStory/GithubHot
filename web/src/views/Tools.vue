<script setup>
// 工具箱（参考站 /tools 分组下载卡结构）：API 组 + Agent 组 + 资源组
import BannerMini from '../components/BannerMini.vue'

const groups = [
  {
    title: 'API 端点',
    desc: '面向 APP / 脚本 / Agent 的 REST 接口',
    tools: [
      { name: '三榜合一', desc: 'GET /api/v1/hot', url: '/api/v1/hot' },
      { name: 'GitHub 项目榜', desc: 'GET /api/v1/hot/github', url: '/api/v1/hot/github' },
      { name: 'AI 资讯榜', desc: 'GET /api/v1/hot/news', url: '/api/v1/hot/news' },
      { name: '融合配对', desc: 'GET /api/v1/hot/fusion', url: '/api/v1/hot/fusion' },
      { name: '期刊列表', desc: 'GET /api/v1/digests', url: '/api/v1/digests?page=1&pageSize=10' },
      { name: '事件详情', desc: 'GET /api/v1/story/{id}', url: '/api/v1/story/story-ac6f3cc6f4' },
      { name: '站内搜索', desc: 'GET /api/v1/search?q=', url: '/api/v1/search?q=agent' },
      { name: '日报 Markdown', desc: 'GET /api/v1/digest/latest?format=raw', url: '/api/v1/digest/latest?format=raw' }
    ]
  },
  {
    title: 'Agent 接入',
    desc: '给 Claude / ChatGPT / 自定义 Agent 的入口',
    tools: [
      { name: 'llms.txt', desc: '站点说明与端点索引', url: '/llms.txt' },
      { name: 'Agent Markdown', desc: '双榜完整 Markdown 报告', url: '/api/v1/agent/hot.md' },
      { name: 'MCP 服务器', desc: 'githubhot mcp（stdio JSON-RPC，5 个工具）', url: 'https://github.com/NoraStory/GithubHot#mcp--脚本推送--精选校准' }
    ]
  },
  {
    title: '订阅',
    desc: 'RSS 三路',
    tools: [
      { name: '资讯 RSS', desc: '/feed/news.xml', url: '/feed/news.xml' },
      { name: '项目 RSS', desc: '/feed/github.xml', url: '/feed/github.xml' },
      { name: '期刊 RSS', desc: '/feed/digest.xml', url: '/feed/digest.xml' }
    ]
  }
]
</script>

<template>
  <BannerMini title="工具库" subtitle="API · Agent · 订阅的全部工具入口" />
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <section v-for="g in groups" :key="g.title">
          <h2>{{ g.title }}</h2>
          <p class="group-desc">{{ g.desc }}</p>
          <div class="tool-grid">
            <a v-for="t in g.tools" :key="t.name" class="tool-card fade-up" :href="t.url" :target="t.url.startsWith('http') ? '_blank' : '_self'" rel="noopener">
              <div class="tool-name">{{ t.name }}</div>
              <div class="tool-desc">{{ t.desc }}</div>
            </a>
          </div>
        </section>
      </div>
    </div>
  </main>
</template>

<style scoped>
section h2 { font-size: 1.25rem; margin: 1.8rem 0 .4rem; position: relative; padding-left: 1.35rem; }
section h2::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: ccc 1.6s linear infinite; }
@keyframes ccc { 0% { transform: rotate(0); } to { transform: rotate(-1turn); } }
.group-desc { color: var(--anzhiyu-gray); font-size: .84rem; margin-bottom: 12px; }
.tool-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 12px; }
.tool-card { display: block; background: var(--anzhiyu-card-bg); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 14px 16px; transition: all .25s; }
.tool-card:hover { border-color: var(--anzhiyu-theme); transform: translateY(-2px); box-shadow: var(--card-box-shadow); }
.tool-name { font-weight: 700; font-size: .95rem; color: var(--anzhiyu-fontcolor); }
.tool-desc { color: var(--anzhiyu-gray); font-size: .8rem; margin-top: 2px; }
</style>
