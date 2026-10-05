// P2-1 pHash（感知哈希）：Canvas 精确 SHA-256 在"驱动更新 / 抗指纹噪声"下会漂移，
// 导致同一台设备被当成新设备。pHash 对像素做 32×32 灰度 → DCT-II → 取左上 8×8 低频块 →
// 中位数二值化，得到 64bit 感知哈希：图形微改（噪声、抗指纹扰动）汉明距离仍然很小。
//
// 算法（规格 §5 P2-1 / 附录 A）：
//   1. 灰度化 0.299R+0.587G+0.114B；双线性降采样到 32×32；
//   2. 32×32 DCT-II（行/列可分离，两遍一维变换）；
//   3. 取左上 8×8 低频块，以**除去 DC 项后的中位数**为阈值二值化（DC 是整体亮度，最易变）；
//   4. 63 位有效比特按 ZigZag 无关的固定光栅序输出，末位补 0 → 64bit → hex16。
//
// 纯函数实现，用 node --test 覆盖（web/src/lib/phash.test.js）。

export const PHASH_SIDE = 32
export const PHASH_LOW = 8

// dctMatrix 预计算 32 点 DCT-II 基（一次性构造，避免每次上报重复算 cos）
const dctBasis = (() => {
  const n = PHASH_SIDE
  const m = new Float64Array(n * n)
  for (let k = 0; k < n; k++) {
    const s = k === 0 ? Math.sqrt(1 / n) : Math.sqrt(2 / n)
    for (let x = 0; x < n; x++) {
      m[k * n + x] = s * Math.cos((Math.PI * (2 * x + 1) * k) / (2 * n))
    }
  }
  return m
})()

// toGray 灰度化并双线性降采样到 32×32。
// @param {{data: Uint8ClampedArray|Uint8Array, width: number, height: number}} img
// @returns {Float64Array} 32×32 灰度（行优先）
export function toGray(img) {
  const { data, width, height } = img
  const out = new Float64Array(PHASH_SIDE * PHASH_SIDE)
  for (let y = 0; y < PHASH_SIDE; y++) {
    const sy = (y * (height - 1)) / (PHASH_SIDE - 1)
    const y0 = Math.floor(sy)
    const y1 = Math.min(y0 + 1, height - 1)
    const fy = sy - y0
    for (let x = 0; x < PHASH_SIDE; x++) {
      const sx = (x * (width - 1)) / (PHASH_SIDE - 1)
      const x0 = Math.floor(sx)
      const x1 = Math.min(x0 + 1, width - 1)
      const fx = sx - x0
      const lum = (px, py) => {
        const i = (py * width + px) * 4
        return 0.299 * data[i] + 0.587 * data[i + 1] + 0.114 * data[i + 2]
      }
      const v = (lum(x0, y0) * (1 - fx) + lum(x1, y0) * fx) * (1 - fy) +
        (lum(x0, y1) * (1 - fx) + lum(x1, y1) * fx) * fy
      out[y * PHASH_SIDE + x] = v
    }
  }
  return out
}

// dct2 二维 DCT-II（行/列各一遍一维变换，可分离实现）。
export function dct2(gray) {
  const n = PHASH_SIDE
  const tmp = new Float64Array(n * n)
  const out = new Float64Array(n * n)
  for (let y = 0; y < n; y++) {
    for (let k = 0; k < n; k++) {
      let s = 0
      for (let x = 0; x < n; x++) s += gray[y * n + x] * dctBasis[k * n + x]
      tmp[y * n + k] = s
    }
  }
  for (let k = 0; k < n; k++) {
    for (let v = 0; v < n; v++) {
      let s = 0
      for (let y = 0; y < n; y++) s += tmp[y * n + k] * dctBasis[v * n + y]
      out[v * n + k] = s
    }
  }
  return out
}

// phashFromImageData 计算 64bit 感知哈希（hex16）。
// @param {{data: Uint8ClampedArray|Uint8Array, width: number, height: number}} img
// @returns {string} 16 位十六进制；输入非法时返回空串
export function phashFromImageData(img) {
  if (!img || !img.data || !img.width || !img.height) return ''
  const gray = toGray(img)
  const coeff = dct2(gray)
  const n = PHASH_SIDE
  const low = []
  for (let y = 0; y < PHASH_LOW; y++) {
    for (let x = 0; x < PHASH_LOW; x++) {
      if (x === 0 && y === 0) continue // 跳过 DC：整体亮度随显示器/伽马变化
      low.push(coeff[y * n + x])
    }
  }
  const sorted = [...low].sort((a, b) => a - b)
  const median = sorted[Math.floor(sorted.length / 2)]
  let bits = ''
  for (const c of low) bits += c > median ? '1' : '0'
  bits += '0' // 第 64 位保留（有效位 63），保证 hex 定长 16
  let hex = ''
  for (let i = 0; i < 64; i += 4) hex += parseInt(bits.slice(i, i + 4), 2).toString(16)
  return hex
}

// hexToBits 十六进制哈希 → 比特数组（非法输入返回空数组）。
export function hexToBits(hex) {
  const s = String(hex || '').trim().toLowerCase()
  if (!/^[0-9a-f]+$/.test(s)) return []
  const bits = []
  for (const ch of s) {
    const v = parseInt(ch, 16)
    for (let b = 3; b >= 0; b--) bits.push((v >> b) & 1)
  }
  return bits
}

// hammingDistance 两个十六进制感知哈希的汉明距离（非法/长度不符时返回 Infinity，
// 让调用方按"不相似"处理，绝不误判为同源）。
export function hammingDistance(a, b) {
  const ba = hexToBits(a)
  const bb = hexToBits(b)
  if (!ba.length || ba.length !== bb.length) return Infinity
  let d = 0
  for (let i = 0; i < ba.length; i++) if (ba[i] !== bb[i]) d++
  return d
}

// similarPHash 汉明距离 ≤ maxDistance 视为同源（规格默认 10）。
export function similarPHash(a, b, maxDistance = 10) {
  const d = hammingDistance(a, b)
  return d !== Infinity && d <= maxDistance
}
