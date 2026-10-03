<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'
import CustomMusicPlayer from '../components/CustomMusicPlayer.vue'
import { repoAvatar, repoCover, digestCover, newsCover } from '../lib/covers'

const router = useRouter()
const view = ref({ github: [], news: [], fusion: [] })
const loading = ref(true)
const filter = ref('all')

const digestsAll = ref([])
const filteredDigests = computed(() =>
  (digestsAll.value || []).filter((d) => filter.value === 'all' || d.kind === filter.value).slice(0, 4)
)
// 三栏榜：GitHub / AI 热点前五
const topGithub = computed(() => (view.value.github || []).slice(0, 5))
const topNews = computed(() => (view.value.news || []).slice(0, 5))

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

// 侧边栏：热点速览 + 标签云 + 问候语轮换
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
  const cfg = await api.get('/api/v1/site/config').catch(() => null)
  if (cfg && cfg.homeVideos) videoList.value = cfg.homeVideos
  // 自定义音乐播放器（music-index 改造版）与 peoplecanvas 画布挂载同步
  bootPeopleCanvas()
  setTimeout(() => document.dispatchEvent(new Event('pjax:complete')), 120)
  // 背景视频加载器（index_media.js）：仅首页有 #home-media-container 时注入，避免其他页面报错
  if (document.getElementById('home-media-container') && !window.__indexMediaLoaded) {
    window.__indexMediaLoaded = true
    const s = document.createElement('script')
    s.src = '/anzhiyu/js/index_media.js'
    s.async = true
    document.body.appendChild(s)
  }
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
        <!-- 统一面板：分类筛选条 + 三栏同容器，一张卡片（原来筛选条与三栏各自独立、视觉零散） -->
        <div class="home-panel">
          <div id="categoryBar">
            <div class="category-bar" id="category-bar">
              <div id="catalog-bar">
                <div id="catalog-list">
                  <div v-for="k in [{ v: 'all', l: '全部' }, { v: 'daily', l: '日报' }, { v: 'weekly', l: '周报' }, { v: 'monthly', l: '月报' }]" :key="k.v" class="catalog-list-item" :id="k.v" :class="{ selected: filter === k.v }">
                    <a href="javascript:void(0)" @click="filter = k.v">{{ k.l }}</a>
                  </div>
                </div>
                <a class="catalog-more" href="javascript:void(0)" @click="$router.push('/categories')">更多</a>
              </div>
            </div>
          </div>

          <div v-if="loading" class="loading">加载中 </div>

          <!-- 三栏：期刊 / GitHub 热点 / AI 热点 -->
          <div class="home-columns">
            <!-- 栏一：期刊（受分类条筛选） -->
            <section class="home-col">
              <div class="home-col-head">
                <span class="home-col-title"><i class="anzhiyufont anzhiyu-icon-book"></i> 期刊报告</span>
                <router-link class="home-col-more" to="/digests">全部期刊 ›</router-link>
              </div>
              <div v-if="!filteredDigests.length" class="empty">暂无期刊</div>
              <router-link v-for="d in filteredDigests" :key="d.date" class="col-item digest-item" :to="`/digest/${d.date}`" :title="d.date">
                <img class="col-item-cover" :src="digestCover(d.date)" alt="cover">
                <div class="col-item-body">
                  <div class="col-item-kind">{{ d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报' }}</div>
                  <div class="col-item-title">{{ d.date }} 双热点报告</div>
                  <div class="col-item-meta">
                    <span>🔥 {{ d.stats ? d.stats.githubItems : 0 }} 项目</span>
                    <span>🤖 {{ d.stats ? d.stats.newsItems : 0 }} 资讯</span>
                    <span class="col-item-date">{{ d.date }}</span>
                  </div>
                </div>
              </router-link>
            </section>

          <!-- 栏二：GitHub 热点榜 -->
          <section class="home-col">
            <div class="home-col-head">
              <span class="home-col-title gh"><i class="anzhiyufont anzhiyu-icon-fire"></i> GitHub 热点</span>
              <router-link class="home-col-more" to="/github">完整榜单 ›</router-link>
            </div>
            <div v-if="!topGithub.length" class="empty">暂无项目</div>
            <a v-for="(p, i) in topGithub" :key="p.fullName" class="col-item rank-item" :href="p.url" target="_blank" rel="noopener" :title="p.fullName">
              <img class="col-avatar" :src="repoAvatar(p.fullName)" :alt="p.fullName" @error="e => e.target.src = repoCover(p.fullName)">
              <span class="rank-num" :class="{ top: i < 3 }">{{ i + 1 }}</span>
              <div class="col-item-body">
                <div class="col-item-title">{{ p.fullName }}</div>
                <div class="col-item-meta">
                  <span class="hot">+{{ p.starsGained }} ★</span>
                  <span class="desc">热度 {{ p.hotness.toFixed(1) }}</span>
                  <span v-if="p.language" class="lang-chip">{{ p.language }}</span>
                </div>
              </div>
            </a>
          </section>

          <!-- 栏三：AI 热点榜 -->
          <section class="home-col">
            <div class="home-col-head">
              <span class="home-col-title ai"><i class="anzhiyufont anzhiyu-icon-shapes"></i> AI 热点</span>
              <router-link class="home-col-more" to="/news">完整榜单 ›</router-link>
            </div>
            <div v-if="!topNews.length" class="empty">暂无资讯</div>
            <router-link v-for="(n, i) in topNews" :key="n.storyId" class="col-item rank-item" :to="`/story/${n.storyId}`" :title="n.titleZh">
              <img class="col-avatar" :src="newsCover(n.titleZh, n.tags)" :alt="n.titleZh">
              <span class="rank-num" :class="{ top: i < 3 }">{{ i + 1 }}</span>
              <div class="col-item-body">
                <div class="col-item-title">{{ n.titleZh }}</div>
                <div class="col-item-meta">
                  <span class="hot">热度 {{ n.hotness.toFixed(1) }}</span>
                  <span class="desc">{{ n.sourceCount }} 个来源</span>
                </div>
              </div>
            </router-link>
          </section>
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
          <div class="item-headline"><i class="anzhiyufont anzhiyu-icon-fire"></i><span>热点速览</span></div>
          <div class="item-content">
            <router-link to="/news" class="headline-right" title="查看更多"><i class="fas fa-angle-right"></i></router-link>
            <div class="aside-list" id="latest-comments">
              <router-link v-for="s in stories.slice(0, 5)" :key="s.storyId" class="aside-list-item" :to="`/story/${s.storyId}`" :title="s.titleZh">
                <span class="chip">{{ s.hotness.toFixed(0) }}</span> {{ s.titleZh }}
              </router-link>
              <div v-if="!stories.length" class="empty">暂无热点事件</div>
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

<style scoped>
/* ===== 统一面板：筛选条 + 三栏同容器 ===== */
.home-panel { background: var(--anzhiyu-maskbg); border: 1px solid var(--anzhiyu-card-border); border-radius: 14px; padding: 4px 16px 14px; box-shadow: var(--card-box-shadow); }
/* 面板内筛选条不再单独成卡：融为面板头部 */
.home-panel #categoryBar,
.home-panel #category-bar { background: transparent !important; border: none !important; box-shadow: none !important; }
.home-panel #categoryBar { padding: 6px 0; border-bottom: 1px dashed var(--anzhiyu-card-border) !important; border-radius: 0; margin-bottom: 4px; }

/* ===== 首页三栏：期刊 / GitHub 热点 / AI 热点 ===== */
/* minmax(0,…)：fr 轨道默认最小宽度是 min-content，长标题会把第三栏撑出容器、
   盖到右侧边栏下面（AI 热点栏被头像卡片遮住） */
.home-columns { width: 100%; display: grid; grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr) minmax(0, 1fr); gap: 0; align-items: stretch; }
/* 榜单行的内容比卡片矮时底部留白——行弹性均分剩余空间，三栏都撑满 */
.home-col { min-width: 0; display: flex; flex-direction: column; padding: 12px 16px 6px; }
.home-col + .home-col { border-left: 1px dashed var(--anzhiyu-card-border); }
.home-col .rank-item { flex: 1 0 auto; }
@media (max-width: 1200px) {
  .home-columns { grid-template-columns: 1fr; }
  .home-col + .home-col { border-left: none; border-top: 1px dashed var(--anzhiyu-card-border); }
}
.home-col-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; padding-bottom: 8px; border-bottom: 1px dashed var(--anzhiyu-card-border); }
.home-col-title { font-weight: 700; font-size: 1rem; color: var(--anzhiyu-fontcolor); }
.home-col-title i { color: var(--anzhiyu-hover); margin-right: 2px; }
.home-col-title.gh i { color: #58a6ff; }
.home-col-title.ai i { color: #bc8cff; }
.home-col-more { color: var(--anzhiyu-gray); font-size: .8rem; }
.home-col-more:hover { color: var(--anzhiyu-hover); }
.col-item { display: flex; gap: 10px; align-items: center; padding: 8px 6px; border-radius: 10px; color: var(--anzhiyu-fontcolor); transition: background .2s; }
.col-item:hover { background: var(--anzhiyu-theme-op); }
.col-item + .col-item { border-top: 1px dashed var(--anzhiyu-card-border); border-top-left-radius: 0; border-top-right-radius: 0; }
.digest-item .col-item-cover { width: 86px; height: 56px; object-fit: cover; border-radius: 8px; flex-shrink: 0; }
.col-item-body { flex: 1; min-width: 0; }
.col-item-kind { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 7px; font-size: .7rem; margin-bottom: 3px; }
.col-item-title { font-weight: 600; font-size: .9rem; line-height: 1.45; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.col-item:hover .col-item-title { color: var(--anzhiyu-hover); }
.col-item-meta { display: flex; gap: 10px; align-items: center; margin-top: 3px; font-size: .76rem; color: var(--anzhiyu-gray); flex-wrap: wrap; }
.col-item-meta .hot { color: var(--anzhiyu-hover); font-weight: 700; }
.col-item-date { margin-left: auto; }
.col-avatar { width: 40px; height: 40px; border-radius: 10px; object-fit: cover; flex-shrink: 0; background: var(--anzhiyu-background); }
.rank-num { width: 24px; height: 24px; border-radius: 7px; background: var(--anzhiyu-background); color: var(--anzhiyu-gray); display: flex; align-items: center; justify-content: center; font-size: .8rem; font-weight: 700; flex-shrink: 0; }
.rank-num.top { background: var(--anzhiyu-theme); color: #fff; }
.rank-item:nth-child(2) .rank-num.top { background: #ff7242; }
.rank-item:nth-child(3) .rank-num.top { background: #fbbc4c; }
.lang-chip { background: var(--anzhiyu-background); border-radius: 6px; padding: 0 7px; font-size: .72rem; }
</style>
