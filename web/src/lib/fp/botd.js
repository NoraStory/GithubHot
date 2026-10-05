// P1-4 BotD（FingerprintJS，MIT）接入：现成的自动化/无头检测弹药。
// 结果展开为 botd_<botKind> 与 botd_<detector>_1 两类 flag，与服务端既有 flags 通道共用
// （表结构零改动）。灰度纪律同其它检测项：服务端默认只记录不计分。
//
// 库走动态 import：不参与主包解析，只在指纹上报时异步加载（失败即降级为零命中）。

// normalizeName BotKind / 检测器名 → flag 片段（小写 + 下划线）
export function normalizeName(s) {
  return String(s || '')
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
}

/**
 * botdFlags 把 BotD 结果展开为 flag 列表（纯函数，可单测）。
 * @param {{bot?: boolean, botKind?: string}} result detect() 的总体结论
 * @param {Record<string, {bot?: boolean, botKind?: string}>} [detections] getDetections() 的逐项结论
 * @returns {string[]}
 */
export function botdFlags(result, detections) {
  const flags = []
  if (result && result.bot) {
    flags.push('botd_' + (normalizeName(result.botKind) || 'detected'))
  }
  for (const [name, d] of Object.entries(detections || {})) {
    if (d && d.bot) flags.push('botd_' + normalizeName(name) + '_1')
  }
  return [...new Set(flags)]
}

// botdDetect 加载并执行 BotD；任何失败都降级为空命中（不抛错、不阻塞上报）。
export async function botdDetect() {
  try {
    const { load } = await import('@fingerprintjs/botd')
    const detector = await load({ monitoring: false })
    const result = detector.detect()
    let detections
    try {
      detections = typeof detector.getDetections === 'function' ? detector.getDetections() : undefined
    } catch {
      detections = undefined
    }
    return { flags: botdFlags(result, detections), raw: { result, detections } }
  } catch {
    return { flags: [], raw: { error: true } }
  }
}
