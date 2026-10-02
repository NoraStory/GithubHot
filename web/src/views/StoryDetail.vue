<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'

const route = useRoute()
const detail = ref(null)
const error = ref('')
onMounted(async () => {
  try {
    detail.value = await api.get(`/api/v1/story/${route.params.id}`)
  } catch (e) {
    error.value = e.message
  }
})
function maxHot(history) {
  return Math.max(...history.map((h) => h.hotness), 1)
}
</script>

<template>
  <div class="layout page-enter">
    <main id="article-container">
      <div class="card article" v-if="detail">
        <h2 class="first-title">{{ detail.story.titleZh }} <a class="origin" :href="detail.story.url" target="_blank" rel="noopener">原文 ↗</a></h2>
        <div class="badges">
          <span v-for="b in detail.story.badges" :key="b" class="chip">{{ b }}</span>
          <span v-for="p in detail.story.projects" :key="p" class="chip blue">GitHub: {{ p }}</span>
          <span class="chip red">热度 {{ detail.story.hotness.toFixed(1) }}</span>
        </div>
        <div v-if="detail.story.overview" class="overview">{{ detail.story.overview }}</div>
        <div class="summary">{{ detail.story.summaryZh }}</div>

        <h3>热度走势</h3>
        <div v-if="detail.history.length >= 2" class="hist">
          <div v-for="(h, i) in detail.history" :key="i" class="bar" :style="{ height: (h.hotness / maxHot(detail.history) * 100) + '%' }" :title="h.hotness.toFixed(1)">
            <span>{{ h.hotness.toFixed(0) }}</span>
          </div>
        </div>
        <div v-else class="empty">历史数据不足两次观测，暂无走势</div>

        <h3>关联 GitHub 项目</h3>
        <table v-if="detail.projects.length">
          <thead><tr><th>项目</th><th>24h ★</th><th>热度</th></tr></thead>
          <tbody>
            <tr v-for="p in detail.projects" :key="p.fullName">
              <td><a class="repo-name" :href="p.url" target="_blank" rel="noopener">{{ p.fullName }}</a><div class="desc">{{ p.descriptionZh || p.description }}</div></td>
              <td class="num gain">+{{ p.starsGained }}</td>
              <td class="num hot">{{ p.hotness.toFixed(1) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty">未关联项目</div>

        <h3>同事件报道（{{ detail.members.length }}）</h3>
        <table v-if="detail.members.length">
          <thead><tr><th>来源</th><th>标题</th><th>评分 A/B</th><th>时间</th></tr></thead>
          <tbody>
            <tr v-for="m in detail.members" :key="m.id">
              <td>{{ m.sourceName }}</td>
              <td><a :href="m.url" target="_blank" rel="noopener">{{ m.titleZh || m.title }}</a></td>
              <td class="num">{{ m.scoreA.toFixed(1) }} / {{ m.scoreB.toFixed(1) }}</td>
              <td class="desc">{{ m.published }}</td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty">无成员条目</div>
      </div>
      <div class="card article" v-else-if="error"><div class="empty">{{ error }}</div></div>
      <div class="card article" v-else><div class="loading">加载中 </div></div>
    </main>
  </div>
</template>

<style scoped>
.origin { font-size: 1rem; color: var(--anzhiyu-blue); margin-left: 10px; }
.origin:hover { color: var(--anzhiyu-hover); }
.badges { margin: 10px 0; }
.badges .chip { margin-right: 6px; }
.overview { margin: 12px 0; padding: 12px 16px; background: var(--anzhiyu-background); border-left: 3px solid var(--anzhiyu-theme); border-radius: 6px; }
.summary { color: var(--anzhiyu-secondary); }
.hist { display: flex; align-items: flex-end; gap: 5px; height: 90px; padding: 8px 0 24px; }
.hist .bar { flex: 1; background: linear-gradient(180deg, var(--anzhiyu-theme), #f6d6c3); border-radius: 3px 3px 0 0; min-height: 4px; position: relative; }
.hist .bar span { position: absolute; bottom: -20px; left: 50%; transform: translateX(-50%); font-size: .66rem; color: var(--anzhiyu-gray); }
.repo-name { color: var(--anzhiyu-blue); font-weight: 700; }
.repo-name:hover { color: var(--anzhiyu-hover); }
.desc { color: var(--anzhiyu-gray); font-size: .82rem; }
.num { font-variant-numeric: tabular-nums; }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
</style>
