<script setup>
// 子页横幅（post-header 风格：240px 渐变封面 + 标题，AnZhiYu 子页同构）
defineProps({ title: String, subtitle: String })
</script>

<template>
  <div class="mini-banner">
    <div class="inner">
      <h1 class="title">{{ title }}</h1>
      <div class="subtitle">{{ subtitle }}</div>
    </div>
  </div>
</template>

<style scoped>
.mini-banner { position: relative; height: 240px; overflow: hidden;
  background:
    radial-gradient(ellipse 55% 85% at 12% 8%, rgba(66,90,239,.35), transparent 62%),
    radial-gradient(ellipse 50% 80% at 88% 12%, rgba(234,188,189,.5), transparent 60%),
    linear-gradient(160deg, #66717f 0%, #4c586f 48%, #3d4a63 100%);
}
.inner { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; color: #fff; text-align: center; }
.title { font-size: 2rem; font-weight: 700; letter-spacing: 2px; text-shadow: 0 3px 14px rgba(0,0,0,.3); }
.subtitle { margin-top: 8px; font-size: .95rem; opacity: .9; }
@media (max-width: 768px) { .mini-banner { height: 190px; } .title { font-size: 1.5rem; } }
</style>
