<script setup>
// 关于页：post 页式（AnZhiYu 关于页同构）
</script>

<template>
  <header class="post-bg post-bg--purple" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">关于</router-link></div></div>
      <h1 class="post-title">关于 GithubHot</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">GitHub 开源项目热点 × AI 资讯热点 —— 双热度追踪</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <h2>这是什么</h2>
        <p>GithubHot 是一个自己找热点、自己写日报的站点：每天从 GitHub 与 AI 圈两条线采集资料，LLM 预筛 + 独立双评分 + 中文写作，聚簇成事件、按独立来源算热度，并与 GitHub 项目互相印证（融合观察），最终产出双榜与每日报告。</p>
        <h2>架构</h2>
        <p>Go 实现，DDD 四层（domain / application / infrastructure / interfaces）。SQLite 存储，单静态二进制部署；Vue3 前端经 go:embed 内嵌。灵感与示范信源来自 <a href="https://github.com/KKKKhazix/AIHOT" target="_blank" rel="noopener">KKKKhazix/AIHOT</a>（MIT），致谢。</p>
        <h2>两个热度公式</h2>
        <p><strong>AI 资讯事件</strong>：H = Σ(每个独立来源 w(分级) × 0.5^(年龄h/24)) × 10 × 融合加成——同媒体发十篇只算一次，48h 窗口，24h 减半；与 GitHub 项目互证的事件 ×1.25。</p>
        <p><strong>GitHub 项目</strong>：H = 10 × log2(1 + 窗口内新增 star) + trending 加成 + 新爆加成——增量而非存量，快照差分，重复抓取不虚增。</p>
        <h2>给 Agent 与 APP</h2>
        <p>REST API（<code>/api/v1/*</code>）即未来手机 APP 的契约；另有 RSS 三路订阅、<code>/llms.txt</code>、<code>/api/v1/agent/hot.md</code> 与 MCP 服务器（<code>githubhot mcp</code>）。</p>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 19rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.article p { color: var(--anzhiyu-secondary); margin: .5rem 0; }
.article a { color: var(--anzhiyu-blue); }
.article a:hover { color: var(--anzhiyu-hover); }
.article code { background: var(--anzhiyu-background); padding: 1px 6px; border-radius: 4px; font-size: .85rem; }
@media (max-width: 768px) { .post-bg { height: 15rem; } }
</style>
