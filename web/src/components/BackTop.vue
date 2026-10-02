<script setup>
import { onMounted, onBeforeUnmount, ref } from 'vue'

const show = ref(false)
function onScroll() { show.value = window.scrollY > 400 }
function top() { window.scrollTo({ top: 0, behavior: 'smooth' }) }
onMounted(() => window.addEventListener('scroll', onScroll, { passive: true }))
onBeforeUnmount(() => window.removeEventListener('scroll', onScroll))
</script>

<template>
  <div class="back-top" :class="{ show }" title="返回顶部" @click="top">⬆</div>
</template>
