<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../lib/api'

const usage = ref(null)
const loading = ref(true)

onMounted(async () => {
  usage.value = await api.get('/api/v1/admin/usage')
  loading.value = false
})
const pct = () => usage.value.budgetTokensPerDay ? Math.min(100, Math.round(usage.value.todayTotalTokens / usage.value.budgetTokensPerDay * 100)) : 0
</script>

<template>
  <div class="page-enter">
    <h2>💰 Token 用量（近 24h）</h2>
    <div v-if="loading" class="loading">加载中 </div>
    <template v-if="usage">
      <div class="card stat-row">
        <div class="stat"><div class="v">{{ usage.todayPromptTokens.toLocaleString() }}</div><div class="k">输入 tokens</div></div>
        <div class="stat"><div class="v">{{ usage.todayCompletionTokens.toLocaleString() }}</div><div class="k">输出 tokens</div></div>
        <div class="stat"><div class="v hot">{{ usage.todayTotalTokens.toLocaleString() }}</div><div class="k">合计</div></div>
        <div class="stat"><div class="v">¥{{ usage.estimatedCostToday.toFixed(4) }}</div><div class="k">估算成本</div></div>
        <div class="stat">
          <div class="v">{{ usage.budgetExceeded ? '已熔断' : (usage.budgetTokensPerDay ? '正常' : '未启用') }}</div>
          <div class="k">预算状态</div>
        </div>
      </div>
      <div class="card">
        <div class="budgetbar"><div :style="{ width: pct() + '%' }"></div></div>
        <div class="meta">每日预算 {{ usage.budgetTokensPerDay.toLocaleString() }} tokens（0 = 不熔断），已用 {{ pct() }}%</div>
        <table>
          <thead><tr><th>阶段</th><th>调用次数</th><th>输入</th><th>输出</th></tr></thead>
          <tbody>
            <tr v-for="p in usage.byPhase" :key="p.phase">
              <td><span class="chip">{{ p.phase }}</span></td>
              <td class="num">{{ p.calls }}</td>
              <td class="num">{{ p.promptTokens.toLocaleString() }}</td>
              <td class="num">{{ p.completionTokens.toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <h2>📅 近 7 日</h2>
      <div class="card">
        <table>
          <thead><tr><th>日期(UTC)</th><th>输入</th><th>输出</th><th>合计</th></tr></thead>
          <tbody>
            <tr v-for="d in usage.days" :key="d.day">
              <td>{{ d.day }}</td>
              <td class="num">{{ d.promptTokens.toLocaleString() }}</td>
              <td class="num">{{ d.completionTokens.toLocaleString() }}</td>
              <td class="num hot">{{ (d.promptTokens + d.completionTokens).toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style scoped>
h2 { font-size: 1.2rem; margin: 1.4rem 0 .8rem; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1.1rem 1.4rem; margin-bottom: 1rem; overflow-x: auto; }
.stat-row { display: flex; gap: 10px; flex-wrap: wrap; }
.stat { flex: 1; min-width: 120px; text-align: center; padding: 10px; background: var(--anzhiyu-background); border-radius: var(--anzhiyu-radius); }
.stat .v { font-weight: 700; font-size: 1.15rem; }
.stat .v.hot { color: var(--anzhiyu-hover); }
.stat .k { color: var(--anzhiyu-gray); font-size: .78rem; }
.budgetbar { height: 10px; background: var(--anzhiyu-background); border-radius: 6px; overflow: hidden; margin-bottom: 8px; }
.budgetbar div { height: 100%; background: linear-gradient(90deg, var(--anzhiyu-theme), var(--anzhiyu-hover)); border-radius: 6px; }
.meta { color: var(--anzhiyu-gray); font-size: .82rem; margin-bottom: 10px; }
.num { font-variant-numeric: tabular-nums; }
.hot { color: var(--anzhiyu-hover); font-weight: 700; }
</style>
