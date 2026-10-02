<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api'

const route = useRoute()
const md = ref('')
const loading = ref(true)
const date = route.params.date

function renderMarkdown(src) {
  // 极简 Markdown 渲染（标题/表格行/引用/列表/粗体/链接），足够展示日报
  const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const inline = (s) => esc(s)
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
  const lines = src.split('\n')
  let html = ''
  let inTable = false
  for (const line of lines) {
    const t = line.trim()
    if (t.startsWith('|')) {
      if (/^\|[\s:|-]+\|$/.test(t)) continue
      const cells = t.split('|').slice(1, -1)
      if (!inTable) { html += '<table><tbody>'; inTable = true }
      html += '<tr>' + cells.map((c) => `<td>${inline(c.trim())}</td>`).join('') + '</tr>'
      continue
    }
    if (inTable) { html += '</tbody></table>'; inTable = false }
    if (t.startsWith('# ')) html += `<h1>${inline(t.slice(2))}</h1>`
    else if (t.startsWith('## ')) html += `<h2>${inline(t.slice(3))}</h2>`
    else if (t.startsWith('### ')) html += `<h3>${inline(t.slice(4))}</h3>`
    else if (t.startsWith('> ')) html += `<blockquote>${inline(t.slice(2))}</blockquote>`
    else if (t.startsWith('- ')) html += `<div class="li">${inline(t.slice(2))}</div>`
    else if (t.startsWith('---')) html += '<hr>'
    else if (t) html += `<p>${inline(t)}</p>`
  }
  if (inTable) html += '</tbody></table>'
  return html
}

onMounted(async () => {
  md.value = renderMarkdown(await api.raw(`/api/v1/digest/${encodeURIComponent(date)}?format=raw`))
  loading.value = false
})
</script>

<template>
  <div class="layout page-enter">
    <main id="article-container">
      <div class="card article digest">
        <div v-if="loading" class="loading">加载中 </div>
        <div class="md" v-html="md"></div>
      </div>
    </main>
  </div>
</template>

<style scoped>
.digest { max-width: 1000px; margin: 0 auto; }
.md :deep(h1) { font-size: 1.5rem; margin-bottom: .6rem; }
.md :deep(h2) { font-size: 1.25rem; margin: 1.6rem 0 .8rem; position: relative; padding-left: 1.35rem; }
.md :deep(h2)::before { content: '✽'; position: absolute; left: 0; color: #fb7061; animation: anzhiyu-rotate 1.6s linear infinite; }
.md :deep(h3) { font-size: 1.08rem; margin: 1.1rem 0 .5rem; }
.md :deep(p) { margin: .5rem 0; color: var(--anzhiyu-secondary); }
.md :deep(blockquote) { margin: .6rem 0; padding: 8px 14px; background: var(--anzhiyu-background); border-left: 3px solid var(--anzhiyu-theme); border-radius: 6px; color: var(--anzhiyu-secondary); font-size: .9rem; }
.md :deep(.li) { padding: 3px 0 3px 8px; color: var(--anzhiyu-secondary); font-size: .92rem; }
.md :deep(hr) { border: none; border-top: 1px dashed var(--anzhiyu-card-border); margin: 1rem 0; }
.md :deep(a) { color: var(--anzhiyu-blue); }
.md :deep(a:hover) { color: var(--anzhiyu-hover); }
.md :deep(td) { font-size: .88rem; }
.md :deep(code) { background: var(--anzhiyu-background); padding: 1px 6px; border-radius: 4px; font-size: .85rem; }
</style>
