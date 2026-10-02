<script setup>
// 小空调（参考站同款：外部 Vue 组件脚本挂载到 #air-conditioner-vue）
import { onMounted } from 'vue'

onMounted(() => {
  if (document.getElementById('air-conditioner-script')) return
  const s = document.createElement('script')
  s.id = 'air-conditioner-script'
  s.src = '/anzhiyu/js/air-conditioner.js'
  document.body.appendChild(s)
})
</script>

<template>
  <header class="post-bg post-bg--sky" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">站点</router-link></div></div>
      <h1 class="post-title">小空调</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">天热了，进来吹会儿空调～</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <blockquote class="quote"><p>yesterday, 吹下空调~</p></blockquote>
        <div id="air-conditioner-vue"></div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 16rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.quote { margin: 0 0 1rem; padding: 10px 16px; background: var(--anzhiyu-background); border-left: 3px solid var(--anzhiyu-theme); border-radius: 6px; color: var(--anzhiyu-secondary); }
.quote p { margin: 0; }
#air-conditioner-vue { min-height: 300px; }
</style>
