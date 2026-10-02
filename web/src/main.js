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
import AdminLayout from './admin/AdminLayout.vue'
import AdminLogin from './admin/AdminLogin.vue'
import AdminUsage from './admin/AdminUsage.vue'
import AdminDiagnostics from './admin/AdminDiagnostics.vue'
import AdminRuns from './admin/AdminRuns.vue'
import AdminSources from './admin/AdminSources.vue'
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
    { path: '/digest/:date', component: DigestDetail },
    { path: '/about', component: About },
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

// 页面切换淡入
router.afterEach(() => {
  document.documentElement.classList.add('page-switching')
  setTimeout(() => document.documentElement.classList.remove('page-switching'), 60)
})

createApp(App).use(router).mount('#app')
