<script setup>
// 留言板（内置后端 /api/v1/messages，30 秒限频）
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const items = ref([])
const form = ref({ nickname: '', content: '' })
const msg = ref('')
const loading = ref(true)
const posting = ref(false)

async function load() {
  loading.value = true
  const d = await api.get('/api/v1/messages?pageSize=50')
  items.value = d.items || []
  loading.value = false
}
onMounted(load)

async function submit() {
  if (!form.value.content.trim()) { msg.value = '内容不能为空'; return }
  posting.value = true
  msg.value = ''
  try {
    await api.post('/api/v1/messages', form.value)
    form.value.content = ''
    msg.value = '留言成功 ✓'
    await load()
  } catch (e) {
    msg.value = e.message
  }
  posting.value = false
}
</script>

<template>
  <header class="post-bg" id="page-header">
    <div id="post-info">
      <div id="post-firstinfo"><div class="meta-firstline"><router-link class="post-meta-original" to="/">留言板</router-link></div></div>
      <h1 class="post-title">给我留言</h1>
      <div id="post-meta"><div class="meta-firstline">
        <span class="post-meta-label">分享你对双热点站的建议 · 每 30 秒可留一条</span>
      </div></div>
    </div>
  </header>
  <main class="layout" id="content-inner">
    <div id="post">
      <div id="article-container" class="article">
        <h2 class="first-title">写留言</h2>
        <div class="form">
          <input v-model="form.nickname" placeholder="昵称（可选）" maxlength="24">
          <textarea v-model="form.content" placeholder="说点什么…（最多 400 字）" maxlength="400" rows="4"></textarea>
          <button :disabled="posting" @click="submit">{{ posting ? '提交中…' : '提交留言' }}</button>
        </div>
        <div v-if="msg" class="form-msg">{{ msg }}</div>

        <h2>全部留言（{{ items.length }}）</h2>
        <div v-if="loading" class="loading">加载中 </div>
        <div v-else-if="!items.length" class="empty">还没有留言，来抢沙发～</div>
        <div v-for="m in items" :key="m.id" class="msg-card fade-up">
          <div class="msg-head">
            <span class="msg-name">{{ m.nickname }}</span>
            <span class="msg-time">{{ (m.createdAt || '').replace('T', ' ').slice(0, 16) }}</span>
          </div>
          <div class="msg-content">{{ m.content }}</div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.post-bg { height: 18rem; position: relative; overflow: hidden;
  background: radial-gradient(ellipse 55% 85% at 15% 10%, rgba(66,90,239,.35), transparent 62%),
              radial-gradient(ellipse 50% 80% at 85% 12%, rgba(234,188,189,.5), transparent 60%),
              linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%); }
#post-info { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.post-title { font-size: 1.8rem; font-weight: 700; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.post-meta-original { background: var(--anzhiyu-theme); color: #fff; padding: 1px 12px; border-radius: 50px; font-size: .8rem; }
#post-meta .meta-firstline { opacity: .9; font-size: .85rem; }
h2 { font-size: 1.2rem; margin: 1.6rem 0 .8rem; position: relative; padding-left: 1.35rem; }
h2::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: ccc 1.6s linear infinite; }
@keyframes ccc { 0% { transform: rotate(0); } to { transform: rotate(-1turn); } }
.form { display: flex; flex-direction: column; gap: 10px; max-width: 560px; }
.form input, .form textarea { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 10px 14px; font: inherit; font-size: .92rem; color: var(--anzhiyu-fontcolor); outline: none; resize: vertical; }
.form input:focus, .form textarea:focus { border-color: var(--anzhiyu-theme); }
.form button { align-self: flex-start; border: none; background: var(--anzhiyu-theme); color: #fff; border-radius: var(--anzhiyu-radius); padding: 9px 26px; cursor: pointer; font: inherit; font-size: .9rem; }
.form button:hover { background: var(--anzhiyu-hover); }
.form-msg { color: var(--anzhiyu-gray); font-size: .85rem; margin-top: 8px; }
.msg-card { padding: 12px 6px; border-bottom: 1px dashed var(--anzhiyu-card-border); }
.msg-card:last-child { border-bottom: none; }
.msg-head { display: flex; justify-content: space-between; align-items: baseline; }
.msg-name { font-weight: 700; color: var(--anzhiyu-blue); }
.msg-time { color: var(--anzhiyu-gray); font-size: .78rem; }
.msg-content { margin-top: 4px; color: var(--anzhiyu-secondary); white-space: pre-wrap; word-break: break-word; }
</style>
