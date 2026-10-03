<script setup>
// 音乐馆（参考站 /music/ 同构，UI 统一改造）：
// #anMusic-page = 歌单信息条 + 随机/刷新/切换控制 + meting-js 播放器卡片
// 支持 ?id=&server= 参数（白名单校验），背景随封面联动 #an_music_bg
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const currentId = ref('652135520')
const currentServer = ref('netease')
const switching = ref(false)
const songCount = ref(0)

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
  el.setAttribute('list-max-height', '560px!important')
  holder.appendChild(el)
  currentId.value = id
  currentServer.value = server
  songCount.value = 0
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
  songCount.value = (ap.list && ap.list.audios && ap.list.audios.length) || 0
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
    handlers.push([getSong, "click", h])
  }
  if (refresh) {
    const h = () => mountMeting(currentId.value, currentServer.value)
    refresh.addEventListener('click', h)
    handlers.push([refresh, "click", h])
  }
  if (switchingBtn) {
    const h = () => {
      switching.value = !switching.value
      if (switching.value) mountMeting('652135520', 'netease')
      else mountMeting(validId(route.query.id), validServer(route.query.server))
    }
    switchingBtn.addEventListener('click', h)
    handlers.push([switchingBtn, "click", h])
  }
  // 键盘控制（参考站 addEventListenerMusic 同构：空格/←→/↑↓）
  const kh = (e) => {
    const t = e.target
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
    if (e.code === 'Space') { e.preventDefault(); ap.toggle() }
    else if (e.keyCode === 39) { e.preventDefault(); ap.skipForward() }
    else if (e.keyCode === 37) { e.preventDefault(); ap.skipBack() }
    else if (e.keyCode === 38) { e.preventDefault(); try { ap.volume(Math.min(1, ap.audio.volume + 0.1), true) } catch (err) { /* 忽略 */ } }
    else if (e.keyCode === 40) { e.preventDefault(); try { ap.volume(Math.max(0, ap.audio.volume - 0.1), true) } catch (err) { /* 忽略 */ } }
  }
  document.addEventListener('keydown', kh)
  handlers.push([document, "keydown", kh])
  // 播放列表弹层遮罩（参考站 menu-mask 同构）
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
      handlers.push([btn, "click", h])
    }
  }, 400)
  timers.push(t)
}

onMounted(() => {
  // 参考站 body[data-type=music] 沉浸样式：隐藏页脚与悬浮播放器，页头特殊化
  document.body.dataset.type = 'music'
  const mask = document.getElementById('menu-mask')
  if (mask) {
    const h = () => {
      mask.style.display = 'none'
      const list = document.querySelector('#anMusic-page .aplayer-list')
      if (list) list.classList.remove('aplayer-list-hide')
    }
    mask.addEventListener('click', h)
    handlers.push([mask, "click", h])
  }
  mountMeting(validId(route.query.id), validServer(route.query.server))
})

onBeforeUnmount(() => {
  document.body.dataset.type = ''
  timers.forEach((t) => clearInterval(t))
  // handlers 每项是 [元素, 事件类型, 回调] 三元组；解构错会把字符串当回调传入
  // removeEventListener 抛 TypeError，导致路由卸载中断——导航永远完不成（页面卡死）。
  handlers.forEach(([el, type, h]) => el.removeEventListener(type, h))
  // 停掉 APlayer：不处理的话音频元素随 DOM 摘除进入游离状态仍继续下载播放，
  // 既白费流量又和首页播放器叠音。注意三个坑（实测踩过）：
  //  1) APlayer 1.10.1 没有 destroy() 方法；
  //  2) 摘掉 src 会触发 audio error 事件，其内置处理器 2 秒后 skipForward() 回切
  //     列表——此时播放器 DOM 已卸载，list.switch 读不到列表节点抛 TypeError；
  //  3) 所以要先清空 list.audios 再摘 src，让 error 处理器走空分支。
  const meting = document.querySelector('#anMusic-page-meting meting-js')
  if (meting && meting.aplayer) {
    const ap = meting.aplayer
    try { ap.pause() } catch (e) { /* 忽略 */ }
    try { ap.list.audios.length = 0 } catch (e) { /* 忽略 */ }
    try {
      ap.audio.removeAttribute('src')
      ap.audio.load()
    } catch (e) { /* 忽略 */ }
  }
  const holder = document.getElementById('anMusic-page-meting')
  if (holder) holder.innerHTML = ''
  const bg = document.getElementById('an_music_bg')
  if (bg) bg.style.backgroundImage = ''
})
</script>

<template>
  <header class="post-bg post-bg--music" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">音乐</router-link></div></div>
      <h1 class="post-title">音乐馆</h1>
      <div id="post-meta"><div class="meta-firstline"><span class="post-meta-label">换个歌单，换个心情 🎧</span></div></div>
    </div>
  </header>
  <main class="layout hide-aside" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div id="anMusic-page">
          <div class="anMusic-info">
            <span class="anMusic-info-name">{{ switching ? '默认歌单' : '当前歌单' }}</span>
            <span class="anMusic-info-meta">#{{ currentId }} · {{ currentServer }}{{ songCount ? ' · ' + songCount + ' 首' : '' }}</span>
            <span class="anMusic-info-tip">空格 播放/暂停 · ←→ 切歌 · ↑↓ 音量</span>
          </div>
          <div class="anMusic-tools">
            <div id="anMusicBtnGetSong" title="随机一首，打开异世界的大梦"><i class="anzhiyufont anzhiyu-icon-shuffle"></i></div>
            <div id="anMusicRefreshBtn" title="立即刷新最新歌单"><i class="anzhiyufont anzhiyu-icon-arrows-rotate"></i></div>
            <div id="anMusicSwitching" title="切换歌单"><i class="anzhiyufont anzhiyu-icon-repeat"></i></div>
          </div>
          <div id="anMusic-page-meting"></div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }

/* 音乐馆主体：信息条 + 工具行 + 播放器卡片（与其他子页统一） */
#anMusic-page { position: relative; min-height: 320px; }
.anMusic-info { display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap; padding: 0 2px 12px; border-bottom: 1px dashed var(--anzhiyu-card-border); margin-bottom: 14px; }
.anMusic-info-name { font-weight: 700; font-size: 1.05rem; color: var(--anzhiyu-fontcolor); }
.anMusic-info-meta { color: var(--anzhiyu-secondary); font-size: .86rem; }
.anMusic-info-tip { margin-left: auto; color: var(--anzhiyu-gray); font-size: .8rem; }
.anMusic-tools { display: flex; gap: 10px; margin-bottom: 14px; }
#anMusicBtnGetSong, #anMusicRefreshBtn, #anMusicSwitching { width: 38px; height: 38px; border-radius: 50%; background: var(--anzhiyu-card-bg); box-shadow: var(--anzhiyu-shadow-border); display: flex; align-items: center; justify-content: center; cursor: pointer; color: var(--anzhiyu-main); transition: transform .2s; }
#anMusicBtnGetSong:hover, #anMusicRefreshBtn:hover, #anMusicSwitching:hover { transform: scale(1.12); color: var(--anzhiyu-hover); }
#anMusic-page-meting { background: var(--anzhiyu-card-bg); border: 1px solid var(--anzhiyu-card-border); border-radius: 12px; padding: 10px 12px; box-shadow: var(--card-box-shadow, 0 3px 8px 6px rgba(7,17,27,.05)); }
#anMusic-page-meting .aplayer { background: transparent; border: none; box-shadow: none; margin: 0; }
@media (max-width: 768px) {
  .post-bg { height: 14rem; }
  .anMusic-info-tip { margin-left: 0; width: 100%; }
}
</style>
