<script setup>
import { ref, computed, onMounted } from 'vue'
import BannerHero from '../components/BannerHero.vue'
import SideBar from '../components/SideBar.vue'
import Pagination from '../components/Pagination.vue'
import { api } from '../lib/api'

const view = ref({ github: [], news: [], fusion: [], digests: [], generatedAt: '' })
const loading = ref(true)
const tab = ref('github')
const page = ref(1)
const pageSize = 10

const list = computed(() => {
  if (tab.value === 'github') return view.value.github
  return view.value.news
})
const paged = computed(() => list.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const total = computed(() => list.value.length)

onMounted(async () => {
  view.value = await api.get('/api/v1/hot')
  loading.value = false
})
</script>

<template>
  <BannerHero />
  <div class="layout" id="article-container">
    <main id="article-container article-main">
      <div class="card article fade-up">
        <div class="tabs">
          <button class="tab" :class="{ active: tab === 'github' }" @click="tab = 'github'; page = 1">🔥 GitHub 项目榜</button>
          <button class="tab" :class="{ active: tab === 'news' }" @click="tab = 'news'; page = 1">🤖 AI 资讯榜</button>
          <router-link class="tab link" to="/fusion">🔗 融合观察 →</router-link>
        </div>
        <div class="meta-line">生成于 {{ (view.generatedAt || '').slice(0, 16).replace('T', ' ') }} · 独立来源热度 × 快照差分增长</div>

        <div v-if="loading" class="loading">加载中 </div>

        <!-- GitHub 榜 -->
        <table v-if="!loading && tab === 'github'">
          <thead><tr><th></th><th>项目</th><th>语言</th><th>24h ★</th><th>热度</th></tr></thead>
          <tbody>
            <tr v-for="p in paged" :key="p.fullName" class="fade-up">
              <td class="rank"><span class="medal" :class="'m' + p.rank" v-if="p.rank <= 3">{{ p.rank }}</span><span v-else>{{ p.rank }}</span></td>
              <td>
                <router-link class="repo-name" :to="`/story/${p.storyId}`" v-if="p.storyId">{{ p.fullName }}</router-link>
                <a v-else class="repo-name" :href="p.url" target="_blank" rel="noopener">{{ p.fullName }}</a>
                <span v-for="b in p.badges" :key="b" class="badge" :class="{ new: b === '新', gh: b.startsWith('trending') }">{{ b }}</span>
                <div class="repo-desc">
                  <span class="zh" v-if="p.descriptionZh">{{ p.descriptionZh }}</span>
                  <span class="en" v-if="p.description"> {{ p.description }}</span>
                </div>
                <div class="topics"><span v-for="t in p.topics.slice(0, 4)" :key="t" class="chip">{{ t }}</span></div>
              </td>
              <td><span class="chip">{{ p.language || '-' }}</span></td>
              <td class="num gain">+{{ p.starsGained }}</td>
              <td class="num hot">{{ p.hotness.toFixed(1) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="!loading && tab === 'github' && !view.github.length" class="empty">暂无数据——先运行一次 githubhot run</div>

        <!-- AI 资讯榜 -->
        <div v-if="!loading && tab === 'news'">
          <div v-for="s in paged" :key="s.storyId" class="story fade-up">
            <div class="story-head">
              <span class="rank"><span class="medal" :class="'m' + s.rank" v-if="s.rank <= 3">{{ s.rank }}</span><span v-else>{{ s.rank }}</span></span>
              <a class="title" :href="s.url" target="_blank" rel="noopener">{{ s.titleZh }}</a>
              <span v-for="b in s.badges" :key="b" class="badge" :class="{ new: b === '新', rise: b === '上升', gh: b === 'GitHub关联' }">{{ b }}</span>
            </div>
            <div class="summary">{{ s.summaryZh }}</div>
            <div v-if="s.overview" class="overview">{{ s.overview }}</div>
            <div class="story-meta">来源 {{ s.sourceNames.join('、') }} · 评分 {{ s.score.toFixed(1) }} · 热度 {{ s.hotness.toFixed(1) }} · {{ s.tags.join(' / ') }}</div>
          </div>
          <div v-if="!view.news.length" class="empty">暂无数据</div>
        </div>

        <Pagination :total="total" :page="page" :page-size="pageSize" @change="page = $event" />
      </div>
    </main>
    <SideBar :view="view" />
  </div>
</template>

<style scoped>
.article { padding: 1.8rem 2.2rem; }
.tabs { display: flex; gap: 8px; flex-wrap: wrap; }
.tab { border: none; background: var(--anzhiyu-background); color: var(--anzhiyu-fontcolor); padding: 8px 20px; border-radius: var(--anzhiyu-radius-full); cursor: pointer; font: inherit; font-size: .92rem; transition: all .25s; }
.tab.active { background: var(--anzhiyu-theme); color: #fff; }
.tab.link { text-decoration: none; display: inline-flex; align-items: center; }
.meta-line { color: var(--anzhiyu-gray); font-size: .8rem; margin: 12px 0 4px; }
.rank { width: 42px; text-align: center; font-weight: 700; color: var(--anzhiyu-gray); }
.medal { display: inline-flex; width: 26px; height: 26px; border-radius: 50%; align-items: center; justify-content: center; font-size: .85rem; color: #fff; }
.medal.m1 { background: linear-gradient(135deg, #ffd18c, #f7a94b); }
.medal.m2 { background: linear-gradient(135deg, #dfe4ef, #b7c1d4); }
.medal.m3 { background: linear-gradient(135deg, #f3c3a4, #dd9368); }
.repo-name { font-weight: 700; color: var(--anzhiyu-blue); }
.repo-name:hover { color: var(--anzhiyu-hover); }
.repo-desc { color: var(--anzhiyu-secondary); font-size: .86rem; margin-top: 2px; }
.repo-desc .zh { color: var(--anzhiyu-fontcolor); }
.repo-desc .en { color: var(--anzhiyu-gray); font-size: .8rem; }
.topics { margin-top: 2px; }
.topics .chip { margin-right: 4px; }
.num { font-variant-numeric: tabular-nums; }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.story { padding: 14px 4px; border-bottom: 1px dashed var(--anzhiyu-card-border); }
.story:last-child { border-bottom: none; }
.story-head { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; }
.story-head .title { font-weight: 700; font-size: 1.05rem; }
.story-head .title:hover { color: var(--anzhiyu-hover); }
.summary { color: var(--anzhiyu-secondary); margin-top: 6px; font-size: .95rem; }
.overview { margin-top: 8px; padding: 8px 12px; background: var(--anzhiyu-background); border-radius: 6px; border-left: 3px solid var(--anzhiyu-theme); font-size: .88rem; color: var(--anzhiyu-secondary); }
.story-meta { color: var(--anzhiyu-gray); font-size: .8rem; margin-top: 6px; }
@media (max-width: 768px) { .article { padding: 1.2rem 1rem; } }
</style>
