<script setup>
// P4-2 通行密钥管理页：列表 / 注册 / 删除。
import { ref, onMounted } from 'vue'
import { api, passkeyBeginRegister, listPasskeys, deletePasskey, webAuthnAvailable } from '../lib/api'

const creds = ref([])
const message = ref('')
const error = ref('')
const loading = ref(true)
const busy = ref(false)
const supported = webAuthnAvailable()

async function load() {
  loading.value = true
  try {
    const d = await listPasskeys()
    creds.value = d.credentials || []
  } catch (e) {
    error.value = e.message
  }
  loading.value = false
}

async function register() {
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const d = await passkeyBeginRegister()
    message.value = '注册成功：' + (d.credential_id || '').slice(0, 12) + '…'
    await load()
  } catch (e) {
    error.value = e.message
  }
  busy.value = false
}

async function remove(id) {
  busy.value = true
  try {
    await deletePasskey(id)
    await load()
  } catch (e) {
    error.value = e.message
  }
  busy.value = false
}

onMounted(() => { if (supported) load() })
</script>

<template>
  <div class="page">
    <h2>🔑 通行密钥（WebAuthn）</h2>
    <p class="hint">注册后可在登录页免密进入管理端。密钥绑定本设备的平台认证器（Windows Hello / 触控 ID / 安全钥匙）。</p>
    <div v-if="!supported" class="warn">当前浏览器不支持 WebAuthn（需 HTTPS 或 localhost）。</div>
    <template v-else>
      <button class="primary" :disabled="busy" @click="register">{{ busy ? '等待认证器…' : '➕ 注册本设备通行密钥' }}</button>
      <div v-if="message" class="ok">{{ message }}</div>
      <div v-if="error" class="err">{{ error }}</div>
      <div v-if="loading" class="hint">加载中…</div>
      <table v-else-if="creds.length">
        <thead><tr><th>凭据 ID</th><th>类型</th><th>签名计数</th><th>注册时间</th><th></th></tr></thead>
        <tbody>
          <tr v-for="c in creds" :key="c.id">
            <td class="mono">{{ c.credential_id.slice(0, 14) }}…</td>
            <td>{{ c.attestation_type || 'none' }}</td>
            <td>{{ c.sign_count }}</td>
            <td>{{ new Date(c.created_at).toLocaleString() }}</td>
            <td><button class="danger" :disabled="busy" @click="remove(c.id)">删除</button></td>
          </tr>
        </tbody>
      </table>
      <div v-else class="hint">尚未注册任何通行密钥。</div>
    </template>
  </div>
</template>

<style scoped>
.page h2 { margin: 0 0 8px; }
.hint { color: var(--grey9, #888); font-size: 13px; }
.warn { color: #c0392b; margin: 10px 0; }
.ok { color: #27ae60; margin: 10px 0; }
.err { color: #c0392b; margin: 10px 0; white-space: pre-wrap; }
.mono { font-family: monospace; font-size: 12px; }
.primary { background: var(--main, #425066); color: #fff; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; }
.danger { background: transparent; border: 1px solid #c0392b; color: #c0392b; border-radius: 6px; cursor: pointer; }
table { width: 100%; margin-top: 14px; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid var(--light-border, #eee); }
button:disabled { opacity: 0.5; cursor: default; }
</style>
