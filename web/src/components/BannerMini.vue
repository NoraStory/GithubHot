<script setup>
// 子页横幅：与文章页/事件页一致的 post-bg 页头（面包屑药丸 + 标题 + 副标题）
defineProps({ title: String, subtitle: String })
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline"><router-link class="post-meta-original" to="/">{{ title }}</router-link></div>
      </div>
      <h1 class="post-title">{{ title }}</h1>
      <div id="post-meta">
        <div class="meta-firstline"><span class="post-meta-label">{{ subtitle }}</span></div>
      </div>
    </div>
  </header>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .88rem; }
@media (max-width: 768px) { .post-bg { height: 15rem; } .post-title { font-size: 1.45rem; } }
</style>
