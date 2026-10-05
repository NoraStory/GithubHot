// ALTCHA PoW 前端求解器（P4-3）：nonce 满足难度、确定性、超时保护。
import test from 'node:test'
import assert from 'node:assert/strict'
import { solveChallenge, leadingZeroBits } from './altcha.js'

async function sha256Buf(s) {
  return crypto.subtle.digest('SHA-256', new TextEncoder().encode(s))
}

test('leadingZeroBits 向量', () => {
  const mk = bytes => new Uint8Array(bytes).buffer
  assert.equal(leadingZeroBits(mk([0xff])), 0)
  assert.equal(leadingZeroBits(mk([0x00, 0x0f])), 12)
  assert.equal(leadingZeroBits(mk([0, 0, 0])), 24)
})

test('solveChallenge：解满足前导零要求且确定性', async () => {
  for (const d of [4, 8, 10]) {
    const n1 = await solveChallenge('fixed-challenge-str', d, 5000)
    assert.ok(n1 !== null, `difficulty ${d} 应可解`)
    const bits = leadingZeroBits(await sha256Buf('fixed-challenge-str' + n1))
    assert.ok(bits >= d, `difficulty ${d} 的解仅 ${bits} 比特`)
    const n2 = await solveChallenge('fixed-challenge-str', d, 5000)
    assert.equal(n1, n2, '同挑战同难度应确定性收敛到同一 nonce')
  }
})

test('solveChallenge：超时保护返回 null', async () => {
  const n = await solveChallenge('impossible-in-time', 40, 50)
  assert.equal(n, null, 'difficulty 40 应超时放弃')
})
