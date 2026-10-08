#!/usr/bin/env python3
# 管理端登录 PoW 链路契约测试（由 contract-admin-login.yml 经 stdin 喂给 python3 执行）
# 环境变量：B64PW=base64(管理员密码)
# 验证：1) 无 PoW 登录 → 401；2) 有效 PoW + 正确密码 → 200；3) 同解重放 → 401
import base64
import hashlib
import json
import os
import sys
import urllib.request

BASE = "http://127.0.0.1:8787"
PW = base64.b64decode(os.environ["B64PW"]).decode()
FP = "contract-fp-0123456789abcdef"


def post(path, body):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")[:160]
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")[:160]


def leading_bits(digest):
    bits = 0
    for b in digest:
        if b == 0:
            bits += 8
            continue
        for m in range(7, -1, -1):
            if b & (1 << m):
                break
            bits += 1
        break
    return bits


print("--- 1. 领挑战 ---")
with urllib.request.urlopen(BASE + "/api/v1/altcha/challenge?fp=" + FP, timeout=10) as resp:
    ch = json.loads(resp.read())
print(f"    difficulty={ch['difficulty']} maxage={ch['maxage']}")

print("--- 2. 求解 ---")
nonce = 0
while True:
    digest = hashlib.sha256((ch["challenge"] + str(nonce)).encode()).digest()
    if leading_bits(digest) >= ch["difficulty"]:
        break
    nonce += 1
print(f"    nonce={nonce}")
sol = {"challenge": ch["challenge"], "nonce": str(nonce), "signature": ch["signature"]}

print("--- 3. 无 PoW 登录（应 401 工作量证明）---")
code, body = post("/api/v1/admin/login", {"password": PW})
print(f"    {code}  {body}")
assert code == 401 and "工作量证明" in body, "无 PoW 应 401+工作量证明文案"

print("--- 4. 有效 PoW + 正确密码（应 200）---")
code, body = post("/api/v1/admin/login", {"password": PW, "fp": FP, "altcha": sol})
print(f"    {code}  {body}")
assert code == 200, f"有效 PoW+正确密码应 200，实际 {code} {body}"

print("--- 5. 同解重放（应 401 已被使用）---")
code, body = post("/api/v1/admin/login", {"password": PW, "fp": FP, "altcha": sol})
print(f"    {code}  {body}")
assert code == 401 and "已被使用" in body, "重放应 401+已被使用"

print("CONTRACT ALL PASS ✓")
