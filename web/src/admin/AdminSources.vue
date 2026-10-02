<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const sources = ref([])
const msg = ref('')
const loading = ref(true)
const form = ref({ id: '', name: '', kind: 'rss', tier: 'T2', url: '', intervalMinutes: 120 })
const kinds = ['rss', 'json_api', 'web_list', 'hacker_news', 'github_search', 'github_trending', 'script']

async function load() {
  loading.value = true
  const d = await api.get('/api/v1/sources')
  sources.value = d.items || []
  loading.value = false
}
onMounted(load)

async function add() {
  msg.value = '保存中…'
  try {
    await api.post('/api/v1/admin/sources', form.value)
    msg.value = `已保存：${form.value.id}`
    form.value = { id: '', name: '', kind: 'rss', tier: 'T2', url: '', intervalMinutes: 120 }
    await load()
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
async function del(id) {
  if (!confirm(`删除信源 ${id}？`)) return
  try {
    await api.del(`/api/v1/admin/sources/${encodeURIComponent(id)}`)
    msg.value = `已删除 ${id}`
    await load()
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
async function test(id) {
  const s = sources.value.find((x) => x.id === id) || {}
  msg.value = `试抓 ${id} …`
  try {
    const d = await api.post('/api/v1/admin/sources/test', {
      id, name: s.name, kind: s.kind, tier: s.tier, url: (s.config && s.config.url) || ''
    })
    msg.value = d.ok
      ? `试抓成功，共 ${d.count} 条，预览: ${(d.preview || []).map((p) => p.title).join(' / ')}`
      : `试抓失败: ${d.error}`
  } catch (e) {
    msg.value = '失败: ' + e.message
  }
}
</script>

<template>
  <div class="page-enter">
    <h2>🛠 信源管理</h2>
    <div v-if="msg" class="msg card">{{ msg }}</div>

    <div class="card">
      <h3>新增 / 更新信源</h3>
      <div class="form-grid">
        <input v-model="form.id" placeholder="id（如 rss-xxx）">
        <input v-model="form.name" placeholder="名称">
        <select v-model="form.kind">
          <option v-for="k in kinds" :key="k" :value="k">{{ k }}</option>
        </select>
        <select v-model="form.tier"><option value="T1">T1 官方</option><option value="T2">T2 媒体</option></select>
        <input v-model="form.url" placeholder="URL（feed / 接口 / 页面）" class="wide">
        <button class="primary" @click="add">保存</button>
      </div>
      <div class="desc">信源 config 可配 fulltext=1 开启全文抓取；kind=script 的信源通过 githubhot push 写入。</div>
    </div>

    <div class="card">
      <div v-if="loading" class="loading">加载中 </div>
      <table v-if="!loading && sources.length">
        <thead><tr><th>ID</th><th>名称</th><th>种类 / 分级</th><th>间隔</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="s in sources" :key="s.id">
            <td><code>{{ s.id }}</code></td>
            <td>{{ s.name }}</td>
            <td><span class="chip">{{ s.kind }}</span> {{ s.tier }}</td>
            <td class="num">{{ s.config && s.config.fulltext === '1' ? s.tier : s.tier }}</td>
            <td><span class="badge" :class="s.enabled ? 'okbadge' : 'offbadge'">{{ s.enabled ? '启用' : '停用' }}</span></td>
            <td>
              <button class="op" @click="test(s.id)">试抓</button>
              <button class="op danger" @click="del(s.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="empty">无信源</div>
    </div>
  </div>
</template>

<style scoped>
h2, h3 { font-size: 1.15rem; margin: 1.3rem 0 .7rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; margin-bottom: 1rem; overflow-x: auto; }
.form-grid { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
input, select { background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 8px 12px; font: inherit; font-size: .9rem; color: var(--anzhiyu-fontcolor); }
.wide { flex: 2; min-width: 220px; }
button { border: 1px solid var(--anzhiyu-card-border); background: var(--anzhiyu-background); border-radius: var(--anzhiyu-radius); padding: 8px 16px; font: inherit; font-size: .88rem; cursor: pointer; }
button.primary { background: var(--anzhiyu-theme); color: #fff; border: none; }
button:hover { border-color: var(--anzhiyu-hover); color: var(--anzhiyu-hover); }
button.primary:hover { color: #fff; }
.op { padding: 4px 12px; font-size: .82rem; }
.op.danger:hover { border-color: var(--anzhiyu-red); color: var(--anzhiyu-red); }
.msg { padding: 9px 14px; font-size: .88rem; color: var(--anzhiyu-secondary); }
.desc { color: var(--anzhiyu-gray); font-size: .8rem; }
.badge { padding: 1px 10px; border-radius: 50px; font-size: .74rem; }
.okbadge { background: #238636; color: #fff; }
.offbadge { background: var(--anzhiyu-theme-op); color: #a8766f; }
</style>
