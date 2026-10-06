<template>
  <Transition name="privacy-fade">
    <div v-if="show" class="privacy-overlay" @click.self="handleDismiss">
      <div class="privacy-modal">
        <div class="privacy-header">
          <h3>🔒 隐私收集告知</h3>
          <button class="close-btn" @click="handleDismiss" aria-label="关闭">×</button>
        </div>
        
        <div class="privacy-body">
          <p class="privacy-intro">
            为了保护站点安全、防止恶意访问，我们会收集以下信息用于 IP 封禁判定：
          </p>
          
          <ul class="privacy-list">
            <li>
              <strong>浏览器指纹</strong>
              <span class="privacy-detail">Canvas/WebGL 特征、字体列表、屏幕分辨率、时区等</span>
            </li>
            <li>
              <strong>网络信息</strong>
              <span class="privacy-detail">访问 IP 地址、User-Agent、WebRTC 候选地址（可选）</span>
            </li>
            <li>
              <strong>行为特征</strong>
              <span class="privacy-detail">访问频率、路径模式、DevTools 使用情况</span>
            </li>
            <li>
              <strong>交互数据</strong>
              <span class="privacy-detail">键盘输入模式（按键间隔、输入速度）、鼠标移动轨迹（坐标序列、点击位置）</span>
            </li>
          </ul>

          <div class="privacy-usage">
            <p><strong>数据用途：</strong></p>
            <ul>
              <li>识别和封禁恶意 IP 与设备指纹</li>
              <li>检测异常访问模式（爬虫、DDoS、漏洞扫描）</li>
              <li>关联多个 IP 的相同设备（防止 IP 漂移绕过）</li>
            </ul>
          </div>

          <div class="privacy-guarantee">
            <p><strong>隐私保证：</strong></p>
            <ul>
              <li>✅ 数据仅用于安全防护，不用于广告追踪或第三方分析</li>
              <li>✅ 违规事件保留 7 天后自动清理，指纹数据保留 30 天</li>
              <li>✅ 键盘输入仅收集时序特征（按键间隔、速度），不记录具体按键内容</li>
              <li>✅ 鼠标轨迹用于识别自动化工具，不关联具体操作内容</li>
              <li>✅ 管理员可查看封禁记录，但无法关联到具体身份</li>
            </ul>
          </div>

          <div class="privacy-rights">
            <p class="privacy-note">
              继续访问即表示您同意上述信息收集。如不同意，请关闭此页面。
            </p>
          </div>
        </div>

        <div class="privacy-footer">
          <button class="btn-secondary" @click="handleLearnMore">详细说明</button>
          <button class="btn-primary" @click="handleAccept">我已知晓</button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const show = ref(false)
const STORAGE_KEY = 'privacy_notice_accepted'

onMounted(() => {
  // 检查是否已接受（30天有效期）
  const accepted = localStorage.getItem(STORAGE_KEY)
  if (!accepted) {
    // 延迟 1 秒显示，避免打断首屏加载
    setTimeout(() => {
      show.value = true
    }, 1000)
  } else {
    const acceptedTime = parseInt(accepted, 10)
    const now = Date.now()
    // 30 天后重新提示
    if (now - acceptedTime > 30 * 24 * 3600 * 1000) {
      localStorage.removeItem(STORAGE_KEY)
      setTimeout(() => {
        show.value = true
      }, 1000)
    }
  }
})

function handleAccept() {
  localStorage.setItem(STORAGE_KEY, Date.now().toString())
  show.value = false
}

function handleDismiss() {
  // 点击背景或关闭按钮，视为临时接受（会话级别）
  sessionStorage.setItem(STORAGE_KEY, Date.now().toString())
  show.value = false
}

function handleLearnMore() {
  // 可跳转到详细隐私政策页面
  window.open('/privacy-policy', '_blank')
  handleAccept()
}
</script>

<style scoped>
.privacy-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  padding: 20px;
}

.privacy-modal {
  background: #ffffff;
  border-radius: 12px;
  max-width: 600px;
  width: 100%;
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.privacy-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid #e5e7eb;
}

.privacy-header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #111827;
}

.close-btn {
  background: none;
  border: none;
  font-size: 28px;
  color: #6b7280;
  cursor: pointer;
  padding: 0;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  transition: all 0.2s;
}

.close-btn:hover {
  background: #f3f4f6;
  color: #111827;
}

.privacy-body {
  padding: 24px;
  overflow-y: auto;
  flex: 1;
}

.privacy-intro {
  margin: 0 0 16px 0;
  color: #374151;
  line-height: 1.6;
}

.privacy-list {
  list-style: none;
  padding: 0;
  margin: 0 0 20px 0;
}

.privacy-list > li {
  padding: 12px;
  background: #f9fafb;
  border-radius: 6px;
  margin-bottom: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.privacy-list strong {
  color: #111827;
  font-weight: 600;
}

.privacy-detail {
  color: #6b7280;
  font-size: 14px;
}

.privacy-usage,
.privacy-guarantee {
  margin-bottom: 20px;
}

.privacy-usage p,
.privacy-guarantee p {
  margin: 0 0 8px 0;
  font-weight: 600;
  color: #111827;
}

.privacy-usage ul,
.privacy-guarantee ul {
  margin: 0;
  padding-left: 20px;
  color: #4b5563;
  line-height: 1.7;
}

.privacy-usage ul li,
.privacy-guarantee ul li {
  margin-bottom: 4px;
}

.privacy-rights {
  padding-top: 16px;
  border-top: 1px solid #e5e7eb;
}

.privacy-note {
  margin: 0;
  color: #6b7280;
  font-size: 14px;
  line-height: 1.6;
}

.privacy-footer {
  display: flex;
  gap: 12px;
  padding: 16px 24px;
  border-top: 1px solid #e5e7eb;
  justify-content: flex-end;
}

.btn-primary,
.btn-secondary {
  padding: 10px 20px;
  border-radius: 6px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  border: none;
  font-size: 14px;
}

.btn-primary {
  background: #2563eb;
  color: white;
}

.btn-primary:hover {
  background: #1d4ed8;
}

.btn-secondary {
  background: #f3f4f6;
  color: #374151;
}

.btn-secondary:hover {
  background: #e5e7eb;
}

/* 过渡动画 */
.privacy-fade-enter-active,
.privacy-fade-leave-active {
  transition: opacity 0.3s ease;
}

.privacy-fade-enter-active .privacy-modal,
.privacy-fade-leave-active .privacy-modal {
  transition: transform 0.3s ease;
}

.privacy-fade-enter-from,
.privacy-fade-leave-to {
  opacity: 0;
}

.privacy-fade-enter-from .privacy-modal,
.privacy-fade-leave-to .privacy-modal {
  transform: scale(0.95) translateY(-20px);
}

/* 移动端适配 */
@media (max-width: 640px) {
  .privacy-modal {
    max-height: 95vh;
  }

  .privacy-header h3 {
    font-size: 18px;
  }

  .privacy-body {
    padding: 16px;
  }

  .privacy-footer {
    flex-direction: column-reverse;
  }

  .btn-primary,
  .btn-secondary {
    width: 100%;
  }
}
</style>
