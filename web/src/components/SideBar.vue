<script setup>
import { ref, onMounted } from 'vue'
import { loadSiteConfig } from '../lib/api'

const props = defineProps({ view: { type: Object, default: () => ({}) } })
const emit = defineEmits([])
const dark = ref(false)
const musicCount = ref(0)

onMounted(async () => {
  dark.value = document.documentElement.getAttribute('data-theme') === 'dark'
  const cfg = await loadSiteConfig()
  musicCount.value = (cfg.music || []).length
})
</script>

<template>
  <aside id="aside-content">
    <!-- 作者卡（渐变动画头图 + site-data，AnZhiYu 同构） -->
    <div class="card-widget card-info">
      <div class="avatar-bg"></div>
      <div class="card-info-content">
        <div class="author">🔥 GithubHot</div>
        <div class="desc">双热点追踪站 · 自动采集与 LLM 精选</div>
        <div class="site-data">
          <div><div class="length-num">{{ (view.github || []).length }}</div><div class="headline">项目</div></div>
          <div><div class="length-num">{{ (view.news || []).length }}</div><div class="headline">事件</div></div>
          <div><div class="length-num">{{ (view.fusion || []).length }}</div><div class="headline">融合</div></div>
        </div>
      </div>
    </div>

    <div class="card-widget">
      <div class="widget-title">期刊</div>
      <div class="widget-list">
        <router-link v-for="d in view.digests || []" :key="d.date" :to="`/digest/${d.date}`">
          <span>{{ d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报' }} {{ d.date }}</span>
          <span class="date">→</span>
        </router-link>
        <router-link to="/digests"><span>全部期刊</span><span class="date">→</span></router-link>
      </div>
    </div>

    <div class="card-widget">
      <div class="widget-title">订阅</div>
      <div class="widget-list">
        <a href="/feed/news.xml" target="_blank" rel="noopener"><span>RSS · AI 资讯榜</span><span class="date">→</span></a>
        <a href="/feed/github.xml" target="_blank" rel="noopener"><span>RSS · GitHub 项目榜</span><span class="date">→</span></a>
        <a href="/feed/digest.xml" target="_blank" rel="noopener"><span>RSS · 期刊</span><span class="date">→</span></a>
      </div>
    </div>

    <div class="card-widget">
      <div class="widget-title">更多</div>
      <div class="widget-list">
        <a href="/api/v1/hot" target="_blank" rel="noopener"><span>API · /api/v1/hot</span><span class="date">→</span></a>
        <a href="/llms.txt" target="_blank" rel="noopener"><span>llms.txt（Agent）</span><span class="date">→</span></a>
        <a href="/api/v1/agent/hot.md" target="_blank" rel="noopener"><span>Agent Markdown</span><span class="date">→</span></a>
        <a href="https://github.com/NoraStory/GithubHot" target="_blank" rel="noopener"><span>GitHub 仓库</span><span class="date">→</span></a>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.card-widget { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); transition: box-shadow .3s; padding: 1.1rem 1.25rem; margin-bottom: 1.25rem; }
.card-widget:hover { box-shadow: var(--card-hover-box-shadow); }
.card-info { padding: 0; overflow: hidden; }
.avatar-bg { height: 92px; background: linear-gradient(120deg, #eabcbd, #f6d6c3 40%, #c9d4f4 80%); background-size: 200% 200%; animation: gradient-move 10s ease infinite; }
.card-info-content { padding: .9rem 1.25rem 1.1rem; }
.author { font-weight: 700; font-size: 1.05rem; }
.desc { color: var(--anzhiyu-gray); font-size: .82rem; margin-top: 2px; }
.site-data { display: flex; margin-top: 12px; border-top: 1px dashed var(--anzhiyu-card-border); padding-top: 12px; }
.site-data > div { flex: 1; text-align: center; }
.site-data .headline { color: var(--anzhiyu-gray); font-size: .78rem; }
.site-data .length-num { font-weight: 700; font-size: 1.1rem; color: var(--anzhiyu-hover); }
.widget-title { font-weight: 700; margin-bottom: 8px; }
.widget-list a { display: flex; justify-content: space-between; padding: 6px 2px; border-radius: 6px; font-size: .92rem; color: var(--anzhiyu-secondary); }
.widget-list a:hover { background: var(--anzhiyu-background); color: var(--anzhiyu-hover); padding-left: 8px; }
.widget-list .date { color: var(--anzhiyu-gray); font-size: .78rem; }
</style>
