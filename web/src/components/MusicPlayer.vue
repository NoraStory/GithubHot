<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps({ playlist: { type: Array, default: () => [] } })

const audio = ref(null)
const index = ref(0)
const playing = ref(false)
const expanded = ref(false)
const showPanel = ref(false)
const progress = ref(0)
const volume = ref(0.6)

const current = computed(() => props.playlist[index.value] || null)

function toggle() {
  if (!audio.value) return
  if (playing.value) { audio.value.pause() } else { audio.value.play() }
}
function next() {
  if (!props.playlist.length) return
  index.value = (index.value + 1) % props.playlist.length
  autoplay()
}
function prev() {
  if (!props.playlist.length) return
  index.value = (index.value - 1 + props.playlist.length) % props.playlist.length
  autoplay()
}
function autoplay() {
  setTimeout(() => { if (audio.value) audio.value.play().catch(() => {}) }, 60)
}
function pick(i) { index.value = i; autoplay(); showPanel.value = false }

function onTime() {
  if (audio.value && audio.value.duration) {
    progress.value = (audio.value.currentTime / audio.value.duration) * 100
  }
}
function seek(e) {
  if (!audio.value || !audio.value.duration) return
  const rect = e.currentTarget.getBoundingClientRect()
  audio.value.currentTime = ((e.clientX - rect.left) / rect.width) * audio.value.duration
}
function onEnd() { next() }
function setVolume(e) { volume.value = e.target.value; if (audio.value) audio.value.volume = volume.value }
</script>

<template>
  <!-- 背景音乐播放器（AnZhiYu APlayer 风格：右下角旋转碟片 + 可展开歌单） -->
  <div class="music-player" :class="{ open: showPanel || expanded }">
    <div class="player-main clickable" @click="showPanel = !showPanel">
      <div class="disc" :class="{ spinning: playing }">
        <span class="disc-icon">{{ playing ? '⏸' : '▶' }}</span>
      </div>
      <div class="info">
        <div class="title">{{ current ? current.name : '背景音乐' }}</div>
        <div class="artist">{{ current ? current.artist : '点击展开歌单' }}</div>
        <div class="progress" @click.stop="seek">
          <div class="bar" :style="{ width: progress + '%' }"></div>
        </div>
      </div>
      <div class="ctrl" @click.stop>
        <button class="mini" @click.stop="prev">⏮</button>
        <button class="mini play" @click.stop="toggle">{{ playing ? '⏸' : '▶' }}</button>
        <button class="mini" @click.stop="next">⏭</button>
      </div>
    </div>
    <transition name="panel">
      <div v-if="showPanel" class="panel">
        <div class="vol">
          🔈
          <input type="range" min="0" max="1" step="0.05" :value="volume" @input="setVolume">
        </div>
        <div class="list">
          <div v-for="(t, i) in playlist" :key="i" class="row" :class="{ on: i === index }" @click="pick(i)">
            <span class="idx">{{ i + 1 }}</span>
            <span class="name">{{ t.name }}</span>
            <span class="artist">{{ t.artist }}</span>
          </div>
        </div>
      </div>
    </transition>
    <audio
      v-if="current"
      ref="audio"
      :src="current.url"
      :volume="volume"
      @timeupdate="onTime"
      @play="playing = true"
      @pause="playing = false"
      @ended="onEnd"
    ></audio>
  </div>
</template>

<style scoped>
.music-player { position: fixed; right: 22px; bottom: 22px; z-index: 95; width: 280px; background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius-big); box-shadow: var(--card-hover-box-shadow); overflow: hidden; transition: width 0.3s; }
.player-main { display: flex; align-items: center; gap: 10px; padding: 10px 12px; }
.disc { width: 44px; height: 44px; border-radius: 50%; flex-shrink: 0; background: conic-gradient(from 0deg, var(--anzhiyu-theme), var(--anzhiyu-hover), var(--anzhiyu-blue), var(--anzhiyu-theme)); display: flex; align-items: center; justify-content: center; color: #fff; font-size: 16px; box-shadow: inset 0 0 0 4px rgba(0,0,0,.08); }
.disc.spinning { animation: spin 4s linear infinite; }
.info { flex: 1; min-width: 0; }
.title { font-weight: 700; font-size: .9rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.artist { color: var(--anzhiyu-gray); font-size: .76rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.progress { height: 4px; background: var(--anzhiyu-theme-op); border-radius: 2px; margin-top: 5px; cursor: pointer; }
.progress .bar { height: 100%; background: var(--anzhiyu-hover); border-radius: 2px; width: 0; }
.ctrl { display: flex; gap: 4px; }
.mini { border: none; background: transparent; cursor: pointer; font-size: 14px; color: var(--anzhiyu-fontcolor); padding: 4px; border-radius: 6px; }
.mini:hover { background: var(--anzhiyu-theme-op); color: var(--anzhiyu-hover); }
.panel { border-top: 1px dashed var(--anzhiyu-card-border); padding: 10px 12px; max-height: 260px; overflow: auto; }
.panel-enter-active, .panel-leave-active { transition: all 0.25s; }
.panel-enter-from, .panel-leave-to { opacity: 0; transform: translateY(8px); }
.vol { display: flex; align-items: center; gap: 8px; font-size: 14px; color: var(--anzhiyu-gray); margin-bottom: 8px; }
.vol input { flex: 1; }
.list .row { display: flex; gap: 8px; padding: 7px 8px; border-radius: 6px; cursor: pointer; font-size: .88rem; }
.list .row:hover { background: var(--anzhiyu-background); }
.list .row.on { background: var(--anzhiyu-theme-op); }
.list .idx { color: var(--anzhiyu-gray); width: 18px; }
.list .name { flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.list .artist { color: var(--anzhiyu-gray); font-size: .78rem; }
</style>
