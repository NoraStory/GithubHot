<script setup>
// §10.5 ML 诊断页：模型卡 / PSI 表 / sidecar 状态 / 管道状态。
import { ref, onMounted } from 'vue'
import { api, webAuthnAvailable } from '../lib/api'

const data = ref(null)
const loading = ref(true)
const error = ref('')

onMounted(async () => {
  try {
    data.value = await api.get('/api/v1/admin/ml/diagnostics')
  } catch (e) { error.value = e.message }
  loading.value = false
})
</script>

<template>
  <div class="page">
    <h2>📊 ML 诊断</h2>
    <div v-if="error" class="err">{{ error }}</div>
    <div v-if="loading" class="hint">加载中…</div>
    <template v-else-if="data">
      <!-- 模型卡区 -->
      <div class="section" v-if="data.models?.length">
        <h3>模型</h3>
        <div class="cards">
          <div v-for="m in data.models" :key="m.version" class="card">
            <div class="card-title">{{ m.version }} <span class="badge" :class="m.active ? 'ok' : 'off'">{{ m.active ? 'active' : 'off' }}</span></div>
            <div class="card-body">
              AUC {{ m.metrics?.auc }} | FPR {{ m.metrics?.fpr }} | 样本 {{ m.train_size }}<br>
              训练于 {{ m.trained_at }}
            </div>
          </div>
        </div>
      </div>
      <div v-else class="hint">模型未训练（运行 scripts/ml/run_train.ps1）。</div>

      <!-- PSI 表 -->
      <div class="section" v-if="data.psi?.length">
        <h3>特征漂移（PSI 近似）</h3>
        <table>
          <thead><tr><th>特征</th><th>PSI</th><th>样本</th><th>状态</th></tr></thead>
          <tbody>
            <tr v-for="p in data.psi" :key="p.feature">
              <td>{{ p.feature }}</td>
              <td class="mono">{{ p.psi }}</td>
              <td>{{ p.n }}</td>
              <td><span class="badge" :class="p.level">{{ p.level }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- sidecar 状态 -->
      <div class="section">
        <h3>Sidecar（GNN）</h3>
        <div class="card">
          <div class="card-body">
            <span class="badge" :class="data.sidecar?.enabled ? 'ok' : 'off'">{{ data.sidecar?.enabled ? '已连接' : '未配置' }}</span>
            <span v-if="data.sidecar?.url" class="mono">{{ data.sidecar.url }}</span>
            <span v-if="!data.sidecar?.enabled" class="hint">纯离线模式（Louvain 冷启动）</span>
          </div>
        </div>
      </div>

      <!-- 管道 -->
      <div class="section">
        <h3>管道</h3>
        <div class="card"><div class="card-body mono">
          <div v-for="(v, k) in data.pipeline" :key="k">{{ k }}: {{ v }}</div>
        </div></div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page h2 { margin: 0 0 8px; }
.section { margin-top: 18px; }
.section h3 { margin: 0 0 8px; }
.hint { color: var(--grey9, #888); font-size: 13px; }
.err { color: #c0392b; margin: 10px 0; }
.mono { font-family: monospace; font-size: 12px; }
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; }
.card { background: var(--card-bg, #f5f5f5); border-radius: 8px; padding: 12px; }
.card-title { font-weight: bold; margin-bottom: 6px; }
.card-body { font-size: 13px; color: var(--grey8, #555); }
.badge { display: inline-block; padding: 1px 8px; border-radius: 4px; font-size: 12px; }
.badge.ok, .badge.green { background: #27ae6022; color: #27ae60; }
.badge.yellow { background: #f39c1222; color: #f39c12; }
.badge.red { background: #e74c3c22; color: #e74c3c; }
.badge.off { background: #99922222; color: #999; }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid var(--light-border, #eee); }
</style>
