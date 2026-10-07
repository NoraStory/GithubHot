<script setup>
// 事件画廊（参考站 /album justified 瀑布流 + fancybox 灯箱，改造成 Vue3 内建灯箱）
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'

const router = useRouter()
const stories = ref([])
const loading = ref(true)
const gridEl = ref(null)
const lightbox = ref(null) // 当前预览下标

// ===== 背景视频墙：主页背景片单全量收录（横屏 + 竖屏两组）=====
// 全部走 /video/ 代理路径（服务端磁盘缓存）。preload="none"：卡片不点不联网，
// 16 支视频同屏也不会像背景轮播那样并发缓冲拖垮带宽。
const videoGroups = ref([])
function vidName(src) {
  const m = String(src).match(/([^/]+)\.mp4$/)
  return m ? m[1] : src
}
function togglePlay(e) {
  const v = e.target
  if (v.paused) { const p = v.play(); if (p && p.catch) p.catch(() => {}) }
  else v.pause()
}

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

// 灯箱：图片点击预览；标题点击进详情；支持 ←/→/Esc
function openLightbox(i) { lightbox.value = i }
function lbPrev() { lightbox.value = (lightbox.value - 1 + stories.value.length) % stories.value.length }
function lbNext() { lightbox.value = (lightbox.value + 1) % stories.value.length }
function lbClose() { lightbox.value = null }
function lbDetail() {
  const s = stories.value[lightbox.value]
  if (s) { lightbox.value = null; router.push(`/story/${s.storyId}`) }
}
function onKey(e) {
  if (lightbox.value === null) return
  if (e.key === 'Escape') lbClose()
  else if (e.key === 'ArrowLeft') lbPrev()
  else if (e.key === 'ArrowRight') lbNext()
}

onMounted(async () => {
  const d = await api.get('/api/v1/hot/news')
  stories.value = d.items || []
  loading.value = false
  // 背景片单：失败静默（区块隐藏，不影响画廊主体）
  api.get('/api/v1/site/config').then((cfg) => {
    if (!cfg) return
    const land = String(cfg.homeVideos || '').split('|').filter(Boolean)
    const port = String(cfg.homePortraitVideos || '').split('|').filter(Boolean)
    const groups = []
    if (land.length) groups.push({ label: '横屏 · 主页背景', aspect: '16 / 9', items: land })
    if (port.length) groups.push({ label: '竖屏 · 移动端背景', aspect: '9 / 16', items: port })
    videoGroups.value = groups
  }).catch(() => {})
  await nextTick()
  initGallery()
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))
</script>

<template>
  <header class="post-bg post-bg--sunset" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">相册</router-link></div></div>
      <h1 class="post-title">事件画廊</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">点击图片预览大图 · 点击标题进入事件详情</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <!-- 背景视频墙：主页轮播片单全量收录，点击播放/暂停 -->
        <section v-for="g in videoGroups" :key="g.label" class="video-wall">
          <h2 class="video-wall-title"><i class="anzhiyufont anzhiyu-icon-film"></i> {{ g.label }}（{{ g.items.length }}）</h2>
          <div class="video-grid" :class="{ portrait: g.aspect === '9 / 16' }" :style="{ '--vid-aspect': g.aspect }">
            <figure class="video-card" v-for="src in g.items" :key="src">
              <video :src="src" muted loop playsinline preload="none" @click="togglePlay"></video>
              <figcaption>{{ vidName(src) }}</figcaption>
            </figure>
          </div>
        </section>

        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!stories.length" class="empty">暂无事件</div>
        <div v-else ref="gridEl" class="justified-gallery">
          <a v-for="(s, i) in stories" :key="s.storyId" class="fj-gallery-item" :title="s.titleZh">
            <img :src="coverOf(s.titleZh, i)" :alt="s.titleZh" @click.prevent.stop="openLightbox(i)">
            <div class="gallery-caption">
              <router-link :to="`/story/${s.storyId}`" @click.stop>{{ s.titleZh }}</router-link>
            </div>
          </a>
        </div>
      </div>
    </div>
  </main>

  <!-- 灯箱（参考站 fancybox 的 Vue3 改造版） -->
  <div v-if="lightbox !== null" class="gh-lightbox" @click="lbClose">
    <div class="gh-lb-body" @click.stop>
      <img :src="coverOf(stories[lightbox].titleZh, lightbox)" :alt="stories[lightbox].titleZh">
      <div class="gh-lb-caption">
        <span>{{ stories[lightbox].titleZh }}</span>
        <button class="gh-lb-detail" @click="lbDetail">查看详情 →</button>
      </div>
    </div>
    <button class="gh-lb-nav gh-lb-prev" @click.stop="lbPrev">‹</button>
    <button class="gh-lb-nav gh-lb-next" @click.stop="lbNext">›</button>
    <button class="gh-lb-close" @click="lbClose">✕</button>
  </div>
</template>

<style scoped>
#post { max-width: 100%; margin: 0; }

/* ===== 背景视频墙 ===== */
.video-wall { margin-bottom: 28px; }
.video-wall-title { font-size: 1.15rem; font-weight: 700; color: var(--anzhiyu-fontcolor); margin: 6px 0 12px; }
.video-wall-title i { color: var(--anzhiyu-theme); margin-right: 4px; }
.video-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 12px; }
.video-grid.portrait { grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); }
.video-card { margin: 0; }
.video-card video { display: block; width: 100%; aspect-ratio: var(--vid-aspect); object-fit: cover; border-radius: 10px; background: #000; cursor: pointer; box-shadow: var(--card-box-shadow); }
.video-card video:hover { filter: brightness(1.08); }
.video-card figcaption { text-align: center; font-size: .76rem; color: var(--anzhiyu-gray); padding-top: 5px; }

.post-bg { height: 18rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.justified-gallery { width: 100%; position: relative; overflow: hidden; }
.gallery-caption { position: absolute; left: 0; right: 0; bottom: 0; color: #fff; font-size: .78rem; padding: 18px 10px 6px; opacity: 0; transition: opacity .25s; }
.gallery-caption a { color: #fff; }
.gallery-caption a:hover { color: var(--anzhiyu-theme); }
.justified-gallery > a { position: absolute; overflow: hidden; border-radius: 6px; }
.justified-gallery > a:hover .gallery-caption { opacity: 1; }
.justified-gallery img { width: 100%; height: 100%; object-fit: cover; cursor: zoom-in; }
@media (max-width: 768px) { .post-bg { height: 14rem; } }

/* 灯箱 */
.gh-lightbox { position: fixed; inset: 0; z-index: 10010; background: rgba(0,0,0,.82); display: flex; align-items: center; justify-content: center; }
.gh-lb-body { max-width: 82vw; max-height: 84vh; display: flex; flex-direction: column; align-items: center; }
.gh-lb-body img { max-width: 82vw; max-height: 76vh; border-radius: 8px; box-shadow: 0 12px 48px rgba(0,0,0,.5); }
.gh-lb-caption { display: flex; align-items: center; gap: 14px; color: #fff; padding: 12px 4px 0; font-size: .92rem; }
.gh-lb-detail { background: var(--anzhiyu-theme); color: #fff; border: none; border-radius: 20px; padding: 4px 14px; font-size: .82rem; cursor: pointer; }
.gh-lb-detail:hover { background: var(--anzhiyu-hover); }
.gh-lb-close { position: absolute; top: 22px; right: 26px; background: rgba(255,255,255,.12); border: none; color: #fff; width: 42px; height: 42px; border-radius: 50%; font-size: 1.1rem; cursor: pointer; }
.gh-lb-close:hover { background: rgba(255,255,255,.25); }
.gh-lb-nav { position: absolute; top: 50%; transform: translateY(-50%); background: rgba(255,255,255,.12); border: none; color: #fff; width: 46px; height: 46px; border-radius: 50%; font-size: 1.5rem; cursor: pointer; }
.gh-lb-nav:hover { background: rgba(255,255,255,.25); }
.gh-lb-prev { left: 26px; }
.gh-lb-next { right: 26px; }
</style>
