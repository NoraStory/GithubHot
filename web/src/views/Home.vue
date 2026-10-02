<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'

const router = useRouter()
const view = ref({ github: [], news: [], fusion: [], digests: [], generatedAt: '' })
const loading = ref(true)
const filter = ref('all')
const page = ref(1)
const pageSize = 8

// 期刊数据从分页接口取（全量 50 条供首页展示与筛选）
const digests = computed(() =>
  (digestsAll.value || []).filter((d) => filter.value === 'all' || d.kind === filter.value)
)
const digestsAll = ref([])
const paged = computed(() => digests.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const total = computed(() => digests.value.length)

// 横幅背景（AnZhiYu 由站点配置注入背景图；我们注入深色渐变 + 轮播，主题 :before 自动压暗）
const slides = [
  'radial-gradient(ellipse 55% 85% at 12% 8%, rgba(66,90,239,.55), transparent 62%), radial-gradient(ellipse 50% 80% at 88% 12%, rgba(234,188,189,.45), transparent 60%), linear-gradient(160deg, #3d4a63 0%, #2c3850 55%, #1f2a3d 100%)',
  'radial-gradient(ellipse 60% 80% at 20% 20%, rgba(255,114,66,.45), transparent 60%), radial-gradient(ellipse 55% 75% at 80% 10%, rgba(234,188,189,.4), transparent 62%), linear-gradient(150deg, #4d3f4a 0%, #3a3040 55%, #241f2a 100%)',
  'radial-gradient(ellipse 55% 70% at 75% 15%, rgba(54,181,98,.35), transparent 60%), radial-gradient(ellipse 60% 85% at 20% 10%, rgba(66,90,239,.5), transparent 62%), linear-gradient(165deg, #2b3d4f 0%, #22303e 55%, #161e2a 100%)'
]
const current = ref(0)
let slideTimer

// 随便逛逛：随机跳一个事件/日报
const stories = ref([])
function toRandom() {
  const pool = stories.value.length ? stories.value.map((s) => `/story/${s.storyId}`) : (view.value.digests || []).map((d) => `/digest/${d.date}`)
  if (pool.length) router.push(pool[Math.floor(Math.random() * pool.length)])
}
function scrollDown() {
  const el = document.getElementById('home_top')
  if (el) el.scrollIntoView({ behavior: 'smooth' })
}

// 封面：按标题哈希取主题色渐变（内联 SVG，像素级复刻封面占位）
function coverOf(title) {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  const a = `hsl(${h}, 42%, 62%)`
  const b = `hsl(${(h + 40) % 360}, 48%, 44%)`
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='600' height='336'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs><rect width='600' height='336' fill='url(#g)'/><circle cx='500' cy='70' r='110' fill='rgba(255,255,255,0.12)'/><circle cx='90' cy='290' r='70' fill='rgba(255,255,255,0.09)'/></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}

// 一言打字机
const typed = ref('')
const quotes = [
  '把信源换成你的，把精选标准换成你的 KnowHow。',
  '热度按独立来源算——一家媒体发十篇也只算一次。',
  '300 star 的新项目，比静态 30 万 star 的老项目更热。',
  '两个世界同时说一件事，可信度更高。',
  '48 小时窗口，24 小时减半。'
]
const qi = ref(0)
let typeTimer, quoteTimer
function typeLoop() {
  const text = quotes[qi.value]
  let i = 0
  typed.value = ''
  clearInterval(typeTimer)
  typeTimer = setInterval(() => {
    i++
    typed.value = text.slice(0, i)
    if (i >= text.length) clearInterval(typeTimer)
  }, 70)
}

onMounted(async () => {
  typeLoop()
  quoteTimer = setInterval(() => { qi.value = (qi.value + 1) % quotes.length; typeLoop() }, 6000)
  slideTimer = setInterval(() => { current.value = (current.value + 1) % slides.length }, 6000)
  view.value = await api.get('/api/v1/hot')
  loading.value = false
  const d = await api.get('/api/v1/hot/news')
  stories.value = d.items || []
  const dg = await api.get('/api/v1/digests?pageSize=50')
  digestsAll.value = dg.items || []
})
onBeforeUnmount(() => { clearInterval(typeTimer); clearInterval(quoteTimer); clearInterval(slideTimer) })
</script>

<template>
  <!-- 首页大横幅（full_page：全屏 + 打字机副标题 + 社交图标 + 下滑箭头） -->
  <header class="full_page" id="page-header" :style="{ background: slides[current] }">
    <div id="site-info">
      <h1 id="site-title">GithubHot</h1>
      <div id="site-subtitle"><span id="subtitle">{{ typed }}</span></div>
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

  <!-- home_top：随便逛逛 + 分类三按钮（AnZhiYu bannerGroup 同构） -->
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

    <!-- recent-posts：期刊文章卡（cover + tips + title + meta） -->
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
            <a :href="`/digest/${d.date}`" :title="`${d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报'} ${d.date}`">
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
            <span v-for="p in Math.max(1, Math.ceil(total / pageSize))" :key="p" class="page-item" :class="{ active: p === page }" @click="p !== page && (page = p)">{{ p }}</span>
            <span class="page-item" :class="{ disabled: page >= Math.ceil(total / pageSize) }" @click="page < Math.ceil(total / pageSize) && page++">›</span>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>
