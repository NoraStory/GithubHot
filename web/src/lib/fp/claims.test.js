import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  claimFlags, gpuClassFor, coresBucket, fontAvailability, countFontConflicts, GPU_CLASSES
} from './claims.js'

// ---------- GPU 能力—声明核验 ----------

test('GPU 档位匹配：按 renderer 串归入能力档', () => {
  assert.equal(gpuClassFor('ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11)').vendor, 'nvidia')
  assert.equal(gpuClassFor('ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11)').vendor, 'intel')
  assert.equal(gpuClassFor('Apple M2').vendor, 'apple')
  assert.equal(gpuClassFor('Adreno (TM) 740').vendor, 'mobile')
  assert.equal(gpuClassFor('Google SwiftShader').vendor, 'software')
  assert.equal(gpuClassFor('未知设备'), null)
  assert.equal(gpuClassFor(''), null)
})

test('GPU 声明与实测能力不符 → fpb_gpu_claim_mismatch', () => {
  // 声称 RTX（档位下限 16384）却只拿到 4096 → 改机工具典型特征
  const flags = claimFlags({
    gpuSupported: true,
    renderer: 'ANGLE (NVIDIA, NVIDIA GeForce RTX 4090)',
    maxTextureSize: 4096,
    fontsSupported: true,
    fontSampled: 10,
    fontDisagreements: 0,
    cores: 8,
    benchMs: 40
  })
  assert.deepEqual(flags, ['fpb_gpu_claim_mismatch'])
})

test('GPU 能力达标（含移动档位）→ 不误报', () => {
  const cases = [
    ['ANGLE (NVIDIA, NVIDIA GeForce GTX 1650)', 16384],
    ['ANGLE (Intel, Intel(R) UHD Graphics 620)', 16384],
    ['Apple M2', 16384],
    ['Adreno (TM) 640', 4096],
    ['未知显卡型号', 2048]
  ]
  for (const [renderer, maxTex] of cases) {
    assert.deepEqual(claimFlags({
      gpuSupported: true, renderer, maxTextureSize: maxTex,
      fontsSupported: true, fontSampled: 10, fontDisagreements: 0, cores: 8, benchMs: 40
    }), [], renderer)
  }
})

test('软件光栅化 → fpb_gpu_software（只记录，不算能力不符）', () => {
  const flags = claimFlags({
    gpuSupported: true,
    renderer: 'Google SwiftShader',
    maxTextureSize: 8192,
    softwareGPU: true,
    fontsSupported: true,
    fontSampled: 10,
    fontDisagreements: 0,
    cores: 4,
    benchMs: 90
  })
  assert.deepEqual(flags, ['fpb_gpu_software'])
})

// ---------- 字体双路径核验 ----------

test('fontAvailability：两条路径一致 → installed / absent', () => {
  assert.equal(fontAvailability(3.2, 4.1), 'installed')
  assert.equal(fontAvailability(0.1, 0), 'absent')
})

test('fontAvailability：模糊带（0.5–2px）直接跳过，不当矛盾', () => {
  assert.equal(fontAvailability(1.2, 0), 'ambiguous')
  assert.equal(fontAvailability(0, 1.8), 'ambiguous')
})

test('fontAvailability：两条路径结论相反 → conflict', () => {
  assert.equal(fontAvailability(5, 0), 'conflict')
  assert.equal(fontAvailability(0, 5), 'conflict')
})

test('countFontConflicts：≥3 个冲突才计 flag（单点舍入不背锅）', () => {
  const pairs = [
    { canvas: 5, dom: 5 },
    { canvas: 0, dom: 0 },
    { canvas: 6, dom: 0.2 },
    { canvas: 0.1, dom: 7 },
    { canvas: 1.1, dom: 0.2 }
  ]
  assert.equal(countFontConflicts(pairs), 2)
  const base = { gpuSupported: true, renderer: 'Apple M2', maxTextureSize: 16384, cores: 8, benchMs: 40 }
  assert.deepEqual(claimFlags({ ...base, fontsSupported: true, fontSampled: 10, fontDisagreements: 2 }), [])
  assert.deepEqual(claimFlags({ ...base, fontsSupported: true, fontSampled: 10, fontDisagreements: 3 }),
    ['fpb_font_claim_mismatch'])
})

test('字体探测不可用 → fpb_font_check_unsupported', () => {
  assert.deepEqual(claimFlags({ gpuSupported: true, renderer: 'Apple M2', maxTextureSize: 16384, fontsSupported: false }), ['fpb_font_check_unsupported'])
})

// ---------- 核数基准 ----------

test('核数基准只抓极端矛盾（防 CPU 节流假阳性）', () => {
  assert.equal(coresBucket(16, 300).mismatch, true)   // 高档声明 + 极慢
  assert.equal(coresBucket(2, 5).mismatch, true)      // 低档声明 + 极快
  assert.equal(coresBucket(16, 40).mismatch, false)
  assert.equal(coresBucket(4, 300).mismatch, false)   // 中档不参与判定（节流常见）
  assert.deepEqual(claimFlags({
    gpuSupported: true, renderer: 'Apple M2', maxTextureSize: 16384,
    fontsSupported: true, fontSampled: 10, fontDisagreements: 0, cores: 16, benchMs: 400
  }), ['fpb_cores_claim_mismatch'])
})

test('探测全不可用 → 各自的 *_unsupported flag（不计分）', () => {
  const flags = claimFlags({ gpuSupported: false, fontsSupported: false, cores: 0, benchMs: 0 })
  assert.deepEqual(flags, ['fpb_gpu_check_unsupported', 'fpb_font_check_unsupported'])
})

test('GPU 能力表结构完整（每条含 vendor/match/下限）', () => {
  assert.ok(Array.isArray(GPU_CLASSES) && GPU_CLASSES.length >= 5)
  for (const c of GPU_CLASSES) {
    assert.ok(c.vendor && Array.isArray(c.match) && c.match.length > 0)
    assert.equal(typeof c.minMaxTextureSize, 'number')
  }
})
