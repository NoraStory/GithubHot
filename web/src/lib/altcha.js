// ALTCHA PoW 客户端（P4-3，规格书 §7）：领挑战 → 暴力 nonce（SHA-256 前导零比特
// ≥ difficulty）→ 组装 {challenge, nonce, signature} 供 fp/report 携带。
// signature 由服务端在签发时绑定 fp（客户端无密钥、不可自造），原样回传即可。
// 服务端未启用（404/503）或求解超时 → 返回 null，fp/report 不带 altcha 字段。

async function sha256Buf(s) {
  return crypto.subtle.digest('SHA-256', new TextEncoder().encode(s))
}

function leadingZeroBits(buf) {
  let bits = 0
  for (const b of new Uint8Array(buf)) {
    if (b === 0) { bits += 8; continue }
    for (let m = 0x80; m !== 0 && (b & m) === 0; m >>= 1) bits++
    break
  }
  return bits
}

// solveChallenge 对固定挑战串暴力求解。死线按难度自适应（3 秒是难度 12 影子模式
// 的旧参数；难度 14 平均 1.6 万次异步哈希，慢设备会撞线超时 → 静默 null → 登录失败）。
export async function solveChallenge(challenge, difficulty, deadlineMs = 0) {
  const deadline = Date.now() + (deadlineMs > 0
    ? deadlineMs
    : Math.min(20000, 2000 + Math.pow(2, difficulty) * 3))
  for (let n = 0; n <= 1 << 22; n++) {
    if (leadingZeroBits(await sha256Buf(challenge + n)) >= difficulty) {
      return String(n)
    }
    if ((n & 255) === 0 && Date.now() > deadline) return null
  }
  return null
}

// solveAltcha 完整流程：领挑战（绑定 fp）→ 求解 → 返回提交体；失败返回 null。
export async function solveAltcha(fp) {
  try {
    const res = await fetch('/api/v1/altcha/challenge?fp=' + encodeURIComponent(fp))
    if (!res.ok) return null
    const { challenge, difficulty, signature } = await res.json()
    if (!challenge || !difficulty || !signature) return null
    const nonce = await solveChallenge(challenge, difficulty)
    if (nonce === null) return null
    return { challenge, nonce, signature }
  } catch { return null }
}

// 供 node --test 的纯函数出口。
export { leadingZeroBits }
