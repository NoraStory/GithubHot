<script setup>
// 友链/资源页（AnZhiYu flink site-card 同构）：本站全部出口资源 + 申请友链
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import BannerMini from '../components/BannerMini.vue'

const router = useRouter()
const applyOpen = ref(false)

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

// 申请友链：模板复制 → 留言板提交（参考站 addFriendLink 的改造版）
const applyText = computed(() => '昵称（请勿包含博客等字样）：\n网站地址（要求博客地址，请勿提交个人主页）：\n头像图片url（请提供尽可能清晰的图片）：\n描述：\n站点截图（可选）：')
async function copyApply() {
  try {
    await navigator.clipboard.writeText(applyText.value)
    window.anzhiyu && window.anzhiyu.snackbarShow('申请模板已复制，去留言板粘贴提交', false, 2500)
  } catch (e) { /* 忽略 */ }
}
function goMessages() { applyOpen.value = false; router.push('/messages') }
</script>

<template>
  <BannerMini title="友链与资源" subtitle="本站的全部出口：RSS · Agent · API · 源码" />
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div class="flink flink-apply">
          <h2>申请友链（0）</h2>
          <div class="apply-bar">
            <span class="apply-tip">把本站加入你的友链后，复制模板内容到留言板提交即可互链～</span>
            <button class="apply-btn" @click="applyOpen = true">+ 申请友链</button>
          </div>
        </div>
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

  <!-- 申请友链弹窗 -->
  <div v-if="applyOpen" class="apply-mask" @click.self="applyOpen = false">
    <div class="apply-dialog">
      <div class="apply-title">申请友链 <button class="apply-close" @click="applyOpen = false">✕</button></div>
      <p class="apply-desc">按下述格式填写你的站点信息，提交到留言板；管理员审核通过后互链展示。</p>
      <textarea class="apply-textarea" readonly :value="applyText" rows="6"></textarea>
      <div class="apply-actions">
        <button class="apply-btn" @click="copyApply">复制申请内容</button>
        <button class="apply-btn plain" @click="goMessages">去留言板提交 →</button>
      </div>
    </div>
  </div>
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

/* 申请友链 */
.apply-bar { display: flex; align-items: center; justify-content: space-between; gap: 14px; flex-wrap: wrap; background: var(--anzhiyu-background); border-radius: var(--anzhiyu-radius); padding: 14px 18px; }
.apply-tip { color: var(--anzhiyu-gray); font-size: .86rem; }
.apply-btn { background: var(--anzhiyu-theme); color: #fff; border: none; border-radius: 20px; padding: 7px 18px; font-size: .88rem; cursor: pointer; }
.apply-btn:hover { background: var(--anzhiyu-hover); }
.apply-btn.plain { background: transparent; color: var(--anzhiyu-main); border: 1px solid var(--anzhiyu-main); }
.apply-btn.plain:hover { background: var(--anzhiyu-theme-op); }
.apply-mask { position: fixed; inset: 0; z-index: 10020; background: rgba(0,0,0,.45); display: flex; align-items: center; justify-content: center; padding: 20px; }
.apply-dialog { width: min(560px, 92vw); background: var(--anzhiyu-card-bg); border-radius: 12px; padding: 22px 24px; box-shadow: var(--anzhiyu-shadow-main); }
.apply-title { font-weight: 700; font-size: 1.1rem; display: flex; justify-content: space-between; align-items: center; }
.apply-close { background: none; border: none; color: var(--anzhiyu-gray); font-size: 1rem; cursor: pointer; }
.apply-desc { color: var(--anzhiyu-gray); font-size: .84rem; margin: 8px 0 12px; }
.apply-textarea { width: 100%; box-sizing: border-box; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: 8px; color: var(--anzhiyu-fontcolor); font-size: .86rem; line-height: 1.7; padding: 10px 12px; resize: vertical; }
.apply-actions { display: flex; gap: 10px; margin-top: 14px; }
</style>
