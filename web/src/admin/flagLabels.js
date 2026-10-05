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
  'app-installer': ['非可信安装渠道', 'APP 安装来源不在可信渠道列表'],
  // P1 干净环境对照（fp/cleanEnv.js）
  'fpb_canvas_diverge': ['画布对照分歧', 'Worker 内 OffscreenCanvas 与主线程渲染像素不一致：Canvas API 被 patch'],
  'fpb_iframe_diverge': ['iframe 参照分歧', 'about:blank iframe 的 navigator/screen 取值与主框架不一致'],
  'fpb_audio_diverge': ['音频对照分歧', '同一 chirp 两次渲染或两条读取路径结果不一致'],
  'fpb_canvas_check_unsupported': ['画布对照不可用', '环境不支持 Worker/OffscreenCanvas（不计分）'],
  'fpb_iframe_check_unsupported': ['iframe 参照不可用', '无法创建同源参照 iframe（不计分）'],
  'fpb_audio_check_unsupported': ['音频对照不可用', '无 OfflineAudioContext（不计分）'],
  // P1 能力—声明核验（fp/claims.js）
  'fpb_gpu_claim_mismatch': ['显卡声明与能力不符', 'UNMASKED_RENDERER 声称的档位高于实测纹理上限'],
  'fpb_gpu_software': ['软件光栅化', 'SwiftShader/llvmpipe 等软件渲染（无头/虚拟机常见）'],
  'fpb_gpu_check_unsupported': ['GPU 核验不可用', '取不到 WebGL 上下文（不计分）'],
  'fpb_font_claim_mismatch': ['字体枚举可疑', 'Canvas 与 DOM 两条文本度量路径结论冲突'],
  'fpb_font_check_unsupported': ['字体核验不可用', 'Canvas/DOM 度量不可用（不计分）'],
  'fpb_cores_claim_mismatch': ['核数声明与算力不符', 'hardwareConcurrency 与微基准耗时不在同一档（低置信）'],
  // P1 JS 拦截取证（fp/forensics.js）
  'fpb_native_fn_tamper': ['原生函数被包装', '热点函数缺少 [native code] 或名字/形参不符'],
  'fpb_stack_version_mismatch': ['错误栈与 UA 内核不符', 'Error.stack 格式与 UA 声称的浏览器内核不一致'],
  'fpb_stack_unavailable': ['错误栈不可用', '取不到 Error.stack（不计分）']
}

// 带前缀的 flag（key 里含冒号或统一前缀）→ 中文名映射
const PREFIX_LABELS = {
  'fpb_getter_patched:': ['取值器被改写', '目标属性的 getter 不是原生实现（JS 层 patch 实证）'],
  'fpb_getter_timing:': ['取值器耗时离群', '读取耗时显著高于同类原生属性（弱信号，仅记录）'],
  'botd_': ['BotD 命中', 'FingerprintJS BotD 自动化检测命中']
}

// flagInfo 返回 [中文名, 说明]；未登记的 key 回退为 [key, '']。
export const flagInfo = f => {
  if (FLAG_LABELS[f]) return FLAG_LABELS[f]
  for (const [prefix, info] of Object.entries(PREFIX_LABELS)) {
    if (f.startsWith(prefix)) return [info[0] + '：' + f.slice(prefix.length), info[1]]
  }
  return [f, '']
}
