<script setup>
// 友链/资源页（AnZhiYu flink site-card 同构）：本站全部出口资源
import BannerMini from '../components/BannerMini.vue'

const groups = [
  {
    title: '订阅（RSS）',
    sites: [
      { name: 'AI 资讯榜 RSS', desc: '精选事件热度榜', url: '/feed/news.xml', icon: '🤖' },
      { name: 'GitHub 项目榜 RSS', desc: '项目增长热度榜', url: '/feed/github.xml', icon: '🔥' },
      { name: '期刊 RSS', desc: '日报/周报/月报', url: '/feed/digest.xml', icon: '📰' }
    ]
  },
  {
    title: 'Agent 接入',
    sites: [
      { name: 'llms.txt', desc: '站点说明与端点索引', url: '/llms.txt', icon: '📄' },
      { name: 'Agent Markdown', desc: '双榜 Markdown 报告', url: '/api/v1/agent/hot.md', icon: '📝' },
      { name: 'JSON API', desc: '/api/v1/hot 三榜合一', url: '/api/v1/hot', icon: '🔗' },
      { name: 'MCP 服务器', desc: 'githubhot mcp（stdio）', url: 'https://github.com/NoraStory/GithubHot#mcp--脚本推送--精选校准', icon: '🔌' }
    ]
  },
  {
    title: '项目',
    sites: [
      { name: '源码仓库', desc: 'NoraStory/GithubHot', url: 'https://github.com/NoraStory/GithubHot', icon: '📦' },
      { name: '灵感致谢', desc: 'KKKKhazix/AIHOT（MIT）', url: 'https://github.com/KKKKhazix/AIHOT', icon: '💡' }
    ]
  }
]
</script>

<template>
  <BannerMini title="友链与资源" subtitle="本站的全部出口：RSS · Agent · API · 源码" />
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-for="g in groups" :key="g.title" class="flink">
          <h2>{{ g.title }}（{{ g.sites.length }}）</h2>
          <div class="site-card-group">
            <div v-for="s in g.sites" :key="s.url + s.name" class="site-card fade-up">
              <a class="img" :href="s.url" :target="s.url.startsWith('http') ? '_blank' : '_self'">
                <span class="flink-avatar">{{ s.icon }}</span>
              </a>
              <a class="info" :href="s.url" :target="s.url.startsWith('http') ? '_blank' : '_self'">
                <div class="site-card-avatar"><span class="flink-avatar">{{ s.icon }}</span></div>
                <div class="site-card-text">
                  <div class="site-card-name">{{ s.name }}</div>
                  <div class="site-card-desc">{{ s.desc }}</div>
                </div>
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.flink h2 { font-size: 1.25rem; margin: 1.6rem 0 .9rem; position: relative; padding-left: 1.35rem; }
.flink h2::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: ccc 1.6s linear infinite; }
@keyframes ccc { 0% { transform: rotate(0); } to { transform: rotate(-1turn); } }
.site-card-group { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 14px; }
.site-card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); overflow: hidden; transition: box-shadow .3s, transform .3s; display: block; }
.site-card:hover { box-shadow: var(--card-hover-box-shadow); transform: translateY(-3px); }
.site-card .img { display: flex; align-items: center; justify-content: center; height: 70px; background: var(--anzhiyu-theme-op); font-size: 2rem; }
.flink-avatar { font-size: 2rem; }
.site-card .info { display: flex; align-items: center; gap: 10px; padding: 10px 14px; }
.site-card-name { font-weight: 700; font-size: .95rem; }
.site-card-desc { color: var(--anzhiyu-gray); font-size: .8rem; }
</style>
