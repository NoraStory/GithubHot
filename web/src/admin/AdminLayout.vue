<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../lib/api'
const router = useRouter()
const menus = [
  { path: '/admin/usage', label: '💰 Token 用量' },
  { path: '/admin/stories', label: '📌 事件管理' },
  { path: '/admin/diagnostics', label: '🩺 内容诊断' },
  { path: '/admin/runs', label: '🖥 运行历史' },
  { path: '/admin/sources', label: '🛠 信源管理' },
  { path: '/admin/digests', label: '📰 期刊' }
]
const health = ref(null)
onMounted(async () => {
  try { health.value = await api.get('/healthz') } catch { health.value = null }
})
</script>

<template>
  <!-- 管理端布局：左侧栏菜单 + 内容区（与用户端视觉分离） -->
  <div class="admin-shell">
    <aside class="sidebar">
      <div class="brand"><span class="dot"></span>GithubHot <span class="tag">管理端</span></div>
      <router-link v-for="m in menus" :key="m.path" :to="m.path" class="menu-item">{{ m.label }}</router-link>
      <div class="spacer"></div>
      <div class="health" v-if="health">
        <span class="dot" :class="health.status === 'ok' ? 'ok' : 'bad'"></span>
        服务正常 · v{{ health.version }}
      </div>
      <router-link class="menu-item back" to="/">← 返回用户端</router-link>
    </aside>
    <main class="content">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="$route.fullPath" />
        </transition>
      </router-view>
    </main>
  </div>
</template>

<style scoped>
.health { padding: 8px; font-size: .78rem; color: var(--anzhiyu-gray); display: flex; align-items: center; gap: 6px; }
.health .dot { width: 8px; height: 8px; border-radius: 50%; }
.health .dot.ok { background: var(--anzhiyu-green); box-shadow: 0 0 0 3px rgba(54,181,98,.2); }
.health .dot.bad { background: var(--anzhiyu-red); }
.admin-shell { display: flex; min-height: 100vh; }
.sidebar { width: 230px; flex-shrink: 0; background: var(--anzhiyu-card-bg); outline: 1px solid var(--anzhiyu-card-border); padding: 1.2rem .9rem; display: flex; flex-direction: column; gap: 4px; position: sticky; top: 0; height: 100vh; }
.brand { font-weight: 700; font-size: 1.05rem; display: flex; align-items: center; gap: 8px; padding: 4px 8px 14px; }
.brand .dot { width: 10px; height: 10px; border-radius: 50%; background: var(--anzhiyu-theme); box-shadow: 0 0 0 4px var(--anzhiyu-theme-op); }
.brand .tag { font-size: .7rem; background: var(--anzhiyu-theme-op); color: #a8766f; padding: 1px 8px; border-radius: 50px; font-weight: 400; }
.menu-item { padding: 10px 12px; border-radius: var(--anzhiyu-radius); font-size: .95rem; color: var(--anzhiyu-secondary); }
.menu-item:hover { background: var(--anzhiyu-background); color: var(--anzhiyu-hover); }
.menu-item.router-link-active { background: var(--anzhiyu-theme-op); color: var(--anzhiyu-fontcolor); font-weight: 600; }
.spacer { flex: 1; }
.back { color: var(--anzhiyu-gray); }
.content { flex: 1; padding: 1.6rem 2rem; min-width: 0; }
@media (max-width: 800px) { .admin-shell { flex-direction: column; } .sidebar { width: 100%; height: auto; position: static; flex-direction: row; flex-wrap: wrap; } .brand { width: 100%; } .spacer { display: none; } }
</style>
