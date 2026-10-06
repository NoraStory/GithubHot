<script setup>
import { ref, onMounted, watch } from 'vue'

const props = defineProps({
  show: {
    type: Boolean,
    default: false
  },
  stage: {
    type: String,
    default: 'initializing' // initializing, collecting, reporting, complete, error
  }
})

const progress = ref(0)
const message = ref('')
const icon = ref('⏳')

const stages = {
  initializing: { progress: 10, message: '正在初始化安全检测...', icon: '⏳' },
  collecting: { progress: 40, message: '正在采集设备指纹...', icon: '🔍' },
  analyzing: { progress: 60, message: '正在分析行为特征...', icon: '🧠' },
  reporting: { progress: 80, message: '正在验证身份...', icon: '📡' },
  complete: { progress: 100, message: '验证完成', icon: '✅' },
  error: { progress: 0, message: '验证失败，请刷新重试', icon: '❌' }
}

watch(() => props.stage, (newStage) => {
  const stageData = stages[newStage] || stages.initializing
  progress.value = stageData.progress
  message.value = stageData.message
  icon.value = stageData.icon
}, { immediate: true })

onMounted(() => {
  // 初始化时立即设置状态
  const stageData = stages[props.stage] || stages.initializing
  progress.value = stageData.progress
  message.value = stageData.message
  icon.value = stageData.icon
})
</script>

<template>
  <Transition name="fade">
    <div v-if="show" class="fingerprint-loader-overlay">
      <div class="fingerprint-loader-card">
        <div class="loader-icon">{{ icon }}</div>
        <div class="loader-message">{{ message }}</div>
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: progress + '%' }"></div>
        </div>
        <div class="progress-text">{{ progress }}%</div>
        <div class="loader-hint">
          这是一次性安全验证，完成后不再打扰
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.fingerprint-loader-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}

.fingerprint-loader-card {
  background: var(--anzhiyu-card-bg);
  border: 1px solid var(--anzhiyu-card-border);
  border-radius: 16px;
  padding: 40px 48px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  max-width: 400px;
  width: 90%;
  text-align: center;
}

.loader-icon {
  font-size: 48px;
  margin-bottom: 20px;
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.1);
    opacity: 0.8;
  }
}

.loader-message {
  font-size: 18px;
  font-weight: 600;
  color: var(--anzhiyu-fontcolor);
  margin-bottom: 24px;
}

.progress-bar {
  width: 100%;
  height: 8px;
  background: var(--anzhiyu-secondbg);
  border-radius: 4px;
  overflow: hidden;
  margin-bottom: 12px;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--anzhiyu-main), var(--anzhiyu-theme));
  border-radius: 4px;
  transition: width 0.3s ease;
}

.progress-text {
  font-size: 14px;
  color: var(--anzhiyu-gray);
  margin-bottom: 16px;
}

.loader-hint {
  font-size: 13px;
  color: var(--anzhiyu-gray);
  opacity: 0.7;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

@media (max-width: 768px) {
  .fingerprint-loader-card {
    padding: 32px 24px;
  }

  .loader-icon {
    font-size: 40px;
  }

  .loader-message {
    font-size: 16px;
  }
}
</style>
