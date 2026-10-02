<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'

const router = useRouter()
const view = ref({ github: [], news: [], fusion: [] })
const loading = ref(true)
const filter = ref('all')
const page = ref(1)
const pageSize = 8

const digestsAll = ref([])
const digests = computed(() =>
  (digestsAll.value || []).filter((d) => filter.value === 'all' || d.kind === filter.value)
)
const paged = computed(() => digests.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const total = computed(() => digests.value.length)

// ===== 横幅背景视频（AnZhiYu #home-media-container 同构：随机选片/竖横屏/视差由 index_media.js 处理）=====
const LANDSCAPE_VIDEOS = [
  'https://pic.lololowe.com/video/x/1.mp4', 'https://pic.lololowe.com/video/x/2.mp4',
  'https://pic.lololowe.com/video/x/3.mp4', 'https://pic.lololowe.com/video/x/4.mp4',
  'https://pic.lololowe.com/video/x/5.mp4', 'https://pic.lololowe.com/video/x/6.mp4'
].join('|')
const videoList = ref(LANDSCAPE_VIDEOS)

// ===== 古诗词（今日诗词 jinrishici，Typed 循环打字进横幅正中心 #subtitle）=====
const typed = ref('')
let poem = '欲穷千里目，更上一层楼。'
let typeTimer, deleteTimer, quoteTimer
const poemFallback = [
  '欲穷千里目，更上一层楼。', '海内存知己，天涯若比邻。',
  '长风破浪会有时，直挂云帆济沧海。', '路漫漫其修远兮，吾将上下而求索。',
  '沉舟侧畔千帆过，病树前头万木春。'
]
function typeLoop() {
  const text = poem
  let i = 0
  typed.value = ''
  clearInterval(typeTimer)
  typeTimer = setInterval(() => {
    i++
    typed.value = text.slice(0, i)
    if (i >= text.length) {
      clearInterval(typeTimer)
      setTimeout(() => deleteBack(text), 2600)
    }
  }, 130)
}
function deleteBack(text) {
  let i = text.length
  deleteTimer = setInterval(() => {
    i--
    typed.value = text.slice(0, i)
    if (i <= 0) {
      clearInterval(deleteTimer)
      qi.value = (qi.value + 1) % poems.length
      poem = poems[qi.value]
      typeLoop()
    }
  }, 45)
}
const poems = [...poemFallback]
const qi = ref(0)

// 从今日诗词 API 拉一首（失败用内置）
function loadPoem() {
  try {
    if (window.jinrishici) {
      window.jinrishici.load((result) => {
        if (result && result.data && result.data.content) {
          poem = result.data.content
          qi.value = 0
          typeLoop()
        }
      })
    }
  } catch { /* 静默 */ }
}

// 随便逛逛
const stories = ref([])
function toRandom() {
  const pool = stories.value.length ? stories.value.map((s) => `/story/${s.storyId}`) : (digestsAll.value || []).map((d) => `/digest/${d.date}`)
  if (pool.length) router.push(pool[Math.floor(Math.random() * pool.length)])
}
function scrollDown() {
  const el = document.getElementById('home_top')
  if (el) el.scrollIntoView({ behavior: 'smooth' })
}

function coverOf(title) {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  const a = `hsl(${h}, 42%, 62%)`
  const b = `hsl(${(h + 40) % 360}, 48%, 44%)`
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='600' height='336'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs><rect width='600' height='336' fill='url(#g)'/><circle cx='500' cy='70' r='110' fill='rgba(255,255,255,0.12)'/></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

onMounted(async () => {
  typeLoop()
  loadPoem()
  view.value = await api.get('/api/v1/hot')
  loading.value = false
  const d = await api.get('/api/v1/hot/news')
  stories.value = d.items || []
  const dg = await api.get('/api/v1/digests?pageSize=50')
  digestsAll.value = dg.items || []
  const cfg = await api.get('/api/v1/site/config').catch(() => null)
  if (cfg && cfg.homeVideos) videoList.value = cfg.homeVideos
})
onBeforeUnmount(() => { clearInterval(typeTimer); clearInterval(deleteTimer); clearInterval(quoteTimer) })
</script>

<template>
  <!-- 首页大横幅（full_page：视频背景黑白老电影 + 古诗词打字机 + 下滑箭头） -->
  <header class="full_page" id="page-header">
    <!-- AnZhiYu 媒体容器：index_media.js 随机选片播放 -->
    <div
      id="home-media-container"
      :data-landscape-video="videoList"
      :data-portrait-video="videoList"
    ></div>
    <div id="site-info">
      <h1 id="site-title">GithubHot</h1>
      <div id="site-subtitle"><span id="subtitle">{{ typed }}<span class="typed-cursor">|</span></span></div>
      <div id="site_social_icons">
        <a class="social-icon faa-parent animated-hover" href="https://github.com/NoraStory/GithubHot" target="_blank" title="Github">
          <i class="anzhiyufont anzhiyu-icon-github"></i>
        </a>
        <a class="social-icon faa-parent animated-hover" href="/feed/digest.xml" target="_blank" title="RSS">
          <i class="anzhiyufont anzhiyu-icon-rss"></i>
        </a>
      </div>
    </div>
    <div id="scroll-down"><i class="anzhiyufont anzhiyu-icon-angle-down scroll-down-effects" @click="scrollDown"></i></div>
  </header>

  <!-- home_top：随便逛逛 + 分类三按钮 -->
  <main id="blog-container">
    <div id="home_top">
      <div id="bannerGroup">
        <div id="random-banner" @click="toRandom">
          <a id="random-hover" href="javascript:void(0)">
            <i class="anzhiyufont anzhiyu-icon-paper-plane"></i>
            <div class="bannerText">随便逛逛<i class="anzhiyufont anzhiyu-icon-arrow-right"></i></div>
          </a>
        </div>
        <div class="categoryGroup">
          <div class="categoryItem" style="box-shadow: var(--anzhiyu-shadow-blue)">
            <router-link class="categoryButton blue" to="/github"><span class="categoryButtonText">GitHub 项目榜</span><i class="anzhiyufont anzhiyu-icon-fire"></i></router-link>
          </div>
          <div class="categoryItem" style="box-shadow: var(--anzhiyu-shadow-red)">
            <router-link class="categoryButton red" to="/news"><span class="categoryButtonText">AI 资讯榜</span><i class="anzhiyufont anzhiyu-icon-shapes"></i></router-link>
          </div>
          <div class="categoryItem" style="box-shadow: var(--anzhiyu-shadow-green)">
            <router-link class="categoryButton green" to="/fusion"><span class="categoryButtonText">融合观察</span><i class="anzhiyufont anzhiyu-icon-dove"></i></router-link>
          </div>
        </div>
      </div>
    </div>

    <div class="layout" id="content-inner">
      <div class="recent-posts" id="recent-posts">
        <div id="categoryBar">
          <div class="category-bar" id="category-bar">
            <div id="catalog-bar">
              <div id="catalog-list">
                <div v-for="k in [{ v: 'all', l: '全部' }, { v: 'daily', l: '日报' }, { v: 'weekly', l: '周报' }, { v: 'monthly', l: '月报' }]" :key="k.v" class="catalog-list-item" :id="k.v">
                  <a href="javascript:void(0)" @click="filter = k.v; page = 1">{{ k.l }}</a>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!paged.length" class="empty">暂无期刊</div>

        <div v-for="d in paged" :key="d.date" class="recent-post-item fade-up" @click="router.push(`/digest/${d.date}`)">
          <div class="post_cover left">
            <a :href="`/digest/${d.date}`" :title="d.date">
              <img class="post_bg" :src="coverOf(d.date)" alt="cover" style="pointer-events: none">
            </a>
          </div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips">
                <div class="article-categories-original">{{ d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报' }}</div>
              </div>
              <a class="article-title" :href="`/digest/${d.date}`" :title="d.date">{{ d.date }} 双热点报告</a>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date">
                <i class="anzhiyufont anzhiyu-icon-calendar-days" style="font-size: 15px"></i>
                <span class="article-meta-label">发表于</span>
                <time>{{ d.date }}</time>
              </span>
              <span class="article-meta tags" v-if="d.stats">
                <a class="article-meta__tags"><span>🔥 {{ d.stats.githubItems }} 项目</span></a>
                <a class="article-meta__tags"><span>🤖 {{ d.stats.newsItems }} 资讯</span></a>
              </span>
            </div>
          </div>
        </div>

        <div id="pagination">
          <div class="pagination">
            <span class="page-item" :class="{ disabled: page === 1 }" @click="page > 1 && page--">‹</span>
            <span v-for="pn in Math.max(1, Math.ceil(total / pageSize))" :key="pn" class="page-item" :class="{ active: pn === page }" @click="pn !== page && (page = pn)">{{ pn }}</span>
            <span class="page-item" :class="{ disabled: page >= Math.ceil(total / pageSize) }" @click="page < Math.ceil(total / pageSize) && page++">›</span>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>
