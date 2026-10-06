// 开发者工具被动检测（威慑性 + 行为记录）。
// 注意：这是客户端防护，挡不住真懂行的攻击者；真正的防线在服务端 IP 防护。
// 管理端路径（/admin）放行，不能妨碍管理员自己调试。

let devtoolsDetected = false
let reportCooldown = false

// 被动检测：窗口尺寸差异（DevTools 打开时，innerWidth/Height 会变小）
function checkWindowSizeDiff() {
  const widthDiff = window.outerWidth - window.innerWidth
  const heightDiff = window.outerHeight - window.innerHeight
  // DevTools 侧边停靠通常占 300px+，底部停靠占 200px+
  const threshold = 160
  return widthDiff > threshold || heightDiff > threshold
}

// 被动检测：console 性能（DevTools 打开时 console 操作变慢）
function checkConsolePerf() {
  const start = performance.now()
  for (let i = 0; i < 50; i++) {
    console.clear()
  }
  const elapsed = performance.now() - start
  // DevTools 关闭时 < 1ms，打开时通常 > 5ms
  return elapsed > 5
}

// 上报检测结果到服务端（用于行为分析，不立即封禁）
async function reportDevtoolsDetection() {
  if (reportCooldown) return
  reportCooldown = true
  setTimeout(() => { reportCooldown = false }, 60000) // 1分钟冷却

  try {
    await fetch('/api/v1/security/devtools-detected', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ua: navigator.userAgent,
        timestamp: Date.now()
      })
    })
  } catch {
    // 静默失败，不影响用户体验
  }
}

// 被动检测循环（降低频率到 5 秒，减少性能影响）
function startPassiveDetection() {
  setInterval(() => {
    const detected = checkWindowSizeDiff() || checkConsolePerf()
    
    if (detected && !devtoolsDetected) {
      devtoolsDetected = true
      console.log('%c⚠️ 检测到开发者工具', 'font-size: 16px; color: #f59e0b;')
      console.log('%c服务端已记录此行为，仅用于安全分析', 'font-size: 12px; color: #6b7280;')
      reportDevtoolsDetection()
    } else if (!detected && devtoolsDetected) {
      devtoolsDetected = false
    }
  }, 5000)
}

// 键盘拦截（保留，作为基础威慑）
function installKeyboardBlock() {
  const blocked = (e) => {
    // F12 或 Ctrl/Cmd+Shift+I / J / C
    if (e.key === 'F12') return true
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && ['I', 'J', 'C', 'i', 'j', 'c'].includes(e.key)) return true
    return false
  }

  window.addEventListener(
    'keydown',
    (e) => {
      if (!blocked(e)) return
      e.preventDefault()
      e.stopPropagation()
      console.log('开发者工具快捷键已被禁用')
    },
    true // 捕获阶段拦截
  )
}

export function installDevtoolsGuard() {
  if (location.pathname.startsWith('/admin')) return

  // 1. 键盘拦截（即时生效）
  installKeyboardBlock()

  // 2. 被动检测（延迟启动，避免影响首屏加载）
  setTimeout(() => {
    startPassiveDetection()
  }, 3000)
}
