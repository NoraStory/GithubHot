<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

// 归档页（AnZhiYu article-sort 时间线同构）：期刊 + 事件按时间归档
const view = ref({ digests: [], stories: [] })
const loading = ref(true)
const year = ref('all')

const years = computed(() => {
  const ys = new Set()
  for (const d of view.value.digests || []) ys.add(d.date.slice(0, 4))
  return [...ys].sort().reverse()
})
const sorted = computed(() => {
  const all = []
  for (const d of view.value.digests || []) {
    all.push({ kind: d.kind === 'weekly' ? '周报' : d.kind === 'monthly' ? '月报' : '日报', title: `${d.date} 双热点报告`, date: d.date, url: `/digest/${d.date}`, y: d.date.slice(0, 4), m: d.date.slice(5, 7) })
  }
  for (const s of view.value.stories || []) {
    const ds = (s.firstSeenAt || '').slice(0, 10)
    all.push({ kind: '事件', title: s.titleZh, date: ds, url: `/story/${s.storyId}`, y: ds.slice(0, 4), m: ds.slice(5, 7) })
  }
  all.sort((a, b) => (a.date < b.date ? 1 : -1))
  return year.value === 'all' ? all : all.filter((x) => x.y === year.value)
})

import { computed } from 'vue'
onMounted(async () => {
  const [dg, sn] = await Promise.all([api.get('/api/v1/digests?pageSize=50'), api.get('/api/v1/hot/news')])
  view.value.digests = dg.items || []
  view.value.stories = sn.items || []
  loading.value = false
})
</script>

<template>
  <header class="post-bg post-bg--indigo" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">归档</router-link></div></div>
      <h1 class="post-title">文章归档</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">期刊与事件按时间归档 · 共 {{ sorted.length }} 篇</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div id="categoryBar">
          <div class="category-bar" id="category-bar">
            <div id="catalog-bar">
              <div id="catalog-list">
                <div class="catalog-list-item" id="all" :class="{ selected: year === 'all' }"><a href="javascript:void(0)" @click="year = 'all'">全部</a></div>
                <div v-for="y in years" :key="y" class="catalog-list-item" :id="y" :class="{ selected: year === y }"><a href="javascript:void(0)" @click="year = y">{{ y }}</a></div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="loading" class="loading">加载中 </div>
        <div class="article-sort" id="article-sort">
          <template v-for="(item, i) in sorted" :key="item.url + i">
            <div v-if="i === 0 || sorted[i - 1].y !== item.y" class="article-sort-item year"><span>{{ item.y }}</span></div>
            <router-link v-if="i === 0 || sorted[i - 1].y !== item.y || sorted[i - 1].m !== item.m" class="article-sort-item month" :to="`/archives/${item.y}/${item.m}`">{{ item.y }} 年 {{ Number(item.m) }} 月 ›</router-link>
            <div class="article-sort-item fade-up">
              <router-link class="article-sort-item-img" :to="item.url" :title="item.title">
                <img src="data:image/svg+xml;utf8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='300' height='168'%3E%3Crect width='300' height='168' fill='%23e3e8f7'/%3E%3C/svg%3E" alt="cover">
              </router-link>
              <div class="article-sort-item-info">
                <router-link class="article-sort-item-title" :to="item.url" :title="item.title">{{ item.title }}</router-link>
                <span class="article-sort-item-index">{{ i + 1 }}</span>
                <div class="article-meta-wrap">
                  <span class="article-sort-item-tags">
                    <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>{{ item.kind }}</span></a>
                  </span>
                  <div class="article-sort-item-time">
                    <time>{{ item.date }}</time>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>
        <div v-if="!loading && !sorted.length" class="empty">暂无归档</div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 19rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
.article-sort-item.month { display: block; margin: 10px 0 4px; padding: 6px 12px; font-weight: 700; font-size: .95rem; color: var(--anzhiyu-hover); background: var(--anzhiyu-theme-op); border-radius: 8px; }
.article-sort-item.month:hover { color: var(--anzhiyu-main); }
@media (max-width: 768px) { .post-bg { height: 15rem; } }
</style>
