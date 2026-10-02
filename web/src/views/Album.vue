<script setup>
// 事件画廊（参考站 /album 的 justified 瀑布流结构，fjGallery 已自托管）
import { ref, onMounted, nextTick } from 'vue'
import { api } from '../lib/api'

const stories = ref([])
const loading = ref(true)
const gridEl = ref(null)

function coverOf(title, idx) {
  let h = (idx * 47) % 360
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  const a = `hsl(${h}, 42%, 62%)`
  const b = `hsl(${(h + 40) % 360}, 46%, 44%)`
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='600' height='420'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs><rect width='600' height='420' fill='url(#g)'/><circle cx='480' cy='80' r='120' fill='rgba(255,255,255,0.12)'/><circle cx='100' cy='350' r='80' fill='rgba(255,255,255,0.09)'/><text x='50%' y='92%' fill='rgba(255,255,255,0.85)' font-size='26' text-anchor='middle' font-family='monospace'>${title.length > 18 ? title.slice(0, 18) + '…' : title}</text></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

function initGallery() {
  if (window.fjGallery && gridEl.value) {
    window.fjGallery(gridEl.value, { rowHeight: 180, margins: 6 })
  }
}

onMounted(async () => {
  const d = await api.get('/api/v1/hot/news')
  stories.value = d.items || []
  loading.value = false
  await nextTick()
  initGallery()
})
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><a class="post-meta-original">相册</a></div></div>
      <h1 class="post-title">事件画廊</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">按热度排列的事件封面 · 点击进入事件详情</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!stories.length" class="empty">暂无事件</div>
        <div v-else ref="gridEl" class="justified-gallery">
          <router-link v-for="(s, i) in stories" :key="s.storyId" :to="`/story/${s.storyId}`" :title="s.titleZh">
            <img :src="coverOf(s.titleZh, i)" :alt="s.titleZh">
            <div class="gallery-caption">{{ s.titleZh }}</div>
          </router-link>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.justified-gallery { width: 100%; position: relative; overflow: hidden; }
.gallery-caption { position: absolute; left: 0; right: 0; bottom: 0; background: linear-gradient(transparent, rgba(0,0,0,.75)); color: #fff; font-size: .78rem; padding: 18px 10px 6px; opacity: 0; transition: opacity .25s; }
.justified-gallery > a { position: absolute; overflow: hidden; border-radius: 6px; }
.justified-gallery > a:hover .gallery-caption { opacity: 1; }
.justified-gallery img { width: 100%; height: 100%; object-fit: cover; }
@media (max-width: 768px) { .post-bg { height: 14rem; } }
</style>
