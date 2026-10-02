<script setup>
// 自定义音乐播放器（参考站 #custom-music-player-placeholder 同构，改造成 Vue3 组件）
// 原版 music-index.js 直连第三方 Meting API；这里走本服务 /api/v1/music/playlist 代理，
// DOM 类名与原版前缀保持一致（anzhiyuCustomPlayer-*），样式块也从原版注入样式逐条移植。
import { ref, computed, onMounted } from 'vue'

const playlist = ref([])
const index = ref(0)
const rotating = ref(false)
const status = ref('正在加载...')
const audioEl = ref(null)

const defaultCover = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'
const current = computed(() => playlist.value[index.value] || null)
const cover = computed(() => {
  const t = current.value
  if (!t || !t.pic) return defaultCover
  return String(t.pic).replace(/param=\d+y\d+/, 'param=256y256')
})

function pad(n) { return String(n).padStart(2, '0') }

function select(i, autoplay = false) {
  if (!playlist.value.length) return
  index.value = i
  const t = playlist.value[i]
  const a = audioEl.value
  if (!a || !t || !t.url) return
  const target = new URL(t.url, window.location.href).href
  if (a.src !== target) a.src = target
  if (autoplay) {
    const p = a.play()
    if (p && p.catch) p.catch(() => {})
  }
}

function next() {
  if (!playlist.value.length) return
  select((index.value + 1) % playlist.value.length, true)
}

function onPlay() { rotating.value = true }
function onPause() { rotating.value = false }

onMounted(async () => {
  const cfg = (window.GLOBAL_CONFIG && window.GLOBAL_CONFIG.musicPlayer) || {}
  const id = /^\d+$/.test(String(cfg.playlistId)) ? String(cfg.playlistId) : '652135520'
  const server = ['netease', 'tencent', 'kugou', 'baidu'].includes(String(cfg.server)) ? String(cfg.server) : 'netease'
  try {
    const r = await fetch(`/api/v1/music/playlist?id=${id}&server=${server}`)
    if (!r.ok) throw new Error('HTTP ' + r.status)
    const data = await r.json()
    if (Array.isArray(data) && data.length) {
      playlist.value = data
      select(0, false)
      status.value = ''
      return
    }
    status.value = '歌单为空'
  } catch (e) {
    status.value = '加载失败'
    playlist.value = []
  }
})
</script>

<template>
  <div class="anzhiyuCustomPlayer-player-container-v3">
    <div class="anzhiyuCustomPlayer-left-column">
      <div id="anzhiyuCustomPlayer-cover-art-wrapper" class="anzhiyuCustomPlayer-cover-art-wrapper">
        <img id="anzhiyuCustomPlayer-cover-art-img" :class="{ rotating }" :src="cover" alt="专辑封面">
      </div>
      <div class="anzhiyuCustomPlayer-song-info-left">
        <h2 id="anzhiyuCustomPlayer-song-title" style="width: 150px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{{ current ? current.name : status }}</h2>
        <p id="anzhiyuCustomPlayer-artist-name" style="width: 150px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{{ current ? current.artist : '' }}</p>
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
        <div v-if="!playlist.length" class="anzhiyuCustomPlayer-playlist-item">{{ status }}</div>
      </div>
      <div class="anzhiyuCustomPlayer-controls-area">
        <audio ref="audioEl" id="anzhiyuCustomPlayer-audio-element" controls @play="onPlay" @pause="onPause" @ended="next"></audio>
      </div>
    </div>
  </div>
</template>

<style>
/* 原版 music-index.js 运行时注入的样式块（styles-v3）逐条移植 */
.anzhiyuCustomPlayer-player-container-v3 { display: flex; height: 100%; box-sizing: border-box; padding: 15px; background-color: var(--anzhiyu-card-bg, white); box-shadow: var(--anzhiyu-shadow-border, 0 0 10px rgba(0,0,0,0.1)); border-radius: var(--anzhiyu-border-radius, 12px); color: var(--anzhiyu-fontcolor, black); gap: 20px; }
.anzhiyuCustomPlayer-left-column { flex: 0 0 200px; display: flex; flex-direction: column; align-items: center; justify-content: center; }
.anzhiyuCustomPlayer-cover-art-wrapper { width: 170px; height: 170px; border-radius: 50%; overflow: hidden; box-shadow: 0 5px 15px rgba(0,0,0,0.25); margin-bottom: 15px; background-color: #e0e0e0; }
#anzhiyuCustomPlayer-cover-art-img { width: 100%; height: 100%; object-fit: cover; display: block; transition: transform 0.3s ease-out; }
#anzhiyuCustomPlayer-cover-art-img.rotating { animation: anzhiyuCustomPlayer-rotate 15s linear infinite; }
@keyframes anzhiyuCustomPlayer-rotate { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
.anzhiyuCustomPlayer-song-info-left { text-align: center; width: 100%; padding: 0 5px; }
#anzhiyuCustomPlayer-song-title { font-size: 1.1em; margin: 0 0 4px 0; font-weight: bold; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--anzhiyu-fontcolor, black); display: block; max-width: 150px; }
#anzhiyuCustomPlayer-artist-name { font-size: 0.85em; margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--anzhiyu-second-fontcolor, gray); display: block; max-width: 150px; }
.anzhiyuCustomPlayer-right-column { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.anzhiyuCustomPlayer-playlist-container { height: 200px; overflow-y: auto; border: 1px solid var(--anzhiyu-gray-c, #ddd); padding: 5px; border-radius: var(--anzhiyu-border-radius-small, 8px); background-color: var(--anzhiyu-background, #f9f9f9); margin-bottom: 10px; }
.anzhiyuCustomPlayer-playlist-item { display: flex; align-items: center; padding: 6px 8px; margin-bottom: 3px; cursor: pointer; border-radius: 6px; transition: background-color 0.2s ease-in-out; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.anzhiyuCustomPlayer-playlist-item:hover { background-color: var(--anzhiyu-gray-a, #eee); }
.anzhiyuCustomPlayer-playlist-item-active { background-color: var(--anzhiyu-theme-op, rgba(255, 102, 102, 0.15)); color: var(--anzhiyu-theme, #ff6666); font-weight: bold; }
.anzhiyuCustomPlayer-playlist-item-number { font-size: 0.8em; color: var(--anzhiyu-third-fontcolor, #999); margin-right: 8px; min-width: 20px; text-align: right; }
.anzhiyuCustomPlayer-playlist-item-active .anzhiyuCustomPlayer-playlist-item-number { color: var(--anzhiyu-theme, #ff6666); }
.anzhiyuCustomPlayer-playlist-item-info { font-size: 0.9em; overflow: hidden; text-overflow: ellipsis; color: var(--anzhiyu-second-fontcolor, #555); }
.anzhiyuCustomPlayer-playlist-item-active .anzhiyuCustomPlayer-playlist-item-info { color: var(--anzhiyu-theme, #ff6666); }
.anzhiyuCustomPlayer-controls-area { margin-top: auto; }
#anzhiyuCustomPlayer-audio-element { width: 100%; border-radius: 8px; display: block; }
#anzhiyuCustomPlayer-audio-element::-webkit-media-controls-panel { background-color: var(--anzhiyu-card-bg, #f0f0f0); border-radius: 8px; }
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
