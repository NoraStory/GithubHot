<script setup>
import BannerMini from '../components/BannerMini.vue'
</script>

<template>
  <BannerMini title="关于 GithubHot" subtitle="GitHub 开源项目热点 × AI 资讯热点 —— 双热度追踪" />
  <div class="layout page-enter">
    <main id="article-container">
      <div class="card article">
        <h2 class="first-title">这是什么</h2>
        <p>GithubHot 是一个自己找热点、自己写日报的站点：每天从 GitHub 与 AI 圈两条线采集资料，LLM 预筛 + 独立双评分 + 中文写作，聚簇成事件、按独立来源算热度，并与 GitHub 项目互相印证（融合观察），最终产出双榜与每日报告。</p>
        <h2>架构</h2>
        <p>Go 实现，DDD 四层（domain / application / infrastructure / interfaces）。SQLite 存储，单静态二进制部署；Vue3 前端。灵感与示范信源来自 <a href="https://github.com/KKKKhazix/AIHOT" target="_blank" rel="noopener">KKKKhazix/AIHOT</a>（MIT），致谢。</p>
        <h2>两个热度公式</h2>
        <p><strong>AI 资讯事件</strong>：H = Σ(每个独立来源 w(分级) × 0.5^(年龄h/24)) × 10 × 融合加成——同媒体发十篇只算一次，48h 窗口，24h 减半；与 GitHub 项目互证的事件 ×1.25。</p>
        <p><strong>GitHub 项目</strong>：H = 10 × log2(1 + 24h 新增 star) + trending 加成 + 新爆加成——增量而非存量，快照差分，重复抓取不虚增。</p>
        <h2>给 Agent</h2>
        <p>REST API（<code>/api/v1/*</code>）即未来手机 APP 的契约；另有 RSS、<code>/llms.txt</code>、<code>/api/v1/agent/hot.md</code> 与 MCP 服务器（<code>githubhot mcp</code>）。</p>
      </div>
    </main>
  </div>
</template>

<style scoped>
p { color: var(--anzhiyu-secondary); margin: .5rem 0; }
a { color: var(--anzhiyu-blue); }
a:hover { color: var(--anzhiyu-hover); }
code { background: var(--anzhiyu-background); padding: 1px 6px; border-radius: 4px; font-size: .85rem; }
</style>
