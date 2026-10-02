<script setup>
// 隐私协议（静态页，post 页式）
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">协议</router-link></div></div>
      <h1 class="post-title">隐私协议</h1>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <h2>数据采集</h2>
        <p>本站通过公开信源（RSS / Hacker News / GitHub 公开 API）采集**公开内容**，不收集任何个人信息。GitHub API 调用使用服务器配置的只读令牌。</p>
        <h2>日志与留言</h2>
        <p>服务日志记录访问 IP 与请求路径，仅用于故障排查与限频（留言板 30 秒限频即基于 IP 摘要，不存储原始 IP）。留言板内容为公开展示，请勿提交个人敏感信息。</p>
        <h2>模型调用</h2>
        <p>精选与摘要环节调用大模型 API（火山方舟），发送的内容仅为公开资讯的标题与摘要，不包含用户数据。</p>
        <h2>Cookie 与本地存储</h2>
        <p>本站不使用跟踪 Cookie；浏览器 localStorage 仅保存深色模式偏好与管理端令牌（本地工具用途）。</p>
        <h2>联系我们</h2>
        <p>如对隐私政策有疑问，欢迎在 <router-link to="/messages">留言板</router-link> 或 <a href="https://github.com/NoraStory/GithubHot" target="_blank" rel="noopener">GitHub 仓库</a> 留言。</p>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 16rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
h2 { font-size: 1.2rem; margin: 1.6rem 0 .6rem; position: relative; padding-left: 1.35rem; }
h2::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: ccc 1.6s linear infinite; }
@keyframes ccc { 0% { transform: rotate(0); } to { transform: rotate(-1turn); } }
p { color: var(--anzhiyu-secondary); margin: .5rem 0; }
p a { color: var(--anzhiyu-blue); }
p a:hover { color: var(--anzhiyu-hover); }
</style>
