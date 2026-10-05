<script setup>
// 检测 flag 命中统计（近 7 天 Top-N 横向条形）。
// 数据源：/api/v1/admin/ipguard/summary 的 flag_stats（见 docs/security-fingerprint-spec.md P0-4）。
// P1/P2 的新检测先"只记录不计分"灰度上线，假阳性率就由这张卡观察。
import { computed } from 'vue'
import { flagInfo } from '../flagLabels'

const props = defineProps({
  stats: { type: Array, default: () => [] },
  active: { type: String, default: '' }
})
const emit = defineEmits(['select'])

const W = 420   // 条形绘图区宽度（SVG viewBox），行高 24
const ROW = 24
const BAR = 170

const maxHits = computed(() => Math.max(1, ...props.stats.map(s => s.hits || 0)))
const height = computed(() => Math.max(1, props.stats.length) * ROW + 8)
const barW = s => Math.max(2, Math.round((s.hits || 0) / maxHits.value * BAR))
const isActive = key => props.active === key
</script>

<template>
  <div class="card">
    <h3>🧪 检测命中统计 <span class="sub">近 7 天 Top-15 · 点击条目联动过滤下方指纹</span></h3>
    <div v-if="!stats.length" class="empty">
      数据积累中：暂未收到携带检测 flag 的指纹上报
      <span class="sub">（检测项随各阶段灰度上线，先只记录不计分）</span>
    </div>
    <svg v-else :viewBox="`0 0 ${W} ${height}`" :height="height" class="bars" role="img">
      <g v-for="(s, i) in stats" :key="s.key" :transform="`translate(0, ${i * ROW + 4})`"
         class="row" :class="{ active: isActive(s.key) }" @click="emit('select', isActive(s.key) ? '' : s.key)">
        <title>{{ flagInfo(s.key)[1] || s.key }}</title>
        <text x="0" y="12" class="label">{{ flagInfo(s.key)[0] }}</text>
        <rect x="176" y="3" :width="barW(s)" height="12" rx="3" class="bar" />
        <text :x="176 + barW(s) + 6" y="12" class="num">{{ s.hits }} 次 / {{ s.affected_fps }} 指纹</text>
      </g>
    </svg>
  </div>
</template>

<style scoped>
h3 { font-size: 1rem; margin: 0 0 .6rem; color: var(--anzhiyu-secondary); }
.sub { color: var(--anzhiyu-gray); font-size: .78rem; font-weight: 400; }
.card { background: var(--anzhiyu-card-bg); border-radius: var(--anzhiyu-radius); box-shadow: var(--card-box-shadow); padding: 1rem 1.3rem; margin-bottom: 1rem; overflow-x: auto; }
.empty { color: var(--anzhiyu-gray); padding: 10px 2px; }
.bars { display: block; width: 100%; min-width: 420px; }
.row { cursor: pointer; }
.row:hover .bar { fill: var(--anzhiyu-theme); }
.row.active .bar { fill: var(--anzhiyu-red); }
.row.active .label { fill: var(--anzhiyu-red); font-weight: 600; }
.label { font-size: 11px; fill: var(--anzhiyu-fontcolor); }
.num { font-size: 10px; fill: var(--anzhiyu-gray); }
.bar { fill: var(--anzhiyu-theme-op); transition: fill .15s; }
</style>
