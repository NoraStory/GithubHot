<script setup>
// 自定义音乐播放器（参考站 #custom-music-player-placeholder 同构，改造成 Vue3 组件）
// 原版 music-index.js 直连第三方 Meting API；这里走本服务 /api/v1/music/playlist 代理，
// DOM 类名与原版前缀保持一致（anzhiyuCustomPlayer-*），样式块也从原版注入样式逐条移植。
// 唱片占位：不用纯色底，跑"随便逛逛"同款人群动画（people.webp 7×15 精灵图的迷你版）。
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'

const playlist = ref([])
const index = ref(0)
const rotating = ref(false)
const status = ref('正在加载...')
const failed = ref(false)
const audioEl = ref(null)
const crowdEl = ref(null)

const defaultCover = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'
const current = computed(() => playlist.value[index.value] || null)
const hasCover = computed(() => !!(current.value && current.value.pic))
const coverError = ref(false)
const showCover = computed(() => hasCover.value && !coverError.value)
const cover = computed(() => {
  const t = current.value
  if (!t || !t.pic) return defaultCover
  return String(t.pic).replace(/param=\d+y\d+/, 'param=256y256')
})

function pad(n) { return String(n).padStart(2, '0') }

function select(i, autoplay = false) {
  if (!playlist.value.length) return null
  index.value = i
  const t = playlist.value[i]
  const a = audioEl.value
  if (!a || !t || !t.url) return null
  const target = new URL(t.url, window.location.href).href
  if (a.src !== target) a.src = target
  if (autoplay) {
    const p = a.play()
    // 把播放结果抛给调用方：自动播放被浏览器策略拦截时好做兜底
    return p && p.catch ? p : Promise.resolve()
  }
  return null
}

// 进站自动播放：随机挑一首起播。
// Chrome 等浏览器默认拦截"带声音的自动播放"，分两级兜底：
//  1) 被拒后改静音播放（唱片转、进度走，视觉正常）；
//  2) 首次点击/按键时恢复声音（若被暂停则补播）。
// 用户主动暂停后不自动恢复。
const soundBlocked = ref(false)
let fallbackArmed = false
function armFallback() {
  if (fallbackArmed) return
  fallbackArmed = true
  const kick = () => {
    document.removeEventListener('click', kick)
    document.removeEventListener('keydown', kick)
    soundBlocked.value = false
    const a = audioEl.value
    if (!a) return
    a.muted = false
    if (a.paused) select(index.value, true)
  }
  document.addEventListener('click', kick)
  document.addEventListener('keydown', kick)
}

function startRandom() {
  if (!playlist.value.length) return
  const i = Math.floor(Math.random() * playlist.value.length)
  const p = select(i, true)
  if (!p) { armFallback(); return }
  p.catch((e) => {
    const a = audioEl.value
    if (!a) return
    if (e && e.name === 'NotAllowedError') {
      // 带声音被自动播放策略拒绝 → 静音先播起来，等首次交互再恢复声音
      a.muted = true
      const mp = a.play()
      soundBlocked.value = true
      armFallback()
      if (mp && mp.catch) mp.catch(() => { a.muted = false })
      return
    }
    // NotSupportedError（源是 302 重定向，元数据未就绪）→ 等 canplay 后再播
    const resume = () => {
      a.removeEventListener('canplay', resume)
      const q = a.play()
      if (q && q.catch) q.catch(() => {})
    }
    a.addEventListener('canplay', resume)
    setTimeout(() => a.removeEventListener('canplay', resume), 15000)
  })
}

function next() {
  if (!playlist.value.length) return
  select((index.value + 1) % playlist.value.length, true)
}

function onPlay() { rotating.value = true }
function onPause() { rotating.value = false }

onMounted(async () => {
  if (!showCover.value) crowdStop = startCrowd(crowdEl.value)
  const cfg = (window.GLOBAL_CONFIG && window.GLOBAL_CONFIG.musicPlayer) || {}
  const id = /^\d+$/.test(String(cfg.playlistId)) ? String(cfg.playlistId) : '652135520'
  const server = ['netease', 'tencent', 'kugou', 'baidu'].includes(String(cfg.server)) ? String(cfg.server) : 'netease'
  const load = async () => {
    try {
      const r = await fetch(`/api/v1/music/playlist?id=${id}&server=${server}`)
      if (!r.ok) throw new Error('HTTP ' + r.status)
      const data = await r.json()
      if (Array.isArray(data) && data.length) {
        playlist.value = data
        status.value = ''
        startRandom()
        return true
      }
      status.value = '歌单为空'
    } catch (e) {
      status.value = '加载失败'
      playlist.value = []
    }
    return false
  }
  if (!(await load())) {
    // 失败自动重试一次（网络/代理抖动），仍失败则给出手动重试按钮
    setTimeout(async () => {
      if (!(await load())) failed.value = true
    }, 1500)
  }
})

async function retry() {
  failed.value = false
  status.value = '正在加载...'
  const cfg = (window.GLOBAL_CONFIG && window.GLOBAL_CONFIG.musicPlayer) || {}
  const id = /^\d+$/.test(String(cfg.playlistId)) ? String(cfg.playlistId) : '652135520'
  const server = ['netease', 'tencent', 'kugou', 'baidu'].includes(String(cfg.server)) ? String(cfg.server) : 'netease'
  try {
    const r = await fetch(`/api/v1/music/playlist?id=${id}&server=${server}`)
    const data = await r.json()
    if (Array.isArray(data) && data.length) {
      playlist.value = data
      status.value = ''
      startRandom()
      return
    }
    status.value = '歌单为空'
  } catch (e) {
    status.value = '加载失败'
    failed.value = true
  }
}

// ===== 唱片占位：随便逛逛同款人群动画（people_2.js 的迷你独立版）=====
// people_2.js 只驱动全站唯一 #peoplecanvas；这里用同一张精灵图在唱片位
// 跑一个简化版：随机小人左右穿行。有真实封面时停止并盖住画布。
let crowdStop = null

// 封面加载失败时回退到人群动画占位，唱片位不会只剩白盘
watch(showCover, (v) => {
  if (v) {
    if (crowdStop) { crowdStop(); crowdStop = null }
  } else if (!crowdStop) {
    crowdStop = startCrowd(crowdEl.value)
  }
})

function startCrowd(canvas) {
  if (!canvas) return () => {}
  const ctx = canvas.getContext('2d')
  const COLS = 7, ROWS = 15
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const img = new Image()
  let W = 0, H = 0, fw = 0, fh = 0, raf = 0
  let alive = true
  const peeps = []

  const resize = () => {
    const r = canvas.parentElement.getBoundingClientRect()
    W = r.width; H = r.height
    canvas.width = W * dpr
    canvas.height = H * dpr
  }
  const spawn = () => {
    if (!fw) return
    const h = H * (0.32 + Math.random() * 0.26)
    const w = h * (fw / fh)
    const dir = Math.random() < 0.5 ? 1 : -1
    peeps.push({
      sx: (Math.random() * COLS | 0) * fw,
      sy: (Math.random() * ROWS | 0) * fh,
      x: dir > 0 ? -w : W + w * 0.2,
      y: H - h * (0.92 + Math.random() * 0.3) + h * 0.3,
      w, h, dir,
      v: 0.28 + Math.random() * 0.5,
    })
  }
  const tick = () => {
    if (!alive) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    ctx.save()
    ctx.scale(dpr, dpr)
    for (let i = peeps.length - 1; i >= 0; i--) {
      const p = peeps[i]
      p.x += p.v * p.dir
      if ((p.dir > 0 && p.x > W + p.w) || (p.dir < 0 && p.x < -p.w)) {
        peeps.splice(i, 1)
        spawn()
        continue
      }
      ctx.save()
      if (p.dir < 0) {
        ctx.translate(p.x + p.w / 2, 0)
        ctx.scale(-1, 1)
        ctx.translate(-(p.x + p.w / 2), 0)
      }
      ctx.drawImage(img, p.sx, p.sy, fw, fh, p.x, p.y, p.w, p.h)
      ctx.restore()
    }
    ctx.restore()
    raf = requestAnimationFrame(tick)
  }
  img.onload = () => {
    fw = img.width / COLS
    fh = img.height / ROWS
    resize()
    for (let i = 0; i < 8; i++) {
      spawn()
      // 初始种群直接散布在画布内，避免开场空盘等小人走进来
      const p = peeps[peeps.length - 1]
      if (p) p.x = Math.random() * (W + p.w) - p.w
    }
    tick()
  }
  img.src = '/anzhiyu/img/people.webp'
  window.addEventListener('resize', resize)
  return () => {
    alive = false
    cancelAnimationFrame(raf)
    window.removeEventListener('resize', resize)
  }
}

onBeforeUnmount(() => { if (crowdStop) crowdStop() })
</script>

<template>
  <div class="anzhiyuCustomPlayer-player-container-v3">
    <div class="anzhiyuCustomPlayer-left-column">
      <div id="anzhiyuCustomPlayer-cover-art-wrapper" class="anzhiyuCustomPlayer-cover-art-wrapper">
        <canvas ref="crowdEl" class="disc-crowd" aria-hidden="true"></canvas>
        <img v-show="showCover" id="anzhiyuCustomPlayer-cover-art-img" :class="{ rotating }" :src="cover" alt="专辑封面" @error="coverError = true" @load="coverError = false">
        <span v-show="showCover" class="disc-center" aria-hidden="true"></span>
      </div>
      <div class="anzhiyuCustomPlayer-song-info-left">
        <h2 id="anzhiyuCustomPlayer-song-title" style="width: 150px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{{ current ? current.name : status }}</h2>
        <p id="anzhiyuCustomPlayer-artist-name" style="width: 150px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{{ current ? current.artist : '' }}</p>
        <p v-if="soundBlocked" class="anzhiyuCustomPlayer-sound-hint">🔇 浏览器拦截了自动发声，点击页面任意处开启声音</p>
      </div>
    </div>
    <div class="anzhiyuCustomPlayer-right-column">
      <div id="anzhiyuCustomPlayer-playlist-container" class="anzhiyuCustomPlayer-playlist-container">
        <div
          v-for="(t, i) in playlist" :key="i"
          class="anzhiyuCustomPlayer-playlist-item"
          :class="{ 'anzhiyuCustomPlayer-playlist-item-active': i === index }"
          :data-index="i"
          @click="select(i, true)"
        >
          <span class="anzhiyuCustomPlayer-playlist-item-number">{{ pad(i + 1) }}</span>
          <span class="anzhiyuCustomPlayer-playlist-item-info" :title="`${t.name || '未知歌曲'} - ${t.artist || '未知艺术家'}`">{{ t.name || '未知歌曲' }} - {{ t.artist || '未知艺术家' }}</span>
        </div>
        <div v-if="!playlist.length" class="anzhiyuCustomPlayer-playlist-item">
          {{ status }}
          <button v-if="failed" class="anzhiyuCustomPlayer-retry" @click="retry">重试</button>
        </div>
      </div>
      <div class="anzhiyuCustomPlayer-controls-area">
        <audio ref="audioEl" id="anzhiyuCustomPlayer-audio-element" controls preload="auto" @play="onPlay" @pause="onPause" @ended="next"></audio>
      </div>
    </div>
  </div>
</template>

<style>
/* 原版 music-index.js 运行时注入的样式块（styles-v3）逐条移植 */
.anzhiyuCustomPlayer-player-container-v3 { display: flex; height: 100%; box-sizing: border-box; padding: 15px; background-color: var(--anzhiyu-card-bg, white); box-shadow: var(--anzhiyu-shadow-border, 0 0 10px rgba(0,0,0,0.1)); border-radius: var(--anzhiyu-border-radius, 12px); color: var(--anzhiyu-fontcolor, black); gap: 20px; position: relative; z-index: 1; }
.anzhiyuCustomPlayer-left-column { flex: 0 0 200px; display: flex; flex-direction: column; align-items: center; justify-content: center; }
/* 唱片位：主题粉渐变底盘 + 白色唱片环 + 主题色描边——无封面时也不是白盘 */
.anzhiyuCustomPlayer-cover-art-wrapper { position: relative; width: 170px; height: 170px; border-radius: 50%; overflow: hidden; box-shadow: 0 5px 15px rgba(0,0,0,0.25); margin-bottom: 15px; background: radial-gradient(circle at 32% 28%, #f9e2e6 0%, #f0cdd3 48%, #e2b3bc 100%); }
.anzhiyuCustomPlayer-cover-art-wrapper::before { content: ""; position: absolute; inset: 6%; border-radius: 50%; background: repeating-radial-gradient(circle at 50% 50%, rgba(255,255,255,.32) 0 2px, rgba(255,255,255,0) 2px 6px); pointer-events: none; }
.anzhiyuCustomPlayer-cover-art-wrapper::after { content: ""; position: absolute; inset: 0; border-radius: 50%; box-shadow: inset 0 0 0 3px rgba(255,255,255,.9), inset 0 0 0 5px var(--anzhiyu-theme, #eabcbd); pointer-events: none; }
.anzhiyuCustomPlayer-cover-art-wrapper .disc-crowd { position: absolute; inset: 0; width: 100%; height: 100%; }
#anzhiyuCustomPlayer-cover-art-img { position: relative; width: 100%; height: 100%; object-fit: cover; display: block; transition: transform 0.3s ease-out; }
#anzhiyuCustomPlayer-cover-art-img.rotating { animation: anzhiyuCustomPlayer-rotate 15s linear infinite; }
/* 封面中心贴：唱片孔视觉，不随封面旋转 */
.anzhiyuCustomPlayer-cover-art-wrapper .disc-center { position: absolute; top: 50%; left: 50%; width: 20%; height: 20%; transform: translate(-50%, -50%); border-radius: 50%; background: var(--anzhiyu-card-bg, #fff); box-shadow: 0 0 0 5px rgba(0,0,0,.10), inset 0 0 0 3px var(--anzhiyu-theme, #eabcbd); z-index: 2; pointer-events: none; }
@keyframes anzhiyuCustomPlayer-rotate { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
.anzhiyuCustomPlayer-song-info-left { text-align: center; width: 100%; padding: 0 5px; }
#anzhiyuCustomPlayer-song-title { font-size: 1.1em; margin: 0 0 4px 0; font-weight: bold; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--anzhiyu-theme, #e0506d); display: block; max-width: 150px; }
#anzhiyuCustomPlayer-artist-name { font-size: 0.85em; margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--anzhiyu-second-fontcolor, gray); display: block; max-width: 150px; }
.anzhiyuCustomPlayer-sound-hint { font-size: 0.72em; margin: 6px 0 0; color: var(--anzhiyu-theme, #e0506d); max-width: 170px; white-space: normal; line-height: 1.5; }
.anzhiyuCustomPlayer-right-column { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.anzhiyuCustomPlayer-playlist-container { height: 200px; overflow-y: auto; border: 1px solid var(--anzhiyu-card-border, #ddd); padding: 5px; border-radius: var(--anzhiyu-border-radius-small, 8px); background-color: var(--anzhiyu-theme-op, rgba(234, 188, 189, .10)); margin-bottom: 10px; }
.anzhiyuCustomPlayer-playlist-item { display: flex; align-items: center; padding: 6px 8px; margin-bottom: 3px; cursor: pointer; border-radius: 6px; transition: background-color 0.2s ease-in-out; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.anzhiyuCustomPlayer-playlist-item:hover { background-color: var(--anzhiyu-gray-a, #eee); }
.anzhiyuCustomPlayer-playlist-item-active { background-color: var(--anzhiyu-theme-op, rgba(255, 102, 102, 0.15)); color: var(--anzhiyu-theme, #ff6666); font-weight: bold; }
.anzhiyuCustomPlayer-playlist-item-number { font-size: 0.8em; color: var(--anzhiyu-third-fontcolor, #999); margin-right: 8px; min-width: 20px; text-align: right; }
.anzhiyuCustomPlayer-playlist-item-active .anzhiyuCustomPlayer-playlist-item-number { color: var(--anzhiyu-theme, #ff6666); }
.anzhiyuCustomPlayer-playlist-item-info { font-size: 0.9em; overflow: hidden; text-overflow: ellipsis; color: var(--anzhiyu-fontcolor, #333); }
.anzhiyuCustomPlayer-playlist-item-active .anzhiyuCustomPlayer-playlist-item-info { color: var(--anzhiyu-theme, #ff6666); }
.anzhiyuCustomPlayer-controls-area { margin-top: auto; }
.anzhiyuCustomPlayer-retry { margin-left: auto; background: var(--anzhiyu-theme, #eabcbd); color: #fff; border: none; border-radius: 12px; padding: 2px 12px; font-size: .8rem; cursor: pointer; }
.anzhiyuCustomPlayer-retry:hover { background: var(--anzhiyu-hover, #ff7242); }
#anzhiyuCustomPlayer-audio-element { width: 100%; border-radius: 8px; display: block; accent-color: var(--anzhiyu-theme, #eabcbd); }
#anzhiyuCustomPlayer-audio-element::-webkit-media-controls-panel { background-color: var(--anzhiyu-theme-op, #fbe4e8); border-radius: 8px; }
#anzhiyuCustomPlayer-audio-element::-webkit-media-controls-play-button { filter: invert(var(--anzhiyu-darkmode-invert-Molar, 0)); }
@media (max-width: 768px) {
  .anzhiyuCustomPlayer-player-container-v3 { flex-direction: column; padding: 10px; gap: 10px; }
  .anzhiyuCustomPlayer-left-column { flex-basis: auto; width: 100%; padding-top: 5px; align-items: center; justify-content: flex-start; }
  .anzhiyuCustomPlayer-cover-art-wrapper { width: 130px; height: 130px; margin-bottom: 10px; }
  .anzhiyuCustomPlayer-right-column { width: 100%; }
  .anzhiyuCustomPlayer-playlist-container { font-size: 0.85em; height: 150px; }
  #anzhiyuCustomPlayer-song-title { font-size: 1em; }
  #anzhiyuCustomPlayer-artist-name { font-size: 0.75em; }
}
</style>
