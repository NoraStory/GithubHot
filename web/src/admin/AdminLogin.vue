<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { setToken } from '../lib/api'

const route = useRoute()
const router = useRouter()
const token = ref('')
const error = ref('')
const loading = ref(false)

async function login() {
  loading.value = true
  error.value = ''
  setToken(token.value.trim())
  try {
    // 用一个真实管理端点验证令牌
    const res = await fetch('/api/v1/admin/sources/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token.value.trim() },
      body: JSON.stringify({ id: 'ping', name: 'ping', kind: 'rss', url: 'https://example.com/feed.xml', dry: true })
    })
    const d = await res.json()
    if (d.ok || !d.error) {
      router.push(route.query.redirect || '/admin/usage')
    } else {
      error.value = d.error || '令牌无效（服务端设置了 ADMIN_TOKEN）'
      localStorage.removeItem('githubhot_admin_token')
    }
  } catch (e) {
    error.value = e.message
  }
  loading.value = false
}
function etTokenRemove() { localStorage.removeItem('githubhot_admin_token') }
</script>

<template>
  <div class="login-wrap">
    <div class="card login-card">
      <h2>🔐 管理端登录</h2>
      <div class="desc">输入服务端 ADMIN_TOKEN（未设置时任意值可通过）</div>
      <input v-model="token" type="password" placeholder="ADMIN_TOKEN" @keyup.enter="login">
      <button :disabled="loading" @click="login">{{ loading ? '验证中…' : '进入控制台' }}</button>
      <div v-if="error" class="err">{{ error }}</div>
    </div>
  </div>
</template>

<style scoped>
.login-wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
.login-card { width: 380px; padding: 2rem 2.2rem; text-align: center; }
.desc { color: var(--anzhiyu-gray); font-size: .85rem; margin: 8px 0 16px; }
input { width: 100%; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 10px 14px; font: inherit; outline: none; margin-bottom: 12px; }
input:focus { border-color: var(--anzhiyu-theme); }
button { width: 100%; background: var(--anzhiyu-theme); color: #fff; border: none; border-radius: var(--anzhiyu-radius); padding: 11px; font: inherit; cursor: pointer; }
button:hover { background: var(--anzhiyu-hover); }
.err { color: var(--anzhiyu-red); font-size: .85rem; margin-top: 10px; }
</style>
