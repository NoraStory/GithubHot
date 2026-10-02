<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'
import CustomMusicPlayer from '../components/CustomMusicPlayer.vue'

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

// 侧边栏：最新留言 + 标签云 + 问候语轮换
const latestComments = ref([])
const tagCloud = computed(() => {
  const counts = {}
  for (const n of stories.value) {
    for (const t of n.tags || []) counts[t] = (counts[t] || 0) + 1
  }
  const max = Math.max(...Object.values(counts), 1)
  return Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, 10)
    .map(([name, count]) => ({ name, count, size: (0.9 + (count / max) * 0.5).toFixed(2) }))
})
function sayhiClick() {
  const el = document.getElementById('author-info__sayhi')
  const skills = (window.GLOBAL_CONFIG && window.GLOBAL_CONFIG.authorStatus && window.GLOBAL_CONFIG.authorStatus.skills) || ['你好呀']
  if (el) el.textContent = skills[Math.floor(Math.random() * skills.length)]
}
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

// 小板报问候（参考站 welcome.js 改造版：本地时间问候 + 欢迎语）
function greeting() {
  const h = new Date().getHours()
  if (h < 5) return '睡个好觉，保证精力充沛'
  if (h < 10) return '一日之计在于晨'
  if (h < 14) return '吃饱了才有力气干活'
  if (h < 18) return '集中精力，攻克难关'
  return '不要太劳累了，早睡更健康'
}
function welcomeHTML() {
  const h = new Date().getHours()
  const timeChange = h >= 5 && h < 11 ? '<span>🌤️ 早上好，一日之计在于晨</span>'
    : h >= 11 && h < 13 ? '<span>☀️ 中午好，记得午休喔~</span>'
    : h >= 13 && h < 17 ? '<span>🕞 下午好，饮茶先啦！</span>'
    : h >= 17 && h < 19 ? '<span>🚶‍♂️ 即将下班，记得按时吃饭~</span>'
    : h >= 19 ? '<span>🌙 晚上好，夜生活嗨起来！</span>'
    : '<span>夜深了，早点休息，少熬夜</span>'
  const lines = ['数据与诗，都在这里相遇', 'GitHub 热点 × AI 资讯，一站式追踪', '老电影开场，热点正在加载', '今天的榜单，由 LLM 为你精选']
  const line = lines[Math.floor(Math.random() * lines.length)]
  return `🙋欢迎来到 <b><span style="color: var(--anzhiyu-main);">GithubHot</span></b> 💖<br>😊${line}🍂<br>🕒<b><span>${new Date().toLocaleDateString('zh-CN')}</span></b><br>${timeChange}<br>`
}

// 参考站 peoplecanvas（gsap 小人动画）：脚本仅在画布挂载后注入一次，此后靠 pjax:success 事件重建
function bootPeopleCanvas() {
  if (window.__people2Loaded) {
    setTimeout(() => document.dispatchEvent(new Event('pjax:success')), 80)
    return
  }
  window.__people2Loaded = true
  const s = document.createElement('script')
  s.src = '/anzhiyu/js/people_2.js'
  s.async = true
  document.body.appendChild(s)
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
  try {
    const ms = await api.get('/api/v1/messages')
    latestComments.value = (ms.items || []).slice(0, 5)
  } catch { /* 静默 */ }
  const cfg = await api.get('/api/v1/site/config').catch(() => null)
  if (cfg && cfg.homeVideos) videoList.value = cfg.homeVideos
  // 自定义音乐播放器（music-index 改造版）与 peoplecanvas 画布挂载同步
  bootPeopleCanvas()
  setTimeout(() => document.dispatchEvent(new Event('pjax:complete')), 120)
  // 小板报欢迎语（参考站 welcome.js 改造版：本地时间问候，不依赖第三方 IP 接口）
  const sayhi = document.getElementById('author-info__sayhi')
  if (sayhi) sayhi.textContent = greeting()
  const w = document.getElementById('welcome-info')
  if (w) w.innerHTML = welcomeHTML()
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

  <!-- home_top：随便逛逛 + 分类三按钮 + 自定义音乐播放器（参考站同构） -->
  <main id="blog-container">
    <div id="home_top">
      <div class="swiper_container_card" style="height: auto; width: 100%">
        <div id="bannerGroup">
          <div id="random-banner" @click="toRandom">
            <canvas id="peoplecanvas"></canvas>
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
        <div id="custom-music-player-placeholder" style="width: 625px; min-height: 340px; margin-left: 0.5rem;">
          <CustomMusicPlayer />
        </div>
      </div>
    </div>

    <div class="layout" id="content-inner">
      <div class="recent-posts" id="recent-posts">
        <div id="categoryBar">
          <div class="category-bar" id="category-bar">
            <div id="catalog-bar">
              <div id="catalog-list">
                <div v-for="k in [{ v: 'all', l: '全部' }, { v: 'daily', l: '日报' }, { v: 'weekly', l: '周报' }, { v: 'monthly', l: '月报' }]" :key="k.v" class="catalog-list-item" :id="k.v" :class="{ selected: filter === k.v }">
                  <a href="javascript:void(0)" @click="filter = k.v; page = 1">{{ k.l }}</a>
                </div>
              </div>
              <a class="catalog-more" href="javascript:void(0)" @click="$router.push('/categories')">更多</a>
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

      <!-- 右侧边栏（参考站 home 同构）：个人信息 / 小板报 / 倒计时 / 最新评论 / 标签云 -->
      <div class="aside-content" id="aside-content">
        <div class="card-widget card-info">
          <div class="card-content">
            <div class="author-info__sayhi" id="author-info__sayhi" @click="sayhiClick"></div>
            <div class="author-info-avatar">
              <img class="avatar-img" src="/img/avatar.webp" alt="avatar" @error="e => e.target.src = '/img/background/GIF/loading3.gif'">
              <div class="author-status"><img class="g-status" src="/img/background/GIF/loading3.gif" alt="status"></div>
            </div>
            <div class="author-info__description">GitHub 开源项目热点 × AI 资讯热点：自动采集、LLM 精选、事件聚簇与融合观察。</div>
            <div class="author-info__bottom-group">
              <a class="author-info__bottom-group-left" href="/">
                <h1 class="author-info__name">GithubHot</h1>
                <div class="author-info__desc">双热点追踪站</div>
              </a>
              <div class="card-info-social-icons is-center">
                <a class="social-icon faa-parent animated-hover" href="https://github.com/NoraStory/GithubHot" target="_blank" title="Github"><i class="anzhiyufont anzhiyu-icon-github"></i></a>
                <a class="social-icon faa-parent animated-hover" href="/feed/digest.xml" target="_blank" title="RSS"><i class="anzhiyufont anzhiyu-icon-rss"></i></a>
              </div>
            </div>
          </div>
        </div>
        <div class="card-widget card-announcement">
          <div class="item-headline"><i class="anzhiyufont anzhiyu-icon-bullhorn anzhiyu-shake"></i><span>小板报</span></div>
          <div class="announcement_content"><div id="welcome-info"></div></div>
        </div>
        <div class="card-widget card-countdown">
          <div class="item-headline"><i class="anzhiyufont anzhiyu-icon-hourglass-half"></i><span>倒计时</span></div>
          <div class="item-content">
            <div class="cd-count-left">
              <span class="cd-text">距离</span>
              <span class="cd-name" id="eventName"></span>
              <span class="cd-time" id="daysUntil"></span>
              <span class="cd-date" id="eventDate"></span>
            </div>
            <div id="countRight" class="cd-count-right"></div>
          </div>
        </div>
        <div class="card-widget card-latest-comments">
          <div class="item-headline"><i class="fas fa-comments"></i><span>最新留言</span></div>
          <div class="item-content">
            <router-link to="/messages" class="headline-right" title="查看更多"><i class="fas fa-angle-right"></i></router-link>
            <div class="aside-list" id="latest-comments">
              <div v-for="m in latestComments" :key="m.id" class="aside-list-item">
                <span class="chip">{{ m.name }}</span> {{ m.content }}
              </div>
              <div v-if="!latestComments.length" class="empty">还没有留言，来抢沙发～</div>
            </div>
          </div>
        </div>
        <div class="sticky_layout">
          <div class="card-widget">
            <div class="card-tags">
              <div class="item-headline"><i class="anzhiyufont anzhiyu-icon-tags"></i><span>标签</span></div>
              <div class="card-tag-cloud">
                <router-link v-for="c in tagCloud" :key="c.name" :to="{ path: '/search', query: { q: c.name } }" :style="{ fontSize: c.size + 'rem' }">{{ c.name }}<sup>{{ c.count }}</sup></router-link>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>
