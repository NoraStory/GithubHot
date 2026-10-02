<script setup>
defineProps({
  total: { type: Number, default: 0 },
  page: { type: Number, default: 1 },
  pageSize: { type: Number, default: 10 }
})
const emit = defineEmits(['change'])
</script>

<template>
  <div v-if="total > pageSize" class="pagination">
    <span class="page-item" :class="{ disabled: page === 1 }" @click="page > 1 && emit('change', page - 1)">‹</span>
    <span
      v-for="p in Math.ceil(total / pageSize)"
      :key="p"
      class="page-item"
      :class="{ active: p === page }"
      @click="p !== page && emit('change', p)"
    >{{ p }}</span>
    <span class="page-item" :class="{ disabled: page >= Math.ceil(total / pageSize) }" @click="page < Math.ceil(total / pageSize) && emit('change', page + 1)">›</span>
  </div>
</template>
