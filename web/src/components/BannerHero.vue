<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const slides = [
  { name: 'deep-blue', css: 'radial-gradient(ellipse 55% 85% at 12% 8%, rgba(66,90,239,.4), transparent 62%), radial-gradient(ellipse 50% 80% at 88% 12%, rgba(234,188,189,.5), transparent 60%), linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%)' },
  { name: 'sunset-pink', css: 'radial-gradient(ellipse 60% 80% at 20% 20%, rgba(255,114,66,.35), transparent 60%), radial-gradient(ellipse 55% 75% at 80% 10%, rgba(234,188,189,.6), transparent 62%), linear-gradient(150deg, #7a6a75 0%, #6b5560 50%, #4d3f4a 100%)' },
  { name: 'aurora', css: 'radial-gradient(ellipse 55% 70% at 75% 15%, rgba(54,181,98,.28), transparent 60%), radial-gradient(ellipse 60% 85% at 20% 10%, rgba(66,90,239,.42), transparent 62%), linear-gradient(165deg, #4a5b66 0%, #38505c 52%, #2b3d4f 100%)' }
]
const current = ref(0)
const quotes = [
  '把信源换成你的，把精选标准换成你的 KnowHow。',
  '热度按独立来源算——一家媒体发十篇也只算一次。',
  '300 star 的新项目，比静态 30 万 star 的老项目更热。',
  '两个世界同时说一件事，可信度更高。',
  '48 小时窗口，24 小时减半。',
]
const qi = ref(0)
const typed = ref('')
let slideTimer, typeTimer, quoteTimer

const displayQuote = computed(() => quotes[qi.value])

function typeLoop() {
  const text = displayQuote.value
  let i = 0
  typed.value = ''
  clearInterval(typeTimer)
  typeTimer = setInterval(() => {
    i++
    typed.value = text.slice(0, i)
    if (i >= text.length) clearInterval(typeTimer)
  }, 70)
}

function nextSlide() { current.value = (current.value + 1) % slides.length }
function go(i) { current.value = i }
function documentScroll() {
  const el = document.getElementById('article-container')
  if (el) el.scrollIntoView({ behavior: 'smooth' })
}

onMounted(() => {
  typeLoop()
  quoteTimer = setInterval(() => { qi.value = (qi.value + 1) % quotes.length; typeLoop() }, 6000)
  slideTimer = setInterval(nextSlide, 6000)
})
onBeforeUnmount(() => { clearInterval(slideTimer); clearInterval(typeTimer); clearInterval(quoteTimer) })
</script>

<template>
  <!-- 首页大横幅：全宽轮播封面 + 渐变遮罩 + 打字机一言 + 下滑箭头（AnZhiYu 首屏同构） -->
  <header id="page-header" class="home-banner">
    <transition-group name="slide-fade">
      <div v-for="(s, i) in slides" v-show="i === current" :key="s.name" class="slide" :style="{ background: s.css }"></div>
    </transition-group>
    <div class="mask"></div>
    <div id="site-info">
      <h1 class="site-title">GithubHot</h1>
      <div class="tagline">GitHub 开源项目热点 <span class="x">×</span> AI 资讯热点</div>
      <div class="hitokoto"><span class="cursor">|</span>{{ typed }}<span class="cursor blink">|</span></div>
      <form class="searchform" @submit.prevent="router.push({ path: '/search', query: { q: $event.target.q.value } })">
        <input name="q" placeholder="搜索已精选的资讯与事件...">
        <button>搜索</button>
      </form>
      <div class="dots">
        <span v-for="(s, i) in slides" :key="i" :class="{ on: i === current }" @click="go(i)"></span>
      </div>
    </div>
    <a class="scroll-down" href="#article-container" @click.prevent="documentScroll()">
      <span class="arrow">⌄</span>
    </a>
  </header>
</template>

<style scoped>
.home-banner { position: relative; height: 78vh; min-height: 480px; overflow: hidden; }
.slide { position: absolute; inset: 0; transition: opacity 1.2s ease; }
.slide-fade-enter-from, .slide-fade-leave-to { opacity: 0; }
.mask { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(0,0,0,.12), rgba(0,0,0,.28)); }
#site-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; color: #fff; padding: 0 1.5rem; }
.site-title { font-size: 3.6rem; font-weight: 700; letter-spacing: 4px; text-shadow: 0 4px 18px rgba(0,0,0,.3); }
.tagline { margin-top: 14px; font-size: 1.15rem; opacity: .96; text-shadow: 0 2px 8px rgba(0,0,0,.3); }
.tagline .x { color: #ffd7c9; font-weight: 700; padding: 0 8px; }
.hitokoto { margin-top: 12px; font-size: .95rem; opacity: .88; font-style: italic; min-height: 1.6em; }
.cursor { margin: 0 2px; opacity: .9; }
.cursor.blink { animation: blink 1s steps(1) infinite; }
@keyframes blink { 50% { opacity: 0; } }
.searchform { display: flex; max-width: 480px; width: 86%; margin: 24px auto 0; background: rgba(255,255,255,.96); border-radius: 50px; padding: 4px 4px 4px 20px; box-shadow: 0 8px 24px -6px rgba(0,0,0,.3); }
.searchform input { flex: 1; border: none; outline: none; background: transparent; font: inherit; color: #363636; }
.searchform button { border: none; background: var(--anzhiyu-theme); color: #fff; border-radius: 50px; padding: 9px 26px; cursor: pointer; font: inherit; transition: background .3s; }
.searchform button:hover { background: var(--anzhiyu-hover); }
.dots { position: absolute; bottom: 84px; display: flex; gap: 8px; }
.dots span { width: 10px; height: 10px; border-radius: 50%; background: rgba(255,255,255,.45); cursor: pointer; transition: all .3s; }
.dots span.on { background: #fff; transform: scale(1.25); }
.scroll-down { position: absolute; bottom: 22px; left: 50%; transform: translateX(-50%); width: 42px; height: 42px; border-radius: 50%; background: rgba(255,255,255,.2); color: #fff; display: flex; align-items: center; justify-content: center; font-size: 22px; animation: bounce-down 2s infinite; cursor: pointer; }
.scroll-down:hover { background: rgba(255,255,255,.35); }
@media (max-width: 768px) { .home-banner { height: 62vh; } .site-title { font-size: 2.4rem; } }
</style>
