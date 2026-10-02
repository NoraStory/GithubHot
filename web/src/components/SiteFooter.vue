<script setup>
// 全局 Footer（AnZhiYu #footer 同构）+ APlayer 固定播放器挂载
import { ref, onMounted } from 'vue'
import { loadSiteConfig } from '../lib/api'

const footerRef = ref(null)
let player = null
const musicOn = ref(false)

onMounted(async () => {
  const cfg = await loadSiteConfig()
  const list = cfg.music || []
  if (list.length && window.APlayer) {
    player = new window.APlayer({
      container: footerRef.value.querySelector('#aplayer-mount'),
      fixed: true,
      mini: true,
      autoplay: false,
      theme: '#eabcbd',
      audio: list.map((t) => ({ name: t.name, artist: t.artist || '', url: t.url, cover: t.cover || '' }))
    })
    musicOn.value = true
  }
})
</script>

<template>
  <footer id="footer">
    <div id="footer-wrap">
      <div id="footer_deal">
        <a class="deal_link" href="https://github.com/NoraStory/GithubHot" target="_blank" title="Github">
          <i class="anzhiyufont anzhiyu-icon-github"></i>
        </a>
        <a class="deal_link" href="/feed/digest.xml" title="RSS">
          <i class="anzhiyufont anzhiyu-icon-rss"></i>
        </a>
        <img class="footer_mini_logo" title="返回顶部" alt="返回顶部" src="data:image/svg+xml;utf8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='50' height='50'%3E%3Ccircle cx='25' cy='25' r='22' fill='%23eabcbd'/%3E%3Ctext x='25' y='32' font-size='20' text-anchor='middle'%3E🔥%3C/text%3E%3C/svg%3E" onclick="window.scrollTo({ top: 0, behavior: 'smooth' })">
        <a class="deal_link" href="/llms.txt" title="llms.txt">
          <i class="anzhiyufont anzhiyu-icon-envelope"></i>
        </a>
      </div>
      <div id="anzhiyu-footer">
        <div class="footer-group">
          <div class="footer-title">订阅</div>
          <div class="footer-links">
            <a class="footer-item" href="/feed/news.xml" target="_blank">资讯 RSS</a>
            <a class="footer-item" href="/feed/github.xml" target="_blank">项目 RSS</a>
            <a class="footer-item" href="/feed/digest.xml" target="_blank">期刊 RSS</a>
          </div>
        </div>
        <div class="footer-group">
          <div class="footer-title">Agent</div>
          <div class="footer-links">
            <a class="footer-item" href="/llms.txt" target="_blank">llms.txt</a>
            <a class="footer-item" href="/api/v1/agent/hot.md" target="_blank">Markdown 报告</a>
            <a class="footer-item" href="/api/v1/hot" target="_blank">JSON API</a>
          </div>
        </div>
        <div class="footer-group">
          <div class="footer-title">站点</div>
          <div class="footer-links">
            <a class="footer-item" href="/about">关于本站</a>
            <a class="footer-item" href="/digests">全部期刊</a>
            <a class="footer-item" href="https://github.com/NoraStory/GithubHot" target="_blank">源码</a>
          </div>
        </div>
      </div>
      <div id="footer-bar">
        <span>由 <a href="https://github.com/NoraStory/GithubHot">GithubHot</a> 自动生成 · 数据经本地流水线采集与 LLM 精选</span>
        <template v-if="musicOn">
          <span class="sep">|</span>
          <span>🎵 背景音乐已就绪（左下角）</span>
        </template>
      </div>
    </div>
    <div id="aplayer-mount"></div>
  </footer>
</template>

<style scoped>
#footer { padding: 26px 16px 90px; color: var(--anzhiyu-gray); font-size: .88rem; }
#footer-wrap { max-width: 900px; margin: 0 auto; text-align: center; }
#footer_deal { display: flex; gap: 18px; justify-content: center; align-items: center; margin-bottom: 14px; }
.deal_link { color: var(--anzhiyu-fontcolor); font-size: 1.15rem; }
.deal_link:hover { color: var(--anzhiyu-hover); }
.footer_mini_logo { width: 42px; height: 42px; border-radius: 50%; cursor: pointer; transition: transform .3s; }
.footer_mini_logo:hover { transform: translateY(-3px); }
#anzhiyu-footer { display: flex; gap: 30px; justify-content: center; flex-wrap: wrap; margin-bottom: 12px; }
.footer-group { text-align: left; }
.footer-title { font-weight: 700; margin-bottom: 4px; color: var(--anzhiyu-fontcolor); }
.footer-links { display: flex; flex-direction: column; gap: 2px; }
.footer-item { color: var(--anzhiyu-gray); font-size: .84rem; }
.footer-item:hover { color: var(--anzhiyu-hover); }
#footer-bar .sep { margin: 0 8px; color: var(--anzhiyu-card-border); }
</style>
