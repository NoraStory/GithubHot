// 开发者工具屏蔽（威慑性措施，提高普通用户滥用 F12 的门槛）。
// 注意：这是客户端防护，挡不住真懂行的攻击者；真正的防线在服务端 IP 防护。
// 管理端路径（/admin）放行，不能妨碍管理员自己调试。
export function installDevtoolsGuard() {
  if (location.pathname.startsWith('/admin')) return

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
      console.log('开发者工具已被禁用')
    },
    true // 捕获阶段拦截，尽量早于页面自身 handler
  )
}
