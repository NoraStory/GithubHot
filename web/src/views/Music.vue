<script setup>
// 音乐馆（参考站 /music/ 同构，改造成 Vue3）
// #anMusic-page：随机/刷新/切换 三个控制按钮 + meting-js 挂载点；
// 歌单来源支持 ?id=&server= 参数（白名单校验），背景随封面联动 #an_music_bg。
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const currentId = ref('652135520')
const currentServer = ref('netease')
const switching = ref(false)

const timers = []
const handlers = []

function validId(v) { return /^\d{1,20}$/.test(String(v)) ? String(v) : '652135520' }
function validServer(v) { return ['netease', 'tencent', 'kugou', 'baidu'].includes(String(v)) ? String(v) : 'netease' }

function mountMeting(id, server) {
  const holder = document.getElementById('anMusic-page-meting')
  if (!holder) return
  holder.innerHTML = ''
  const el = document.createElement('meting-js')
  el.setAttribute('id', id)
  el.setAttribute('server', server)
  el.setAttribute('type', 'playlist')
  el.setAttribute('mutex', 'true')
  el.setAttribute('preload', 'auto')
  el.setAttribute('theme', 'var(--anzhiyu-main)')
  el.setAttribute('order', 'list')
  el.setAttribute('list-max-height', 'calc(100vh - 169px)!important')
  holder.appendChild(el)
  currentId.value = id
  currentServer.value = server
  waitAplayer(el)
}

function waitAplayer(el) {
  let tries = 0
  const t = setInterval(() => {
    const ap = el.aplayer
    if (ap) {
      clearInterval(t)
      bindButtons(ap)
      return
    }
    if (++tries > 60) clearInterval(t)
  }, 250)
  timers.push(t)
}

function bindButtons(ap) {
  if (ap.__githubhotBound) return
  ap.__githubhotBound = true
  try { ap.volume(0.8, true) } catch (e) { /* 忽略 */ }
  ap.on('loadeddata', () => {
    const pic = document.querySelector('#anMusic-page .aplayer-pic')
    const bg = document.getElementById('an_music_bg')
    if (pic && bg) bg.style.backgroundImage = pic.style.backgroundImage
  })
  const getSong = document.getElementById('anMusicBtnGetSong')
  const refresh = document.getElementById('anMusicRefreshBtn')
  const switchingBtn = document.getElementById('anMusicSwitching')
  if (getSong) {
    const h = () => {
      const audios = ap.list && ap.list.audios
      if (audios && audios.length) ap.list.switch(Math.floor(Math.random() * audios.length))
    }
    getSong.addEventListener('click', h)
    handlers.push([getSong, h])
  }
  if (refresh) {
    const h = () => mountMeting(currentId.value, currentServer.value)
    refresh.addEventListener('click', h)
    handlers.push([refresh, h])
  }
  if (switchingBtn) {
    const h = () => {
      switching.value = !switching.value
      if (switching.value) mountMeting('652135520', 'netease')
      else mountMeting(validId(route.query.id), validServer(route.query.server))
    }
    switchingBtn.addEventListener('click', h)
    handlers.push([switchingBtn, h])
  }
  // 播放列表按钮 → 弹层遮罩（参考站 menu-mask 同构）
  const t = setInterval(() => {
    const btn = document.querySelector('#anMusic-page .aplayer-icon-menu')
    if (btn) {
      clearInterval(t)
      const h = () => {
        const mask = document.getElementById('menu-mask')
        if (mask) {
          mask.style.display = 'block'
          mask.style.animation = '0.5s ease 0s 1 normal none running to_show'
        }
        const list = document.querySelector('#anMusic-page .aplayer-list')
        if (list) list.style.opacity = '1'
      }
      btn.addEventListener('click', h)
      handlers.push([btn, h])
    }
  }, 400)
  timers.push(t)
}

onMounted(() => {
  const mask = document.getElementById('menu-mask')
  if (mask) {
    const h = () => {
      mask.style.display = 'none'
      const list = document.querySelector('#anMusic-page .aplayer-list')
      if (list) list.classList.remove('aplayer-list-hide')
    }
    mask.addEventListener('click', h)
    handlers.push([mask, h])
  }
  mountMeting(validId(route.query.id), validServer(route.query.server))
})

onBeforeUnmount(() => {
  timers.forEach((t) => clearInterval(t))
  handlers.forEach(([el, h]) => el.removeEventListener('click', h))
  const bg = document.getElementById('an_music_bg')
  if (bg) bg.style.backgroundImage = ''
})
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">小物</router-link></div></div>
      <h1 class="post-title">音乐馆</h1>
      <div id="post-meta"><div class="meta-firstline"><span class="post-meta-label">换个歌单，换个心情 🎧</span></div></div>
    </div>
  </header>
  <main class="layout hide-aside" id="content-inner">
    <div id="page">
      <h1 class="page-title">音乐馆</h1>
      <div id="anMusic-page">
        <div id="anMusicBtnGetSong" title="随机一首，打开异世界的大梦"><i class="anzhiyufont anzhiyu-icon-shuffle"></i></div>
        <div id="anMusicRefreshBtn" title="立即刷新最新歌单"><i class="anzhiyufont anzhiyu-icon-arrows-rotate"></i></div>
        <div id="anMusicSwitching" title="切换歌单"><i class="anzhiyufont anzhiyu-icon-repeat"></i></div>
        <div id="anMusic-page-meting"></div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 16rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #5a6478 0%, #464f68 48%, #37435c 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
#anMusic-page { position: relative; min-height: 320px; }
#anMusicBtnGetSong, #anMusicRefreshBtn, #anMusicSwitching { position: absolute; top: 0; right: 0; width: 38px; height: 38px; border-radius: 50%; background: var(--anzhiyu-card-bg); box-shadow: var(--anzhiyu-shadow-border); display: flex; align-items: center; justify-content: center; cursor: pointer; color: var(--anzhiyu-main); z-index: 3; transition: transform .2s; }
#anMusicBtnGetSong:hover, #anMusicRefreshBtn:hover, #anMusicSwitching:hover { transform: scale(1.12); color: var(--anzhiyu-hover); }
#anMusicRefreshBtn { right: 48px; }
#anMusicBtnGetSong { right: 96px; }
</style>
