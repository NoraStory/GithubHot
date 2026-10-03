<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'
import { repoAvatar, repoCover } from '../lib/covers'

const route = useRoute()
const detail = ref(null)
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  try {
    detail.value = await api.get(`/api/v1/story/${route.params.id}`)
  } catch (e) {
    error.value = e.message
  }
  loading.value = false
})
function maxHot(history) {
  return Math.max(...(history || []).map((h) => h.hotness), 1)
}
</script>

<template>
  <!-- post-bg 文章页头（AnZhiYu #post-info 同构） -->
  <header class="post-bg" id="page-header" v-if="detail">
    <div id="post-info">
      <div id="post-firstinfo">
        <div class="meta-firstline">
          <router-link class="post-meta-original" to="/">事件</router-link>
          <a v-if="detail.story.url" class="post-meta-original read-origin" :href="detail.story.url" target="_blank" rel="noopener" title="原文链接，可能需翻墙">阅读原文 ↗</a>
          <span class="article-meta tags">
            <a v-for="t in detail.story.tags" :key="t" class="article-meta__tags"><span><i class="anzhiyufont anzhiyu-icon-hashtag"></i>{{ t }}</span></a>
          </span>
        </div>
      </div>
      <h1 class="post-title">{{ detail.story.titleZh }}</h1>
      <div id="post-meta">
        <div class="meta-firstline">
          <span class="post-meta-date">
            <i class="anzhiyufont anzhiyu-icon-calendar-days post-meta-icon"></i>
            <span class="post-meta-label">发现于</span>
            <time>{{ (detail.story.firstSeenAt || '').slice(0, 10) }}</time>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-wordcount">
            <span class="post-meta-label">热度</span>
            <span class="hot">{{ detail.story.hotness.toFixed(1) }}</span>
          </span>
          <span class="post-meta-separator"></span>
          <span class="post-meta-wordcount">
            <span class="post-meta-label">同事件报道</span>
            <span>{{ detail.members.length }}</span>
          </span>
        </div>
      </div>
    </div>
  </header>
  <div v-else-if="loading" class="loading" style="padding-top:100px">加载中 </div>
  <div v-else class="empty" style="padding-top:100px">{{ error }}</div>

  <main class="layout" id="content-inner" v-if="detail">
    <div id="post">
      <div id="article-container" class="article">
        <blockquote v-if="detail.story.overview" class="event-overview"><strong>事件综述：</strong>{{ detail.story.overview }}</blockquote>
        <p v-if="detail.story.summaryZh">{{ detail.story.summaryZh }}</p>

        <h2>热度走势</h2>
        <div v-if="detail.history.length >= 2" class="hist">
          <div v-for="(h, i) in detail.history" :key="i" class="bar" :style="{ height: (h.hotness / maxHot(detail.history) * 100) + '%' }" :title="h.hotness.toFixed(1)">
            <span>{{ h.hotness.toFixed(0) }}</span>
          </div>
        </div>
        <div v-else class="empty">历史数据不足两次观测，暂无走势</div>

        <h2>关联 GitHub 项目</h2>
        <div v-if="!detail.projects.length" class="empty">未关联项目</div>
        <div v-for="p in detail.projects" :key="p.fullName" class="recent-post-item fade-up">
          <div class="post_cover left"><a :href="p.url" target="_blank" rel="noopener"><img class="post_bg" :src="repoAvatar(p.fullName)" :alt="p.fullName" @error="e => e.target.src = repoCover(p.fullName)"></a></div>
          <div class="recent-post-info">
            <div class="recent-post-info-top">
              <div class="recent-post-info-top-tips"><div class="article-categories-original">GitHub 项目</div></div>
              <a class="article-title" :href="p.url" target="_blank" rel="noopener">{{ p.fullName }}</a>
            </div>
            <div class="article-meta-wrap">
              <span class="post-meta-date"><time class="gain">+{{ p.starsGained }} ★</time><span class="article-meta-separator">·</span><span class="hot">热度 {{ p.hotness.toFixed(1) }}</span></span>
            </div>
            <div class="recent-post-desc">{{ p.descriptionZh || p.description }}</div>
          </div>
        </div>

        <h2>同事件报道（{{ detail.members.length }}）</h2>
        <div v-if="!detail.members.length" class="empty">无成员条目</div>
        <div v-for="(m, i) in detail.members" :key="m.id" class="story-line fade-up">
          <a class="member-link" :href="m.url" target="_blank" rel="noopener">{{ m.titleZh || m.title }}</a>
          <span class="desc">{{ m.sourceName }} · {{ m.published }} · 评分 {{ m.scoreA.toFixed(1) }}/{{ m.scoreB.toFixed(1) }}
            <a v-if="m.url" class="read-origin-mini" :href="m.url" target="_blank" rel="noopener">原文 ↗</a>
          </span>
          <details v-if="m.contentZh" class="member-zh" :open="i === 0">
            <summary><span class="zh-badge">AI 译文</span><span class="zh-title">📄 本地中文翻译<em>原站打不开时读这份</em></span><span class="zh-arrow anzhiyufont anzhiyu-icon-arrow-right"></span></summary>
            <p>{{ m.contentZh }}</p>
          </details>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 22rem; position: relative; overflow: hidden; }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; padding: 0 1.5rem; }
.post-title { font-size: 1.9rem; font-weight: 700; margin: 12px 0; text-shadow: 0 3px 14px rgba(0,0,0,.3); max-width: 900px; }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { display: flex; gap: 12px; align-items: center; justify-content: center; opacity: .92; font-size: .88rem; flex-wrap: wrap; }
.event-overview { margin: 0 0 1.1rem; padding: 0.9rem 1.2rem; background: var(--anzhiyu-theme-op); border-left: 4px solid var(--anzhiyu-hover); border-radius: 0 8px 8px 0; color: var(--anzhiyu-secondary); }
.hist { display: flex; align-items: flex-end; gap: 5px; height: 90px; padding: 8px 0 24px; }
.hist .bar { flex: 1; border-radius: 3px 3px 0 0; min-height: 4px; position: relative; }
.hist .bar span { position: absolute; bottom: -20px; left: 50%; transform: translateX(-50%); font-size: .66rem; color: var(--anzhiyu-gray); }
.gain { color: var(--anzhiyu-green); font-weight: 700; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
.story-line { padding: 10px 4px; border-bottom: 1px dashed var(--anzhiyu-card-border); display: flex; flex-direction: column; gap: 3px; }
.story-line:last-child { border-bottom: none; }
.member-link { font-weight: 600; color: var(--anzhiyu-fontcolor); }
.member-link:hover { color: var(--anzhiyu-hover); }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
.read-origin { background: var(--anzhiyu-main) !important; }
.read-origin-mini { margin-left: 8px; color: var(--anzhiyu-main); }
.read-origin-mini:hover { color: var(--anzhiyu-hover); }
.member-zh {
  margin-top: 10px;
  border: 1px solid var(--anzhiyu-theme-op);
  border-left: 4px solid var(--anzhiyu-theme);
  border-radius: 10px;
  padding: 10px 14px;
  background: linear-gradient(135deg, var(--anzhiyu-theme-op) 0%, transparent 60%);
  box-shadow: 0 2px 10px rgba(0, 0, 0, .04);
}
.member-zh summary {
  cursor: pointer;
  user-select: none;
  list-style: none;
  display: flex;
  align-items: center;
  gap: 10px;
}
.member-zh summary::-webkit-details-marker { display: none; }
.zh-badge {
  flex: none;
  background: var(--anzhiyu-theme);
  color: #fff;
  font-size: .72rem;
  font-weight: 700;
  letter-spacing: .05em;
  padding: 3px 10px;
  border-radius: 50px;
  box-shadow: 0 2px 8px var(--anzhiyu-theme-op);
}
.zh-title { font-size: .9rem; font-weight: 600; color: var(--anzhiyu-fontcolor); display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.zh-title em { font-style: normal; font-size: .74rem; font-weight: 400; color: var(--anzhiyu-gray); }
.zh-arrow { margin-left: auto; flex: none; font-size: .8rem; color: var(--anzhiyu-theme); transition: transform .25s; }
.member-zh[open] .zh-arrow { transform: rotate(90deg); }
.member-zh p {
  margin: 10px 2px 2px;
  padding-top: 10px;
  border-top: 1px dashed var(--anzhiyu-theme-op);
  font-size: .92rem;
  line-height: 1.95;
  color: var(--anzhiyu-secondary);
  white-space: pre-line;
}
@media (max-width: 768px) { .post-bg { height: 18rem; } .post-title { font-size: 1.4rem; } }
</style>

<style>
/* ===== 事件详情阅读排版（与日报查看器一致：窄栏/舒适行高/标题层级）===== */
#content-inner > #post { max-width: 46rem; margin: 0 auto; }
#article-container.article { font-size: 1.04rem; line-height: 1.9; letter-spacing: 0.01em; }
#article-container.article h2 { font-size: 1.28rem; margin: 2rem 0 0.9rem; padding: 0.55rem 1rem; background: var(--anzhiyu-theme-op); border-radius: 8px; }
#article-container.article p { margin: 0.9rem 0; color: var(--anzhiyu-secondary); }
#article-container.article .event-overview strong { color: var(--anzhiyu-hover); }
#article-container.article .recent-post-desc { line-height: 1.85; }
#article-container.article .member-link { font-size: 1.02rem; line-height: 1.7; }
</style>
