import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import { getToken } from './lib/api'

import Home from './views/Home.vue'
import Board from './views/Board.vue'
import Fusion from './views/Fusion.vue'
import StoryDetail from './views/StoryDetail.vue'
import SearchPage from './views/SearchPage.vue'
import Digests from './views/Digests.vue'
import DigestDetail from './views/DigestDetail.vue'
import About from './views/About.vue'
import Tools from './views/Tools.vue'
import Album from './views/Album.vue'
import Music from './views/Music.vue'
import AirConditioner from './views/AirConditioner.vue'
import Privacy from './views/Privacy.vue'
import Listing from './views/Listing.vue'
import Archives from './views/Archives.vue'
import TagCloud from './views/TagCloud.vue'
import Link from './views/Link.vue'
import Charts from './views/Charts.vue'
import AdminLayout from './admin/AdminLayout.vue'
import AdminLogin from './admin/AdminLogin.vue'
import AdminUsage from './admin/AdminUsage.vue'
import AdminDiagnostics from './admin/AdminDiagnostics.vue'
import AdminRuns from './admin/AdminRuns.vue'
import AdminSources from './admin/AdminSources.vue'
import AdminStories from './admin/AdminStories.vue'
import AdminDigests from './admin/AdminDigests.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Home },
    { path: '/github', component: Board, props: { board: 'github' } },
    { path: '/news', component: Board, props: { board: 'news' } },
    { path: '/fusion', component: Fusion },
    { path: '/story/:id', component: StoryDetail },
    { path: '/search', component: SearchPage },
    { path: '/digests', component: Digests },
    { path: '/digest/latest', component: DigestDetail, props: { latest: true } },
    { path: '/digest/:date', component: DigestDetail },
    { path: '/about', component: About },
    { path: '/tools', component: Tools },
    { path: '/album', component: Album },
    { path: '/music', component: Music },
    { path: '/air-conditioner', component: AirConditioner },
    { path: '/privacy', component: Privacy },
    { path: '/archives', component: Archives },
    { path: '/categories', component: TagCloud, props: { mode: 'categories' } },
    { path: '/tags', component: TagCloud, props: { mode: 'tags' } },
    { path: '/link', component: Link },
    { path: '/charts', component: Charts },
    { path: '/categories/:name', component: Listing },
    { path: '/tags/:name', component: Listing },
    { path: '/archives/:year/:month', component: Listing },
    { path: '/admin/login', component: AdminLogin },
    {
      path: '/admin',
      component: AdminLayout,
      children: [
        { path: '', redirect: '/admin/usage' },
        { path: 'usage', component: AdminUsage },
        { path: 'diagnostics', component: AdminDiagnostics },
        { path: 'runs', component: AdminRuns },
        { path: 'sources', component: AdminSources },
        { path: 'stories', component: AdminStories },
        { path: 'digests', component: AdminDigests }
      ]
    },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ],
  scrollBehavior(to, from, saved) {
    return saved || { top: 0 }
  }
})

// 管理端登录门：无令牌时引导到登录页
router.beforeEach((to) => {
  if (to.path.startsWith('/admin') && to.path !== '/admin/login' && !getToken()) {
    return { path: '/admin/login', query: { redirect: to.fullPath } }
  }
})

// 背景视频本地缓存：SW 把远端 mp4 落 Cache Storage，重播/刷新不再卡网
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(() => {})
}

createApp(App).use(router).mount('#app')
