// 检测 flag 的中文说明（管理端共用：IP 下钻与 flag 命中统计面板）。
// 新增检测（P1 fpb_* / BotD botd_*）落地时在此补条目即可，两处展示自动同步。
export const FLAG_LABELS = {
  // 第三层环境核验（客户端自报）
  'navigator-webdriver': ['navigator.webdriver = true', '典型自动化驱动特征'],
  'automation-global': ['自动化框架全局变量', 'Selenium/CDP 注入痕迹'],
  'headless-ua': ['无头浏览器', 'UA 含 headless/phantom/selenium 标记'],
  'ua-platform-mismatch': ['UA 与系统矛盾', 'navigator.platform 与 UA 声明系统不一致'],
  'ua-ch-mismatch': ['Client Hints 与 UA 矛盾', 'Sec-CH-UA-Platform 与 UA 声明系统不一致'],
  'lang-tz-mismatch': ['语言与时区矛盾', '语言环境与系统时区不匹配，常见于伪造 header'],
  'no-plugins': ['桌面环境无插件', '桌面 Chrome 无插件接口，无头/精简环境特征'],
  'env-incoherent': ['环境不自洽', '客户端上报环境参数存在矛盾'],
  // APP 威胁检测（components.threat 落入 flags 的 app-* 前缀键）
  'app-root': ['Root 环境', 'APP 自检检测到设备已 Root'],
  'app-emulator': ['模拟器/云机', 'APP 自检检测到模拟器特征'],
  'app-debugger': ['调试器附加', 'APP 自检检测到调试器'],
  'app-hook': ['Hook 框架', 'APP 自检检测到 Xposed/Frida'],
  'app-installer': ['非可信安装渠道', 'APP 安装来源不在可信渠道列表']
}

// flagInfo 返回 [中文名, 说明]；未登记的 key 回退为 [key, '']。
export const flagInfo = f => FLAG_LABELS[f] || [f, '']
