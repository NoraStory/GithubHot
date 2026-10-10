<script setup>
// 全局 Footer（AnZhiYu #footer 同构）：友链组 + 版权 + 底部古诗词打字机
import { onMounted } from 'vue'

onMounted(() => {
  // 参考站 footer-type-tips：jinrishici 诗句 + Typed 打字机
  const el = document.getElementById('footer-type-tips')
  if (!el) return
  if (window.Typed && window.jinrishici && window.jinrishici.load) {
    window.jinrishici.load((result) => {
      const content = result && result.data && result.data.content
      // 回调是异步的：直接传元素引用并二次校验，避免路由切换后选择器落空
      const tips = document.getElementById('footer-type-tips')
      if (content && tips && window.Typed) {
        new window.Typed(tips, {
          strings: [content], startDelay: 300, typeSpeed: 150, loop: true, backSpeed: 70
        })
      } else if (tips) {
        tips.textContent = content || '数据与诗，都在这里相遇'
      }
    })
  } else {
    el.textContent = '数据与诗，都在这里相遇'
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
        <img class="footer_mini_logo" title="返回顶部" alt="返回顶部" src="/img/avatar.webp" onclick="anzhiyu.scrollToDest(0, 500)" size="50px">
        <a class="deal_link" href="/llms.txt" title="llms.txt">
          <i class="anzhiyufont anzhiyu-icon-envelope"></i>
        </a>
        <a class="deal_link" href="/music" title="音乐馆">
          <i class="anzhiyufont anzhiyu-icon-music"></i>
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
          <div class="footer-title">发现</div>
          <div class="footer-links">
            <a class="footer-item" href="/github">GitHub 项目榜</a>
            <a class="footer-item" href="/news">AI 资讯榜</a>
            <a class="footer-item" href="/domestic">国内热榜</a>
            <a class="footer-item" href="/fusion">融合观察</a>
          </div>
        </div>
        <div class="footer-group">
          <div class="footer-title">本站协议</div>
          <div class="footer-links">
            <a class="footer-item" title="隐私协议" href="/privacy">隐私协议</a>
            <a class="footer-item" title="关于本站" href="/about">关于本站</a>
            <a class="footer-item" title="源码" target="_blank" href="https://github.com/NoraStory/GithubHot">源码仓库</a>
          </div>
        </div>
      </div>
    </div>
    <div id="footer-bar">
      <div class="footer-bar-links">
        <div class="footer-bar-left">
          <div id="footer-bar-tips">
            <div class="copyright">&copy;2025 - 2026 By <a class="footer-bar-link" href="/" title="GithubHot">GithubHot</a><span class="icp-sep">｜</span><a class="footer-bar-link icp-link" target="_blank" rel="noopener" href="https://beian.miit.gov.cn/" title="工信部备案查询">陕ICP备2025082213号</a></div>
          </div>
          <div id="footer-type-tips"></div>
        </div>
        <div class="footer-bar-right">
          <a class="footer-bar-link" target="_blank" rel="noopener" href="https://github.com/NoraStory/GithubHot" title="源码">源码</a>
          <a class="footer-bar-link" target="_blank" rel="noopener" href="/api/v1/hot" title="API">API</a>
          <a class="footer-bar-link" href="/healthz" title="服务监测">服务监测</a>
          <a class="footer-bar-link" href="/music" title="音乐馆">音乐馆</a>
        </div>
      </div>
    </div>
  </footer>
</template>

<style scoped>
#footer { padding: 26px 16px 40px; color: var(--anzhiyu-gray); font-size: .88rem; }
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
#footer-bar { padding-top: 12px; border-top: 1px dashed var(--anzhiyu-card-border); }
.footer-bar-links { max-width: 900px; margin: 0 auto; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; }
.footer-bar-left { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; }
.footer-bar-link { color: var(--anzhiyu-gray); margin: 0 6px; }
.footer-bar-link:hover { color: var(--anzhiyu-hover); }
#footer-type-tips { color: var(--anzhiyu-secondtext); font-size: .82rem; min-height: 1.2em; }
.icp-sep { color: var(--anzhiyu-card-border); margin: 0 6px; }
.icp-link { font-size: .84rem; }
</style>
