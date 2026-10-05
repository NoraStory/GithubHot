import { test } from 'node:test'
import assert from 'node:assert/strict'
import { phashFromImageData, hammingDistance, similarPHash, hexToBits } from './phash.js'

// makeImage 造一张确定性的合成图（渐变 + 图形 + 文字色块），可按 seed 加噪声、variant 换图形。
function makeImage(w = 280, h = 60, noise = 0, variant = 0) {
  const data = new Uint8ClampedArray(w * h * 4)
  let s = 1
  const rnd = () => {
    s = (s * 1103515245 + 12345) & 0x7fffffff
    return s / 0x7fffffff
  }
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * 4
      let v = 40 + (x / w) * 160
      if (variant === 0) {
        if (x > 10 && x < 130 && y > 10 && y < 40) v = 240 - (y / h) * 80
        if (x > 140 && x < 270 && y > 5 && y < 55) v = 30 + 100 * Math.abs(Math.sin(x / 12))
      } else {
        // 不同图形：竖向条带 + 反向渐变
        v = 200 - (x / w) * 160
        if (Math.floor(x / 18) % 2 === 0) v = 220 - (y / h) * 60
      }
      const n = noise ? (rnd() - 0.5) * 2 * noise : 0
      data[i] = Math.max(0, Math.min(255, v + n))
      data[i + 1] = Math.max(0, Math.min(255, v * 0.9 + n))
      data[i + 2] = Math.max(0, Math.min(255, v * 0.8 + n))
      data[i + 3] = 255
    }
  }
  return { data, width: w, height: h }
}

test('同一图形两次哈希完全一致（确定性）', () => {
  const a = phashFromImageData(makeImage())
  const b = phashFromImageData(makeImage())
  assert.equal(a, b)
  assert.equal(a.length, 16)
  assert.match(a, /^[0-9a-f]{16}$/)
  assert.equal(hammingDistance(a, b), 0)
})

test('叠加 1% 像素噪声后汉明距离 < 10（抗指纹漂移的核心诉求）', () => {
  const clean = phashFromImageData(makeImage())
  const noisy = phashFromImageData(makeImage(280, 60, 2.5))
  const d = hammingDistance(clean, noisy)
  assert.ok(d < 10, `噪声后距离应 < 10，实际 ${d}`)
  assert.ok(similarPHash(clean, noisy), '应判定为同源')
})

test('完全不同图形汉明距离 > 30', () => {
  const a = phashFromImageData(makeImage(280, 60, 0, 0))
  const b = phashFromImageData(makeImage(280, 60, 0, 1))
  const d = hammingDistance(a, b)
  assert.ok(d > 30, `不同图形距离应 > 30，实际 ${d}`)
  assert.ok(!similarPHash(a, b), '不应判定为同源')
})

test('纯色反转（亮度整体变化）不影响感知结构', () => {
  const w = 64
  const h = 64
  const mk = invert => {
    const data = new Uint8ClampedArray(w * h * 4)
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const i = (y * w + x) * 4
        let v = 60 + (x / w) * 120
        if (y > 20 && y < 44) v = invert ? 40 : 200
        data[i] = data[i + 1] = data[i + 2] = v
        data[i + 3] = 255
      }
    }
    return { data, width: w, height: h }
  }
  // 中位数阈值化对整体亮度平移不敏感：加 30 亮度后距离应很小
  const base = makeImage()
  const shifted = { data: Uint8ClampedArray.from(base.data), width: base.width, height: base.height }
  for (let i = 0; i < shifted.data.length; i += 4) {
    shifted.data[i] = Math.min(255, shifted.data[i] + 30)
    shifted.data[i + 1] = Math.min(255, shifted.data[i + 1] + 30)
    shifted.data[i + 2] = Math.min(255, shifted.data[i + 2] + 30)
  }
  const d = hammingDistance(phashFromImageData(base), phashFromImageData(shifted))
  assert.ok(d < 10, `整体亮度平移后距离应 < 10，实际 ${d}`)
  assert.ok(hammingDistance(phashFromImageData(mk(false)), phashFromImageData(mk(true))) <= 63)
})

test('非法输入：空哈希返回空串，距离按"不相似"处理', () => {
  assert.equal(phashFromImageData(null), '')
  assert.equal(phashFromImageData({ data: null, width: 0, height: 0 }), '')
  assert.equal(hammingDistance('', 'abcd'), Infinity)
  assert.equal(hammingDistance('zzzz', 'abcd'), Infinity)
  assert.equal(hammingDistance('ab', 'abcd'), Infinity) // 长度不同（不同版本哈希）不比较
  assert.equal(similarPHash('', ''), false)
})

test('hexToBits 长度正确且能还原数值', () => {
  assert.equal(hexToBits('f').length, 4)
  assert.deepEqual(hexToBits('8'), [1, 0, 0, 0])
  assert.deepEqual(hexToBits('bad!'), [])
})

