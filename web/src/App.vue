<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api, setToken, getToken } from './lib/api'
import SiteFooter from './components/SiteFooter.vue'

const router = useRouter()
const route = useRoute()
const scrolled = ref(false)
const dark = ref(localStorage.getItem('githubhot_theme') === 'dark')
const searchMask = ref(false)
const searchQ = ref('')
const searchResults = ref(null)
const menuOpen = ref(false)
const consoleOpen = ref(false)

// 中控台数据
const stories = ref([])
const digests = ref([])
const musicOn = ref(true) // 参考站 #nav-music 常驻，meting-js 懒加载

function applyTheme() {
  document.documentElement.setAttribute('data-theme', dark.value ? 'dark' : 'light')
  localStorage.setItem('githubhot_theme', dark.value ? 'dark' : 'light')
}
function toggleTheme() { dark.value = !dark.value; applyTheme() }
function onScroll() { scrolled.value = window.scrollY > 20; updatePercent(); updateRightside() }
function toggleMenu() { menuOpen.value = !menuOpen.value }
function closeMenu() { menuOpen.value = false }
function toggleConsole() { consoleOpen.value = !consoleOpen.value }

function toRandom() {
  const pool = stories.value.map((s) => `/story/${s.storyId}`)
    .concat(digests.value.map((d) => `/digest/${d.date}`))
  if (pool.length) router.push(pool[Math.floor(Math.random() * pool.length)])
}

function toggleMusic() {
  // 参考站同构：nav-music 由 meting-js 自定义元素挂载 APlayer
  const meting = document.querySelector('#nav-music meting-js')
  const ap = meting && meting.aplayer
  if (ap) ap.toggle()
  musicOn.value = ap ? !ap.audio.paused : !musicOn.value
}

function toggleRightside() {
  // 参考站 main.js rightSideFn["rightside-config"] 同构：.show 展开配置行，.status 保持 300ms 渐隐
  const hide = document.getElementById('rightside-config-hide')
  if (!hide) return
  if (hide.classList.contains('show')) {
    hide.classList.add('status')
    setTimeout(() => hide.classList.remove('status'), 300)
  }
  hide.classList.toggle('show')
}

// 参考站 scrollFn 同构：滚动后右侧工具滑入；页面不满一屏时始终显示
function updateRightside() {
  const rs = document.getElementById('rightside')
  if (!rs) return
  const innerHeight = window.innerHeight + 56
  if (window.scrollY > 5) {
    if (window.getComputedStyle(rs).getPropertyValue('opacity') === '0') {
      rs.style.cssText = 'opacity: 0.8; transform: translateX(-58px)'
    }
  } else if (document.body.scrollHeight <= innerHeight) {
    rs.style.cssText = 'opacity: 1; transform: translateX(-58px)'
  } else {
    rs.style.cssText = ''
  }
}

function translateToggle() {
  if (window.translateFn) window.translateFn.translatePage()
}

// nav-totop 百分比（参考站 #percent 同构）
function updatePercent() {
  const el = document.getElementById('percent')
  if (!el) return
  const total = document.documentElement.scrollHeight - document.documentElement.clientHeight
  el.textContent = total > 0 ? Math.min(100, Math.round((window.scrollY / total) * 100)) + '' : '0'
}

async function doSearch() {
  if (!searchQ.value.trim()) return
  const d = await api.get(`/api/v1/search?q=${encodeURIComponent(searchQ.value)}`)
  searchResults.value = d.results || []
}

// 中控台标签云
const tagCloud = computed(() => {
  const counts = {}
  for (const n of stories.value) {
    for (const t of n.tags || []) counts[t] = (counts[t] || 0) + 1
    for (const b of n.badges || []) if (b === 'GitHub关联') counts['GitHub'] = (counts[b] || 0) + 1
  }
  const max = Math.max(...Object.values(counts), 1)
  return Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, 8)
    .map(([name, count]) => ({ name, count, size: (0.85 + (count / max) * 0.7).toFixed(2) }))
})

// 中控台归档（按月）
const months = computed(() => {
  const counts = {}
  for (const d of digests.value) {
    const ym = d.date.slice(0, 7)
    counts[ym] = (counts[ym] || 0) + 1
  }
  return Object.entries(counts).sort((a, b) => (a[0] < b[0] ? 1 : -1)).slice(0, 4)
})

const monthLabel = (ym) => {
  const [y, m] = ym.split('-')
  return ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十', '十一', '十二'][parseInt(m, 10) - 1] + '月 ' + y
}

onMounted(async () => {
  applyTheme()
  window.addEventListener('scroll', onScroll, { passive: true })
  updateRightside()
  try {
    const [sn, dg] = await Promise.all([api.get('/api/v1/hot/news'), api.get('/api/v1/digests?pageSize=50')])
    stories.value = sn.items || []
    digests.value = dg.items || []
  } catch { /* 静默 */ }
})
onBeforeUnmount(() => window.removeEventListener('scroll', onScroll))

router.afterEach(() => { menuOpen.value = false; searchMask.value = false; consoleOpen.value = false })
</script>

<template>
  <!-- AnZhiYu 星空背景（火箭/地球/月球/宇航员 + 闪烁星星，主题 CSS 全套动画） -->
  <div id="web_bg">
    <div class="bg_stars">
      <div class="bg_objects">
        <img class="bg_object_rocket" src="/anzhiyu/img/bg/rocket.svg" width="40px" alt="web_bg">
        <div class="bg_earth-moon">
          <img class="bg_object_earth" src="/anzhiyu/img/bg/earth.svg" width="100px" alt="web_bg">
          <img class="bg_object_moon" src="/anzhiyu/img/bg/moon.svg" width="80px" alt="web_bg">
        </div>
        <div class="bg_box_astronaut"><img class="bg_object_astronaut" src="/anzhiyu/img/bg/astronaut.svg" width="140px" alt="web_bg"></div>
      </div>
      <div class="bg_glowing_stars">
        <div class="bg_star"></div>
        <div class="bg_star"></div>
        <div class="bg_star"></div>
        <div class="bg_star"></div>
        <div class="bg_star"></div>
      </div>
    </div>
  </div>
  <!-- 音乐馆封面背景层（Music 页面加载后随歌曲封面联动） -->
  <div id="an_music_bg"></div>

  <!-- AnZhiYu #nav：桌面端 悬停下拉；窄屏 #toggle-menu 汉堡 → #sidebar-menus 抽屉 -->
  <nav id="nav" :class="{ 'nav-fixed': scrolled }">
    <div id="nav-group">
      <span id="blog_name">
        <a id="site-name" href="/" @click.prevent="router.push('/')">
          <span class="site-name-text">GithubHot</span>
        </a>
        <div class="back-home-button">
          <i class="anzhiyufont anzhiyu-icon-grip-vertical"></i>
          <div class="back-menu-list-groups">
            <div class="back-menu-list-group">
              <div class="back-menu-list-title">热点</div>
              <div class="back-menu-list">
                <a class="back-menu-item" href="/github"><span class="back-menu-item-text">GitHub 项目榜</span></a>
                <a class="back-menu-item" href="/news"><span class="back-menu-item-text">AI 资讯榜</span></a>
                <a class="back-menu-item" href="/fusion"><span class="back-menu-item-text">融合观察</span></a>
                <a class="back-menu-item" href="javascript:void(0)" @click="toRandom"><span class="back-menu-item-text">随便逛逛</span></a>
              </div>
            </div>
            <div class="back-menu-list-group">
              <div class="back-menu-list-title">期刊</div>
              <div class="back-menu-list">
                <a class="back-menu-item" href="/digest/latest"><span class="back-menu-item-text">最新日报</span></a>
                <a class="back-menu-item" href="/digests"><span class="back-menu-item-text">全部期刊</span></a>
                <a class="back-menu-item" href="/archives"><span class="back-menu-item-text">归档</span></a>
                <a class="back-menu-item" href="/feed/digest.xml" target="_blank"><span class="back-menu-item-text">RSS 订阅</span></a>
              </div>
            </div>
            <div class="back-menu-list-group">
              <div class="back-menu-list-title">发现</div>
              <div class="back-menu-list">
                <a class="back-menu-item" href="/categories"><span class="back-menu-item-text">分类</span></a>
                <a class="back-menu-item" href="/tags"><span class="back-menu-item-text">标签</span></a>
                <a class="back-menu-item" href="/charts"><span class="back-menu-item-text">统计</span></a>
                <a class="back-menu-item" href="/link"><span class="back-menu-item-text">资源</span></a>
              </div>
            </div>
            <div class="back-menu-list-group">
              <div class="back-menu-list-title">站点</div>
              <div class="back-menu-list">
                <a class="back-menu-item" href="/music"><span class="back-menu-item-text">音乐馆</span></a>
                <a class="back-menu-item" href="/about"><span class="back-menu-item-text">关于本站</span></a>
                <a class="back-menu-item" href="https://github.com/NoraStory/GithubHot" target="_blank"><span class="back-menu-item-text">源码仓库</span></a>
                <a class="back-menu-item" v-if="getToken()" href="/admin/usage"><span class="back-menu-item-text">管理端</span></a>
              </div>
            </div>
          </div>
        </div>
      </span>
      <div id="menus_items" class="menus_items">
        <div class="menus_item">
          <a class="site-page" href="javascript:void(0);"><span> 热点</span></a>
          <ul class="menus_item_child">
            <li><router-link class="site-page child faa-parent animated-hover" to="/github"><i class="anzhiyufont anzhiyu-icon-fire faa-tada" style="font-size: 0.9em;"></i><span> GitHub 项目榜</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/news"><i class="anzhiyufont anzhiyu-icon-shapes faa-tada" style="font-size: 0.9em;"></i><span> AI 资讯榜</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/fusion"><i class="anzhiyufont anzhiyu-icon-dove faa-tada" style="font-size: 0.9em;"></i><span> 融合观察</span></router-link></li>
            <li><a class="site-page child faa-parent animated-hover" href="javascript:void(0)" @click="toRandom"><i class="anzhiyufont anzhiyu-icon-dice faa-tada" style="font-size: 0.9em;"></i><span> 随便逛逛</span></a></li>
          </ul>
        </div>
        <div class="menus_item">
          <a class="site-page" href="javascript:void(0);"><span> 期刊</span></a>
          <ul class="menus_item_child">
            <li><router-link class="site-page child faa-parent animated-hover" to="/digest/latest"><i class="anzhiyufont anzhiyu-icon-fire faa-tada" style="font-size: 0.9em;"></i><span> 最新日报</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/digests"><i class="anzhiyufont anzhiyu-icon-box-archive faa-tada" style="font-size: 0.9em;"></i><span> 全部期刊</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/archives"><i class="anzhiyufont anzhiyu-icon-clock-rotate-left faa-tada" style="font-size: 0.9em;"></i><span> 归档</span></router-link></li>
            <li><a class="site-page child faa-parent animated-hover" href="/feed/digest.xml" target="_blank"><i class="anzhiyufont anzhiyu-icon-rss faa-tada" style="font-size: 0.9em;"></i><span> RSS 订阅</span></a></li>
          </ul>
        </div>
        <div class="menus_item">
          <a class="site-page" href="javascript:void(0);"><span> 发现</span></a>
          <ul class="menus_item_child">
            <li><router-link class="site-page child faa-parent animated-hover" to="/categories"><i class="anzhiyufont anzhiyu-icon-shapes faa-tada" style="font-size: 0.9em;"></i><span> 分类</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/tags"><i class="anzhiyufont anzhiyu-icon-tags faa-tada" style="font-size: 0.9em;"></i><span> 标签</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/charts"><i class="fa-solid fa-chart-line faa-tada" style="font-size: 0.9em;"></i><span> 统计</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/link"><i class="anzhiyufont anzhiyu-icon-link faa-tada" style="font-size: 0.9em;"></i><span> 资源</span></router-link></li>
          </ul>
        </div>
        <div class="menus_item">
          <a class="site-page" href="javascript:void(0);"><span> 我的</span></a>
          <ul class="menus_item_child">
            <li><router-link class="site-page child faa-parent animated-hover" to="/tools"><i class="anzhiyufont anzhiyu-icon-tools faa-tada" style="font-size: 0.9em;"></i><span> 工具库</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/album"><i class="anzhiyufont anzhiyu-icon-images faa-tada" style="font-size: 0.9em;"></i><span> 相册集</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/music"><i class="anzhiyufont anzhiyu-icon-music faa-tada" style="font-size: 0.9em;"></i><span> 音乐馆</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/messages"><i class="anzhiyufont anzhiyu-icon-comments faa-tada" style="font-size: 0.9em;"></i><span> 留言板</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/air-conditioner"><i class="anzhiyufont anzhiyu-icon-fan faa-tada" style="font-size: 0.9em;"></i><span> 小空调</span></router-link></li>
          </ul>
        </div>
        <div class="menus_item">
          <a class="site-page" href="javascript:void(0);"><span> 关于</span></a>
          <ul class="menus_item_child">
            <li><a class="site-page child faa-parent animated-hover" href="javascript:void(0)" @click="toRandom"><i class="anzhiyufont anzhiyu-icon-dice faa-tada" style="font-size: 0.9em;"></i><span> 随便逛逛</span></a></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/privacy"><i class="anzhiyufont anzhiyu-icon-file-contract faa-tada" style="font-size: 0.9em;"></i><span> 隐私协议</span></router-link></li>
            <li><router-link class="site-page child faa-parent animated-hover" to="/about"><i class="anzhiyufont anzhiyu-icon-github faa-tada" style="font-size: 0.9em;"></i><span> 关于本站</span></router-link></li>
            <li v-if="getToken()"><router-link class="site-page child faa-parent animated-hover" to="/admin/usage"><i class="anzhiyufont anzhiyu-icon-gear faa-tada" style="font-size: 0.9em;"></i><span> 管理端</span></router-link></li>
          </ul>
        </div>
      </div>
    </div>
    <div id="nav-right">
      <!-- AnZhiYu 招牌动画深色切换（云朵/星星/月亮，样式全部来自主题 CSS） -->
      <div id="nav-naoDark" @click="dark = !dark; applyTheme()" :title="dark ? '切换浅色' : '切换深色'">
        <div class="container">
          <div class="components">
            <div class="main-button">
              <div class="moon"></div>
              <div class="moon"></div>
              <div class="moon"></div>
            </div>
            <div class="daytime-backgrond"></div>
            <div class="daytime-backgrond"></div>
            <div class="daytime-backgrond"></div>
            <div class="cloud">
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
            </div>
            <div class="cloud-light">
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
              <div class="cloud-son"></div>
            </div>
            <div class="stars">
              <div class="star big"><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div></div>
              <div class="star big"><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div></div>
              <div class="star medium"><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div></div>
              <div class="star medium"><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div></div>
              <div class="star small"><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div></div>
              <div class="star small"><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div><div class="star-son"></div></div>
            </div>
          </div>
        </div>
      </div>
      <div class="nav-button" id="randomPost_button">
        <a class="site-page" href="javascript:void(0);" title="随机前往一个事件" @click="toRandom">
          <i class="anzhiyufont anzhiyu-icon-dice"></i>
        </a>
      </div>
      <div class="nav-button" id="search-button" @click="searchMask = true">
        <a class="site-page social-icon search" href="javascript:void(0);" title="搜索🔍">
          <i class="anzhiyufont anzhiyu-icon-magnifying-glass"></i>
          <span> 搜索</span>
        </a>
      </div>
      <!-- 中控台开关（AnZhiYu center-console 同构） -->
      <div class="nav-button" id="center-console-button" title="中控台" @click="consoleOpen = !consoleOpen">
        <a class="site-page social-icon"><i class="anzhiyufont anzhiyu-icon-grip-vertical"></i></a>
      </div>
      <div class="nav-button" id="toggle-menu" title="菜单" @click="toggleMenu">
        <a class="site-page social-icon"><i class="anzhiyufont anzhiyu-icon-bars"></i></a>
      </div>
      <!-- AnZhiYu 返回顶部（带滚动百分比） -->
      <div class="nav-button" id="nav-totop">
        <a class="totopbtn" href="javascript:void(0);" @click="window.anzhiyu && window.anzhiyu.scrollToDest(0, 500)">
          <i class="anzhiyufont anzhiyu-icon-arrow-up"></i><span id="percent">0</span>
        </a>
      </div>
    </div>
  </nav>

  <!-- AnZhiYu 中控台面板（#console.show 主题机制：遮罩 + 卡片组 + 底部工具条） -->
  <div id="console" :class="{ show: consoleOpen }">
    <div class="console-card-group">
      <div class="console-card-group-left">
        <div class="console-card" id="card-newest-stories">
          <div class="card-content">
            <div class="author-content-item-tips">热点</div>
            <span class="author-content-item-title"> 最新事件</span>
          </div>
          <div class="aside-list">
            <a v-for="s in stories.slice(0, 5)" :key="s.storyId" class="aside-list-item" :href="`/story/${s.storyId}`">
              <span class="chip">{{ s.hotness.toFixed(0) }}</span> {{ s.titleZh }}
            </a>
            <div v-if="!stories.length" class="empty">暂无事件</div>
          </div>
        </div>
      </div>
      <div class="console-card-group-right">
        <div class="console-card tags">
          <div class="card-content">
            <div class="author-content-item-tips">兴趣点</div>
            <span class="author-content-item-title">寻找你感兴趣的领域</span>
            <div class="card-tag-cloud">
              <router-link v-for="c in tagCloud" :key="c.name" :to="{ path: '/search', query: { q: c.name } }" :style="{ fontSize: c.size + 'rem' }">
                {{ c.name }}<sup>{{ c.count }}</sup>
              </router-link>
              <div v-if="!tagCloud.length" class="empty">暂无标签</div>
            </div>
          </div>
        </div>
        <hr>
        <div class="console-card history">
          <div class="item-headline">
            <i class="anzhiyufont anzhiyu-icon-box-archive"></i>
            <span>期刊</span>
            <router-link class="card-more-btn" to="/archives" title="查看更多"><i class="anzhiyufont anzhiyu-icon-angle-right"></i></router-link>
          </div>
          <ul class="card-archive-list">
            <li v-for="m in months" :key="m[0]" class="card-archive-list-item">
              <router-link class="card-archive-list-link" to="/digests">
                <span class="card-archive-list-date">{{ monthLabel(m[0]) }}</span>
                <div class="card-archive-list-count-group">
                  <span class="card-archive-list-count">{{ m[1] }}</span>
                  <span>篇</span>
                </div>
              </router-link>
            </li>
          </ul>
        </div>
      </div>
    </div>
    <div class="button-group">
      <div class="console-btn-item">
        <a class="darkmode_switchbutton" title="显示模式切换" href="javascript:void(0);" @click="dark = !dark; applyTheme()">
          <i class="anzhiyufont anzhiyu-icon-moon"></i>
        </a>
      </div>
      <div class="console-btn-item" id="consoleHideAside" title="边栏显示控制" @click="window.anzhiyu && window.anzhiyu.hideAsideBtn()">
        <a class="asideSwitch" href="javascript:void(0);"><i class="anzhiyufont anzhiyu-icon-arrows-left-right"></i></a>
      </div>
      <div class="console-btn-item" id="consoleCommentBarrage" title="热评开关" @click="window.anzhiyu && window.anzhiyu.switchCommentBarrage()">
        <a class="commentBarrage" href="javascript:void(0);"><i class="anzhiyufont anzhiyu-icon-message"></i></a>
      </div>
      <div class="console-btn-item" id="consoleMusic" title="音乐开关" @click="toggleMusic">
        <a class="music-switch" href="javascript:void(0);"><i class="anzhiyufont anzhiyu-icon-music"></i></a>
      </div>
      <div class="console-btn-item" id="consoleKeyboard" title="快捷键开关" @click="window.anzhiyu && window.anzhiyu.keyboardToggle()">
        <a class="keyboard-switch" href="javascript:void(0);"><i class="anzhiyufont anzhiyu-icon-keyboard"></i></a>
      </div>
      <div class="console-btn-item" id="consoleRandomPost" title="随机逛逛" @click="toRandom">
        <a href="javascript:void(0);"><i class="anzhiyufont anzhiyu-icon-dice"></i></a>
      </div>
      <div class="console-btn-item" id="consoleAdmin" title="管理端">
        <router-link to="/admin/usage"><i class="anzhiyufont anzhiyu-icon-gear"></i></router-link>
      </div>
      <div id="console-naoDark">
        <div class="container">
          <div class="components">
            <div class="main-button">
              <div class="moon"></div>
              <div class="moon"></div>
              <div class="moon"></div>
            </div>
            <div class="cloud"></div>
            <div class="cloud-light"></div>
          </div>
        </div>
      </div>
    </div>
    <div class="console-mask" @click="consoleOpen = false"></div>
  </div>

  <!-- 背景音乐（AnZhiYu #nav-music 同构：hoverTips 标签 + meting-js 自定义元素挂载 APlayer） -->
  <div id="nav-music">
    <a id="nav-music-hoverTips" href="javascript:void(0);" accesskey="m" @click="toggleMusic">播放音乐</a>
    <div id="console-music-bg"></div>
    <meting-js id="652135520" server="netease" type="playlist" mutex="true" preload="none" theme="var(--anzhiyu-main)" data-lrctype="0" order="random" volume="0.5"></meting-js>
  </div>

  <!-- AnZhiYu 右侧工具（简繁/昼夜/边栏/设置/回到顶部） -->
  <div id="rightside">
    <div id="rightside-config-hide">
      <button id="translateLink" type="button" title="简繁转换" @click="translateToggle">繁</button>
      <button id="darkmode" type="button" title="浅色和深色模式转换" @click="dark = !dark; applyTheme()"><i class="anzhiyufont anzhiyu-icon-circle-half-stroke"></i></button>
      <button id="hide-aside-btn" type="button" title="单栏和双栏切换" @click="window.anzhiyu && window.anzhiyu.hideAsideBtn()"><i class="anzhiyufont anzhiyu-icon-arrows-left-right"></i></button>
    </div>
    <div id="rightside-config-show">
      <button id="rightside-config" type="button" title="设置" @click="toggleRightside"><i class="anzhiyufont anzhiyu-icon-gear"></i></button>
      <a id="switch-commentBarrage" href="javascript:void(0);" title="开关弹幕" @click="window.anzhiyu && window.anzhiyu.switchCommentBarrage()"><i class="anzhiyufont anzhiyu-icon-danmu"></i></a>
      <button id="go-up" type="button" title="回到顶部" @click="window.anzhiyu && window.anzhiyu.scrollToDest(0, 500)"><i class="anzhiyufont anzhiyu-icon-arrow-up"></i></button>
    </div>
  </div>

  <!-- 窄屏抽屉菜单（AnZhiYu #sidebar-menus 同构，含 menu-mask 与站点数据行） -->
  <div id="sidebar" v-if="menuOpen" @click.self="closeMenu">
    <div id="menu-mask"></div>
    <div class="sidebar-menus" id="sidebar-menus">
      <div class="sidebar-author">
        <div class="author-name">🔥 GithubHot</div>
        <div class="author-desc">双热点追踪站</div>
      </div>
      <div class="sidebar-site-data site-data is-center">
        <router-link to="/archives" title="archive"><div class="headline">期刊</div><div class="length-num">{{ digests.length }}</div></router-link>
        <router-link to="/tags" title="tag"><div class="headline">标签</div><div class="length-num">{{ tagCloud.length }}</div></router-link>
        <router-link to="/categories" title="category"><div class="headline">分类</div><div class="length-num">3</div></router-link>
      </div>
      <div class="menus_groups">
        <router-link class="site-page child" to="/"><span> 首页</span></router-link>
        <div class="group-title">热点</div>
        <router-link class="site-page child" to="/github"><span> GitHub 项目榜</span></router-link>
        <router-link class="site-page child" to="/news"><span> AI 资讯榜</span></router-link>
        <router-link class="site-page child" to="/fusion"><span> 融合观察</span></router-link>
        <div class="group-title">期刊</div>
        <router-link class="site-page child" to="/digest/latest"><span> 最新日报</span></router-link>
        <router-link class="site-page child" to="/digests"><span> 全部期刊</span></router-link>
        <router-link class="site-page child" to="/archives"><span> 归档</span></router-link>
        <a class="site-page child" href="/feed/digest.xml" target="_blank"><span> RSS 订阅</span></a>
        <div class="group-title">发现</div>
        <router-link class="site-page child" to="/categories"><span> 分类</span></router-link>
        <router-link class="site-page child" to="/tags"><span> 标签</span></router-link>
        <router-link class="site-page child" to="/charts"><span> 统计</span></router-link>
        <router-link class="site-page child" to="/link"><span> 资源</span></router-link>
        <router-link class="site-page child" to="/music"><span> 音乐馆</span></router-link>
        <router-link class="site-page child" to="/about"><span> 关于</span></router-link>
        <router-link v-if="getToken()" class="site-page child" to="/admin/usage"><span> 管理端</span></router-link>
      </div>
    </div>
  </div>

  <!-- 站内搜索遮罩（AnZhiYu local search 同构） -->
  <div id="search-mask" v-if="searchMask" @click.self="searchMask = false">
    <div class="search-dialog">
      <div class="search-dialog-title">🔍 站内搜索 <span class="close" @click="searchMask = false">✕</span></div>
      <div class="search-dialog-input">
        <input v-model="searchQ" placeholder="输入关键词搜索已精选的资讯与事件..." @keyup.enter="doSearch">
        <button @click="doSearch">搜索</button>
      </div>
      <div class="search-dialog-results">
        <div v-if="searchResults === null" class="empty">输入关键词后回车</div>
        <div v-else-if="!searchResults.length" class="empty">没有匹配结果</div>
        <a v-for="r in searchResults" :key="r.url" class="result-item" :href="r.url" target="_blank" rel="noopener">
          <span class="chip">{{ r.kind === 'story' ? '事件' : '资讯' }}</span> {{ r.titleZh }}
        </a>
      </div>
    </div>
  </div>

  <router-view v-slot="{ Component }">
    <transition name="page" mode="out-in">
      <component :is="Component" :key="$route.fullPath" />
    </transition>
  </router-view>

  <SiteFooter v-if="!$route.path.startsWith('/admin')" />
</template>

<style>
/* 主题外的少量接线样式（其余全部来自 /anzhiyu/css/index.css） */
#nav #nav-group { display: flex; align-items: center; gap: 14px; flex: 1; min-width: 0; }
#nav #site-name .site-name-text { font-weight: 700; }
#nav #nav-right { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
#nav .nav-button { cursor: pointer; }
#nav .nav-button .site-page { padding: 8px 10px; border-radius: var(--anzhiyu-radius); }
#nav #search-button .site-page span { font-size: 0.88rem; }

/* 中控台：v-if 显示时挂 .show 走主题过渡；遮罩为 #console 子元素（主题 CSS 选择器要求） */
#console .aside-list .aside-list-item { display: block; padding: 6px 4px; font-size: .9rem; color: var(--anzhiyu-fontcolor); }
#console .aside-list .aside-list-item:hover { color: var(--anzhiyu-hover); }
#console .chip { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 7px; font-size: .74rem; margin-right: 6px; }
#console .card-tag-cloud a { margin: 4px 8px; color: var(--anzhiyu-fontcolor); }
#console .card-tag-cloud a:hover { color: var(--anzhiyu-hover); }
#console .empty { color: var(--anzhiyu-gray); font-size: .85rem; padding: 8px 0; }
#console .button-group .console-btn-item a { cursor: pointer; }

/* 背景音乐悬浮挂件（主题 CSS 自带 #nav-music/#nav-music-hoverTips/.aplayer 全套样式） */

/* 窄屏：隐藏平铺菜单，仅汉堡按钮（AnZhiYu 断点行为）；中控台仅桌面 */
#toggle-menu { display: none; }
@media (max-width: 900px) {
  #nav .menus_items { display: none; }
  #nav .back-home-button { display: none; }
  #nav-naoDark { display: none; }
  #toggle-menu { display: block; }
  #center-console-button { display: none; }
}

/* 抽屉菜单 */
#sidebar { position: fixed; inset: 0; z-index: 1001; background: rgba(0, 0, 0, 0.4); }
.sidebar-menus { position: absolute; left: 0; top: 0; bottom: 0; width: min(300px, 80vw); background: var(--anzhiyu-card-bg); padding: 20px 18px; overflow: auto; animation: slide-in-left 0.3s ease; }
@keyframes slide-in-left { from { transform: translateX(-40px); opacity: 0; } to { transform: none; opacity: 1; } }
.sidebar-author { padding: 6px 8px 16px; border-bottom: 1px dashed var(--anzhiyu-card-border); margin-bottom: 10px; }
.author-name { font-weight: 700; font-size: 1.1rem; }
.author-desc { color: var(--anzhiyu-gray); font-size: .82rem; }
.group-title { color: var(--anzhiyu-gray); font-size: .78rem; padding: 12px 8px 4px; }
.menus_groups .site-page.child { display: block; padding: 9px 12px; border-radius: var(--anzhiyu-radius); color: var(--anzhiyu-fontcolor); font-size: .95rem; }
.menus_groups .site-page.child:hover { background: var(--anzhiyu-theme-op); color: var(--anzhiyu-hover); }
.menus_groups .site-page.child.router-link-active { background: var(--anzhiyu-theme-op); font-weight: 600; }

/* 分类条对齐（补全参考站完整结构后的显式约束） */
#categoryBar { width: 100%; justify-content: flex-start; margin-bottom: 0; }
#categoryBar .category-bar { width: 100%; justify-content: flex-start; }
#categoryBar #catalog-bar { justify-content: flex-start; flex: 1; min-width: 0; }
#categoryBar #catalog-list { display: flex; overflow-x: auto; scrollbar-width: none; }
#categoryBar #catalog-list::-webkit-scrollbar { display: none; }
#categoryBar .catalog-more { margin-left: auto; padding: 0 .5rem; }
#categoryBar .catalog-list-item.selected a { background: var(--anzhiyu-theme); color: var(--anzhiyu-white); }

.page-enter-active, .page-leave-active { transition: opacity 0.3s, transform 0.3s; }
.page-enter-from { opacity: 0; transform: translateY(12px); }
.page-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
