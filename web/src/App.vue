<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import MusicPlayer from './components/MusicPlayer.vue'
import BackTop from './components/BackTop.vue'
import { getToken, loadSiteConfig } from './lib/api'

const router = useRouter()
const scrolled = ref(false)
const dark = ref(localStorage.getItem('githubhot_theme') === 'dark')
const bannerVisible = ref(true)
const music = ref([])

function applyTheme() {
  document.documentElement.setAttribute('data-theme', dark.value ? 'dark' : 'light')
  localStorage.setItem('githubhot_theme', dark.value ? 'dark' : 'light')
}
function toggleTheme() {
  dark.value = !dark.value
  applyTheme()
}
function onScroll() {
  scrolled.value = window.scrollY > 20
  bannerVisible.value = window.scrollY < window.innerHeight * 0.72
}
onScroll()

let timer
onMounted(async () => {
  applyTheme()
  window.addEventListener('scroll', onScroll, { passive: true })
  const cfg = await loadSiteConfig()
  music.value = cfg.music || []
  // 打字机副标题由 BannerHero 自己处理；这里只做标题栏时光
  timer = setInterval(onScroll, 800)
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  clearInterval(timer)
})

function logout() {
  setTokenRemove()
  router.push('/admin/login')
}
function setTokenRemove() { localStorage.removeItem('githubhot_admin_token') }
</script>

<template>
  <!-- 顶部导航：横幅上透明，滚动后毛玻璃白/暗（AnZhiYu nav-fixed 行为） -->
  <nav id="nav" :class="{ 'nav-fixed': scrolled, transparent: bannerVisible }">
    <a class="site-name" href="/" @click.prevent="router.push('/')">
      <span class="dot"></span>GithubHot
    </a>
    <div class="menus">
      <router-link class="site-page" to="/">首页</router-link>
      <router-link class="site-page" to="/github">GitHub 榜</router-link>
      <router-link class="site-page" to="/news">AI 榜</router-link>
      <router-link class="site-page" to="/fusion">融合</router-link>
      <router-link class="site-page" to="/digests">期刊</router-link>
      <router-link class="site-page" to="/about">关于</router-link>
      <router-link v-if="getToken()" class="site-page" to="/admin/usage">管理</router-link>
      <a class="site-page icon-btn" href="javascript:void(0)" title="搜索" @click="router.push('/search')">🔍</a>
      <a class="site-page icon-btn" href="javascript:void(0)" :title="dark ? '亮色' : '暗色'" @click="toggleTheme">{{ dark ? '🌞' : '🌙' }}</a>
      <a v-if="getToken()" class="site-page icon-btn" href="javascript:void(0)" title="退出管理" @click="logout">🚪</a>
    </div>
  </nav>

  <router-view v-slot="{ Component }">
    <transition name="page" mode="out-in">
      <component :is="Component" :key="$route.fullPath" />
    </transition>
  </router-view>

  <MusicPlayer v-if="music.length" :playlist="music" />
  <BackTop />
</template>

<style>
/* 导航（AnZhiYu：横幅上透明，滚动后固定白底描边） */
#nav { display: flex; align-items: center; justify-content: space-between; padding: 0 1.5rem; height: 60px; position: fixed; top: 0; left: 0; right: 0; z-index: 91; transition: all 0.5s, border 0.3s; }
#nav.transparent { color: #fff; }
#nav.nav-fixed { background: color-mix(in srgb, var(--anzhiyu-card-bg) 86%, transparent); backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px); outline: 1px solid var(--anzhiyu-card-border); box-shadow: 0 4px 12px -3px rgba(102, 102, 102, 0.15); }
#nav .site-name { font-weight: 700; font-size: 1.2rem; display: flex; align-items: center; gap: 8px; }
#nav .dot { width: 10px; height: 10px; border-radius: 50%; background: var(--anzhiyu-theme); box-shadow: 0 0 0 4px var(--anzhiyu-theme-op); }
#nav .menus { display: flex; gap: 4px; align-items: center; }
#nav .site-page { padding: 8px 14px; border-radius: var(--anzhiyu-radius); font-size: 0.95rem; color: inherit; }
#nav.transparent .site-page { color: #fff; text-shadow: 0 1px 4px rgba(0, 0, 0, 0.3); }
#nav .site-page:hover { background: var(--anzhiyu-background); color: var(--anzhiyu-hover); }
#nav.transparent .site-page:hover { background: rgba(255, 255, 255, 0.18); }
#nav .icon-btn { padding: 8px 10px; }
#nav .router-link-active:not(.icon-btn) { background: var(--anzhiyu-theme-op); }

/* 页面切换过渡 */
.page-enter-active, .page-leave-active { transition: opacity 0.25s, transform 0.25s; }
.page-enter-from { opacity: 0; transform: translateY(12px); }
.page-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
