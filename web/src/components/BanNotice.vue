<script setup>
import { ref, computed } from 'vue'

const props = defineProps({
  show: {
    type: Boolean,
    default: false
  },
  banInfo: {
    type: Object,
    default: () => ({
      reason: '检测到异常访问行为',
      expiresAt: null,
      strikes: 1,
      ip: ''
    })
  }
})

const emit = defineEmits(['close', 'appeal'])

const remainingTime = computed(() => {
  if (!props.banInfo.expiresAt) return '未知'
  
  const now = new Date().getTime()
  const expires = new Date(props.banInfo.expiresAt).getTime()
  const diff = expires - now
  
  if (diff <= 0) return '已过期'
  
  const hours = Math.floor(diff / (1000 * 60 * 60))
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))
  
  if (hours > 0) {
    return `${hours} 小时 ${minutes} 分钟`
  } else {
    return `${minutes} 分钟`
  }
})

const banLevel = computed(() => {
  if (props.banInfo.strikes >= 3) return 'severe'
  if (props.banInfo.strikes === 2) return 'warning'
  return 'notice'
})

const handleAppeal = () => {
  emit('appeal')
}
</script>

<template>
  <Transition name="slide-down">
    <div v-if="show" class="ban-notice" :class="'ban-level-' + banLevel">
      <div class="ban-notice-container">
        <div class="ban-icon">
          <span v-if="banLevel === 'severe'">🚫</span>
          <span v-else-if="banLevel === 'warning'">⚠️</span>
          <span v-else>🔒</span>
        </div>
        
        <div class="ban-content">
          <h3 class="ban-title">
            {{ banLevel === 'severe' ? '访问已被限制' : banLevel === 'warning' ? '访问受限警告' : '安全提示' }}
          </h3>
          
          <div class="ban-reason">
            <strong>原因：</strong>{{ banInfo.reason }}
          </div>
          
          <div class="ban-details">
            <div class="ban-detail-item">
              <span class="detail-label">封禁时长：</span>
              <span class="detail-value">{{ remainingTime }}</span>
            </div>
            <div class="ban-detail-item">
              <span class="detail-label">累计次数：</span>
              <span class="detail-value">第 {{ banInfo.strikes }} 次</span>
            </div>
            <div class="ban-detail-item" v-if="banInfo.ip">
              <span class="detail-label">来源 IP：</span>
              <span class="detail-value">{{ banInfo.ip }}</span>
            </div>
          </div>
          
          <div class="ban-tips">
            <p v-if="banLevel === 'severe'">
              您的访问已被多次限制。请确保您的行为符合网站使用规范，避免使用自动化工具或进行恶意操作。
            </p>
            <p v-else-if="banLevel === 'warning'">
              检测到异常访问模式。请等待限制解除后正常访问，避免继续触发安全规则。
            </p>
            <p v-else>
              检测到短时间内频繁访问。请稍后再试，正常访问不会被限制。
            </p>
          </div>
          
          <div class="ban-actions">
            <button class="btn-appeal" @click="handleAppeal">
              申诉解封
            </button>
            <a href="/privacy" class="btn-link" target="_blank">
              查看隐私政策
            </a>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.ban-notice {
  position: fixed;
  top: 80px;
  left: 50%;
  transform: translateX(-50%);
  width: 90%;
  max-width: 600px;
  z-index: 9998;
  animation: shake 0.5s ease-in-out;
}

@keyframes shake {
  0%, 100% { transform: translateX(-50%) translateY(0); }
  10%, 30%, 50%, 70%, 90% { transform: translateX(-50%) translateY(-5px); }
  20%, 40%, 60%, 80% { transform: translateX(-50%) translateY(5px); }
}

.ban-notice-container {
  background: var(--anzhiyu-card-bg);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
  display: flex;
  padding: 24px;
  border-left: 4px solid var(--ban-color);
}

.ban-level-notice {
  --ban-color: var(--anzhiyu-yellow);
}

.ban-level-warning {
  --ban-color: var(--anzhiyu-orange);
}

.ban-level-severe {
  --ban-color: var(--anzhiyu-red);
}

.ban-icon {
  font-size: 48px;
  margin-right: 20px;
  flex-shrink: 0;
}

.ban-content {
  flex: 1;
}

.ban-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--anzhiyu-fontcolor);
  margin: 0 0 12px 0;
}

.ban-reason {
  font-size: 15px;
  color: var(--anzhiyu-fontcolor);
  margin-bottom: 16px;
  padding: 12px;
  background: var(--anzhiyu-secondbg);
  border-radius: 6px;
}

.ban-reason strong {
  color: var(--ban-color);
}

.ban-details {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.ban-detail-item {
  font-size: 14px;
}

.detail-label {
  color: var(--anzhiyu-gray);
  margin-right: 4px;
}

.detail-value {
  color: var(--anzhiyu-fontcolor);
  font-weight: 600;
}

.ban-tips {
  font-size: 14px;
  color: var(--anzhiyu-secondtext);
  line-height: 1.6;
  margin-bottom: 20px;
  padding: 12px;
  background: var(--anzhiyu-theme-op);
  border-radius: 6px;
}

.ban-tips p {
  margin: 0;
}

.ban-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.btn-appeal {
  padding: 10px 20px;
  background: var(--anzhiyu-main);
  color: white;
  border: none;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s ease;
}

.btn-appeal:hover {
  background: var(--anzhiyu-main-op);
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.btn-link {
  font-size: 14px;
  color: var(--anzhiyu-main);
  text-decoration: none;
  transition: color 0.3s ease;
}

.btn-link:hover {
  color: var(--anzhiyu-theme);
  text-decoration: underline;
}

.slide-down-enter-active,
.slide-down-leave-active {
  transition: all 0.3s ease;
}

.slide-down-enter-from {
  opacity: 0;
  transform: translateX(-50%) translateY(-20px);
}

.slide-down-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(-20px);
}

@media (max-width: 768px) {
  .ban-notice {
    top: 60px;
    width: 95%;
  }

  .ban-notice-container {
    flex-direction: column;
    padding: 20px;
  }

  .ban-icon {
    font-size: 40px;
    margin-right: 0;
    margin-bottom: 12px;
    text-align: center;
  }

  .ban-title {
    font-size: 18px;
  }

  .ban-details {
    grid-template-columns: 1fr;
  }

  .ban-actions {
    flex-direction: column;
    width: 100%;
  }

  .btn-appeal {
    width: 100%;
  }
}
</style>
