<script setup>
// 我的仓库：作者 GitHub 公开仓库快照（流水线定时刷新，原创在前、按最近 push 排序）。
// 数据源 /api/v1/my/repos：fullName/url/description/language/topics/homepage/stars/forks/fork/pushedAt。
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api'

const view = ref({ repos: [], owner: '', generatedAt: '' })
const loading = ref(true)

const own = computed(() => view.value.repos.filter((r) => !r.fork))
const forks = computed(() => view.value.repos.filter((r) => r.fork))

// 语言色点（主流语言配色，未收录用主题灰）
const langColors = {
  Go: '#00ADD8', Python: '#3572A5', JavaScript: '#f1e05a', TypeScript: '#3178c6',
  Rust: '#dea584', Java: '#b07219', Kotlin: '#A97BFF', 'C++': '#f34b7d',
  C: '#555555', 'C#': '#178600', Vue: '#41b883', HTML: '#e34c26', CSS: '#563d7c',
  Shell: '#89e051', Svelte: '#ff3e00', Dart: '#00B4AB', PHP: '#4F5D95',
}
function langColor(lang) { return langColors[lang] || 'var(--anzhiyu-gray)' }
function shortName(fullName) { return (fullName || '').split('/').slice(1).join('/') }
function fmtDate(s) { return (s || '').slice(0, 10) }

onMounted(async () => {
  try { view.value = await api.get('/api/v1/my/repos') } catch (e) { /* 空态兜底 */ }
  loading.value = false
})
</script>

<template>
  <header class="post-bg post-bg--sunset" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <router-link class="post-meta-original" to="/my-repos">我的仓库</router-link>
          <span class="article-meta tags">
            <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>GitHub</span></a>
            <a class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>开源</span></a>
          </span>
        </div>
      </div>
      <h1 class="post-title">作者的开源项目</h1>
      <div id="post-meta" v-if="view.owner">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-github post-meta-icon"></i>
            <span class="post-meta-label">@{{ view.owner }} ·</span>
            <span class="word-count">{{ own.length }}</span>
            <span class="post-meta-label">个原创 · {{ forks.length }} 个 Fork</span>
          </span>
        </div>
      </div>
    </div>
  </header>

  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <div v-if="loading" class="loading">加载中</div>
        <div v-else-if="!view.repos.length" class="empty">暂无仓库数据，流水线完成第一轮刷新后展示</div>

        <template v-else>
          <div class="group-title"><i class="anzhiyufont anzhiyu-icon-cube"></i> 原创项目</div>
          <div class="repo-list">
            <a v-for="r in own" :key="r.fullName" class="repo-row fade-up" :href="r.url" target="_blank" rel="noopener">
              <div class="repo-head">
                <span class="repo-name">{{ shortName(r.fullName) }}</span>
                <span v-if="r.homepage" class="repo-homepage">{{ r.homepage.replace(/^https?:\/\//, '') }}</span>
              </div>
              <div class="repo-desc" v-if="r.description">{{ r.description }}</div>
              <div class="repo-meta">
                <span v-if="r.language" class="lang"><span class="dot" :style="{ background: langColor(r.language) }"></span>{{ r.language }}</span>
                <span v-if="r.stars" class="stat"><i class="anzhiyufont anzhiyu-icon-fire"></i> {{ r.stars }}</span>
                <span v-if="r.forks" class="stat"><i class="anzhiyufont anzhiyu-icon-arrows-left-right"></i> {{ r.forks }}</span>
                <span v-for="t in (r.topics || []).slice(0, 3)" :key="t" class="row-tag">{{ t }}</span>
                <span class="pushed">更新于 {{ fmtDate(r.pushedAt) }}</span>
              </div>
            </a>
          </div>

          <div class="group-title" v-if="forks.length"><i class="anzhiyufont anzhiyu-icon-shuffle"></i> Fork 仓库</div>
          <div class="repo-list" v-if="forks.length">
            <a v-for="r in forks" :key="r.fullName" class="repo-row fade-up" :href="r.url" target="_blank" rel="noopener">
              <div class="repo-head">
                <span class="repo-name">{{ shortName(r.fullName) }}</span>
                <span class="badge fork">Fork</span>
              </div>
              <div class="repo-desc" v-if="r.description">{{ r.description }}</div>
              <div class="repo-meta">
                <span v-if="r.language" class="lang"><span class="dot" :style="{ background: langColor(r.language) }"></span>{{ r.language }}</span>
                <span v-if="r.stars" class="stat"><i class="anzhiyufont anzhiyu-icon-fire"></i> {{ r.stars }}</span>
                <span class="pushed">更新于 {{ fmtDate(r.pushedAt) }}</span>
              </div>
            </a>
          </div>
        </template>
      </div>
    </div>
  </main>
</template>

<style scoped>
#post { max-width: 100%; margin: 0; }
.post-bg { height: 20rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; margin: 10px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
.article-meta__tags { display: inline-block; background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 50px; padding: 1px 10px; font-size: .74rem; margin-left: 6px; }
#post-meta .meta-firstline { display: flex; gap: 12px; align-items: center; justify-content: center; flex-wrap: wrap; opacity: .92; font-size: .88rem; }
.group-title { display: flex; align-items: center; gap: 8px; font-weight: 800; font-size: 1.08rem; color: var(--anzhiyu-fontcolor); margin: 1.6rem 0 .8rem; }
.group-title:first-of-type { margin-top: .4rem; }

/* ===== 仓库行 ===== */
.repo-list { display: flex; flex-direction: column; gap: 10px; }
.repo-row { display: block; background: var(--anzhiyu-maskbg); border: 1px solid var(--anzhiyu-card-border); border-radius: 12px; padding: 14px 16px; text-decoration: none; transition: border-color .2s, transform .2s; }
.repo-row:hover { border-color: var(--anzhiyu-theme); transform: translateY(-1px); }
.repo-head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.repo-name { font-weight: 800; font-size: 1.05rem; color: var(--anzhiyu-blue); }
.repo-row:hover .repo-name { color: var(--anzhiyu-hover); }
.repo-homepage { font-size: .76rem; color: var(--anzhiyu-gray); }
.badge { display: inline-block; border-radius: 6px; padding: 0 7px; font-size: .72rem; }
.badge.fork { background: var(--anzhiyu-theme-op); color: #a8766f; }
.repo-desc { margin-top: 4px; font-size: .86rem; color: var(--anzhiyu-secondary); line-height: 1.7; }
.repo-meta { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-top: 8px; font-size: .78rem; color: var(--anzhiyu-gray); }
.lang { display: inline-flex; align-items: center; gap: 5px; }
.dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.stat { display: inline-flex; align-items: center; gap: 4px; }
.row-tag { background: var(--anzhiyu-theme-op); color: #a8766f; border-radius: 6px; padding: 0 7px; font-size: .72rem; }
.pushed { margin-left: auto; }
@media (max-width: 768px) { .post-bg { height: 16rem; } .post-title { font-size: 1.4rem; } .pushed { margin-left: 0; } }
</style>
