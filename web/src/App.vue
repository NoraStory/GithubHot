<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
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

function applyTheme() {
  document.documentElement.setAttribute('data-theme', dark.value ? 'dark' : 'light')
  localStorage.setItem('githubhot_theme', dark.value ? 'dark' : 'light')
}
function toggleTheme() { dark.value = !dark.value; applyTheme() }
function onScroll() { scrolled.value = window.scrollY > 20 }
function toggleMenu() { menuOpen.value = !menuOpen.value }
function closeMenu() { menuOpen.value = false }

async function doSearch() {
  if (!searchQ.value.trim()) return
  const d = await api.get(`/api/v1/search?q=${encodeURIComponent(searchQ.value)}`)
  searchResults.value = d.results || []
}

onMounted(() => {
  applyTheme()
  window.addEventListener('scroll', onScroll, { passive: true })
})
onBeforeUnmount(() => window.removeEventListener('scroll', onScroll))

router.afterEach(() => { menuOpen.value = false; searchMask.value = false })
</script>

<template>
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
              </div>
            </div>
            <div class="back-menu-list-group">
              <div class="back-menu-list-title">期刊</div>
              <div class="back-menu-list">
                <a class="back-menu-item" href="/digest/latest"><span class="back-menu-item-text">最新日报</span></a>
                <a class="back-menu-item" href="/digests"><span class="back-menu-item-text">全部期刊</span></a>
                <a class="back-menu-item" href="/archives"><span class="back-menu-item-text">归档</span></a>
                <a class="back-menu-item" href="/llms.txt" target="_blank"><span class="back-menu-item-text">llms.txt</span></a>
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
          <router-link class="site-page" to="/about"><span> 关于</span></router-link>
        </div>
      </div>
    </div>
    <div id="nav-right">
      <div class="nav-button" id="search-button" title="站内搜索" @click="searchMask = true">
        <a class="site-page social-icon search"><i class="anzhiyufont anzhiyu-icon-magnifying-glass"></i></a>
      </div>
      <div class="nav-button" title="深色/浅色" @click="dark = !dark; applyTheme()">
        <a class="site-page social-icon">{{ dark ? '🌞' : '🌙' }}</a>
      </div>
      <router-link v-if="getToken()" class="nav-button" to="/admin/usage" title="管理端">
        <a class="site-page social-icon">⚙️</a>
      </router-link>
      <div class="nav-button" id="toggle-menu" title="菜单" @click="toggleMenu">
        <a class="site-page social-icon"><i class="anzhiyufont anzhiyu-icon-bars"></i></a>
      </div>
    </div>
  </nav>

  <!-- 窄屏抽屉菜单（AnZhiYu #sidebar-menus 同构） -->
  <div id="sidebar" v-if="menuOpen" @click.self="closeMenu">
    <div class="sidebar-menus" id="sidebar-menus">
      <div class="sidebar-author">
        <div class="author-name">🔥 GithubHot</div>
        <div class="author-desc">双热点追踪站</div>
      </div>
      <div class="menus_groups">
        <router-link class="site-page child" to="/"><span> 首页</span></router-link>
        <div class="group-title">热点</div>
        <router-link class="site-page child" to="/github"><span> GitHub 项目榜</span></router-link>
        <router-link class="site-page child" to="/news"><span> AI 资讯榜</span></router-link>
        <router-link class="site-page child" to="/fusion"><span> 融合观察</span></router-link>
        <div class="group-title">期刊</div>
        <router-link class="site-page child" to="/digests"><span> 全部期刊</span></router-link>
        <router-link class="site-page child" to="/archives"><span> 归档</span></router-link>
        <a class="site-page child" href="/feed/digest.xml" target="_blank"><span> RSS 订阅</span></a>
        <div class="group-title">发现</div>
        <router-link class="site-page child" to="/categories"><span> 分类</span></router-link>
        <router-link class="site-page child" to="/tags"><span> 标签</span></router-link>
        <router-link class="site-page child" to="/charts"><span> 统计</span></router-link>
        <router-link class="site-page child" to="/link"><span> 资源</span></router-link>
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
/* 主题外的少量接线样式 */
#nav #nav-group { display: flex; align-items: center; gap: 14px; flex: 1; min-width: 0; }
#nav #site-name .site-name-text { font-weight: 700; }
#nav #nav-right { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
#nav .nav-button { cursor: pointer; }
#nav .nav-button .site-page { padding: 8px 10px; border-radius: var(--anzhiyu-radius); }
#search-mask { position: fixed; inset: 0; z-index: 1001; background: rgba(0, 0, 0, 0.45); display: flex; align-items: flex-start; justify-content: center; padding-top: 12vh; backdrop-filter: blur(3px); }
.search-dialog { width: min(640px, 90vw); background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: 0 2px 16px -3px rgba(0,0,0,.15); overflow: hidden; }
.search-dialog-title { padding: 12px 16px; font-weight: 700; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--anzhiyu-card-border); }
.search-dialog-title .close { cursor: pointer; color: var(--anzhiyu-gray); }
.search-dialog-input { display: flex; gap: 8px; padding: 12px 16px; }
.search-dialog-input input { flex: 1; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 9px 12px; font: inherit; outline: none; color: var(--anzhiyu-fontcolor); }
.search-dialog-input button { border: none; background: var(--anzhiyu-theme); color: #fff; border-radius: var(--anzhiyu-radius); padding: 0 18px; cursor: pointer; }
.search-dialog-results { max-height: 50vh; overflow: auto; padding: 8px 16px 14px; }
.result-item { display: block; padding: 8px 6px; border-radius: 6px; font-size: 0.92rem; }
.result-item:hover { background: var(--anzhiyu-background); }

/* 窄屏：隐藏平铺菜单，仅汉堡按钮（AnZhiYu 断点行为） */
#toggle-menu { display: none; }
@media (max-width: 900px) {
  #nav .menus_items { display: none; }
  #toggle-menu { display: block; }
  #nav .back-home-button { display: none; }
}

/* 抽屉菜单（#sidebar-menus 主题同构） */
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

.page-enter-active, .page-leave-active { transition: opacity 0.3s, transform 0.3s; }
.page-enter-from { opacity: 0; transform: translateY(12px); }
.page-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
