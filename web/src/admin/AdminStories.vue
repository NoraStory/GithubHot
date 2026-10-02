<script setup>
// 事件管理：活跃事件列表 + 人工锁定/解锁（消费 /api/v1/hot/news + /admin/story/{id}/lock）
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const stories = ref([])
const loading = ref(true)
const msg = ref('')

async function load() {
  loading.value = true
  const d = await api.get('/api/v1/hot/news')
  stories.value = d.items || []
  loading.value = false
}
onMounted(load)

async function toggleLock(s) {
  msg.value = '处理中…'
  try {
    await api.post(`/api/v1/admin/story/${s.storyId}/lock`, { manual: !s.manual })
    s.manual = !s.manual
    msg.value = s.manual ? `已锁定：${s.titleZh}` : `已解锁：${s.titleZh}`
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
</script>

<template>
  <div class="page-enter">
    <h2>📌 事件管理</h2>
    <div v-if="msg" class="msg card">{{ msg }}</div>
    <div class="card">
      <div class="desc" style="margin-bottom:10px">
        锁定后该事件<strong>不再参与自动聚簇合并</strong>（标题/成员保持人工状态，AIHOT 同款保护）。解锁后恢复自动。
      </div>
      <div v-if="loading" class="loading">加载中 </div>
      <table v-if="!loading && stories.length">
        <thead><tr><th>事件</th><th>来源数</th><th>热度</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="s in stories" :key="s.storyId">
            <td>
              <router-link class="title" :to="`/story/${s.storyId}`">{{ s.titleZh }}</router-link>
              <div class="desc">{{ (s.projects || []).length ? '关联: ' + s.projects.join(', ') : '' }}</div>
            </td>
            <td class="num">{{ s.sourceCount }}</td>
            <td class="num hot">{{ s.hotness.toFixed(1) }}</td>
            <td>
              <span class="badge" :class="s.manual ? 'lockbadge' : 'okbadge'">{{ s.manual ? '已锁定' : '自动' }}</span>
            </td>
            <td>
              <button class="op" @click="toggleLock(s)">{{ s.manual ? '解锁' : '锁定' }}</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="empty">暂无活跃事件</div>
    </div>
  </div>
</template>

<style scoped>
h2 { font-size: 1.2rem; margin: 1.4rem 0 .8rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; margin-bottom: 1rem; overflow-x: auto; }
.msg { padding: 9px 14px; font-size: .88rem; color: var(--anzhiyu-secondary); }
.title { font-weight: 600; color: var(--anzhiyu-fontcolor); }
.title:hover { color: var(--anzhiyu-hover); }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
.num { font-variant-numeric: tabular-nums; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.badge { padding: 1px 10px; border-radius: 50px; font-size: .74rem; }
.okbadge { background: #238636; color: #fff; }
.lockbadge { background: #b62324; color: #fff; }
.op { border: 1px solid var(--anzhiyu-card-border); background: var(--anzhiyu-background); border-radius: var(--anzhiyu-radius); padding: 4px 14px; font: inherit; font-size: .84rem; cursor: pointer; }
.op:hover { border-color: var(--anzhiyu-hover); color: var(--anzhiyu-hover); }
</style>
