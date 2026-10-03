<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { adminLogin } from '../lib/api'

const route = useRoute()
const router = useRouter()
const password = ref('')
const remember = ref(true)
const error = ref('')
const loading = ref(false)

async function login() {
  if (!password.value) {
    error.value = '请输入密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await adminLogin(password.value, remember.value)
    router.push(route.query.redirect || '/admin/usage')
  } catch (e) {
    error.value = e.message
  }
  loading.value = false
}
</script>

<template>
  <div class="login-wrap">
    <div class="card login-card">
      <h2>🔐 管理端登录</h2>
      <div class="desc">输入管理员密码（Argon2id 加密校验，会话 12 小时有效）</div>
      <input v-model="password" type="password" placeholder="管理员密码" autocomplete="current-password" @keyup.enter="login">
      <label class="remember"><input v-model="remember" type="checkbox"> 记住我（7 天内免登录）</label>
      <button :disabled="loading" @click="login">{{ loading ? '验证中…' : '进入控制台' }}</button>
      <div v-if="error" class="err">{{ error }}</div>
    </div>
  </div>
</template>

<style scoped>
.login-wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
.login-card { width: 380px; padding: 2rem 2.2rem; text-align: center; }
.desc { color: var(--anzhiyu-gray); font-size: .85rem; margin: 8px 0 16px; }
input[type="password"] { width: 100%; background: var(--anzhiyu-background); border: 1px solid var(--anzhiyu-card-border); border-radius: var(--anzhiyu-radius); padding: 10px 14px; font: inherit; outline: none; margin-bottom: 10px; }
input[type="password"]:focus { border-color: var(--anzhiyu-theme); }
.remember { display: flex; align-items: center; gap: 6px; font-size: .82rem; color: var(--anzhiyu-gray); margin: 2px 0 14px; cursor: pointer; }
.remember input { accent-color: var(--anzhiyu-theme); }
button { width: 100%; background: var(--anzhiyu-theme); color: #fff; border: none; border-radius: var(--anzhiyu-radius); padding: 11px; font: inherit; cursor: pointer; }
button:hover { background: var(--anzhiyu-hover); }
button:disabled { opacity: .6; cursor: default; }
.err { color: var(--anzhiyu-red); font-size: .85rem; margin-top: 10px; }
</style>
