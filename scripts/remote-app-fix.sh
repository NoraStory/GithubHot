#!/usr/bin/env bash
# 远端 APP 兼容修复 + 实证（由 infra-app-fix.yml 经 stdin 喂给 bash -s 执行）
# 1) 实证：用握手时代旧种子（gh-dev-seed-v1）完整复刻 APP 签名打真实请求
# 2) 旧种子不在 GRACE 列表则补上（存量 APP 兼容，过渡期 ≤14 天）
# 3) 清理过期封禁行
# 4) 去掉常驻强制升级横幅（APP_FORCE_UPGRADE_URL 清空；需要时重新设置即可）
# 5) 固化 IP_GUARD_SECRET（防重启随机化导致全站 id-token-stale）
set -e
DB=/opt/githubhot/data/githubhot.db

echo "=== 0. 当前种子配置（主种子掩码，GRACE 为历史公开种子可显示）==="
GRACE_NOW=$(grep -oP "(?<=^APP_SIGN_SEED_GRACE=).*" /opt/githubhot/.env 2>/dev/null || sudo grep -oP "(?<=^APP_SIGN_SEED_GRACE=).*" /opt/githubhot/.env 2>/dev/null || systemctl show githubhot -p Environment 2>/dev/null | tr ' ' '\n' | grep -oP "(?<=APP_SIGN_SEED_GRACE=).*" || echo "")
echo "GRACE=[$GRACE_NOW]"

echo "=== 1. 实证：旧种子签名请求 ==="
python3 - <<'PY'
import hashlib, hmac, json, os, time, urllib.request

BASE = "http://127.0.0.1:8787"
FP = "empirical-fp-0123456789ab"
OLD_SEED = "gh-dev-seed-v1"  # 握手时代缓存种子（P0-1 前公开下发）

def session_key(seed, fp):
    return hmac.new((seed + ":" + fp).encode(), b"gh-session-v1", hashlib.sha256).digest()

def sign(seed, fp, ts, nonce, method, path, body=b""):
    key = session_key(seed, fp)
    bh = hashlib.sha256(body).hexdigest()
    payload = f"{ts}\n{nonce}\n{method.upper()}\n{path}\n{bh}"
    return hmac.new(key, payload.encode(), hashlib.sha256).hexdigest()

def app_get(path, seed):
    ts = str(int(time.time()))
    nonce = os.urandom(8).hex()
    sig = sign(seed, FP, ts, nonce, "GET", path, b"")
    token = hashlib.sha256(session_key(seed, FP) + FP.encode()).hexdigest()[:40]
    req = urllib.request.Request(BASE + path)
    for k, v in [
        ("X-App-Sign", sig), ("X-App-Ts", ts), ("X-App-Nonce", nonce),
        ("X-Session-Token", token), ("X-Device-Fingerprint", FP),
        ("X-Device-Model", "diag"), ("User-Agent", "GithubHot-APP/1.0"),
    ]:
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code

print(f"  旧种子 gh-dev-seed-v1 签名 GET /api/v1/hot → {app_get('/api/v1/hot', OLD_SEED)}（200=存量APP兼容，401=需补GRACE）")
PY

echo "=== 2. GRACE 补齐（幂等）==="
if ! grep -q "^APP_SIGN_SEED_GRACE=" /opt/githubhot/.env 2>/dev/null; then
  echo "APP_SIGN_SEED_GRACE=gh-dev-seed-v1" >> /opt/githubhot/.env
  echo "GRACE 已写入 gh-dev-seed-v1"
else
  grep -q "gh-dev-seed-v1" /opt/githubhot/.env && echo "GRACE 已含 gh-dev-seed-v1" || {
    sed -i "s|^APP_SIGN_SEED_GRACE=.*|&,gh-dev-seed-v1|" /opt/githubhot/.env
    echo "GRACE 已追加 gh-dev-seed-v1"
  }
fi
chmod 600 /opt/githubhot/.env

echo "=== 3. 清理过期封禁行 ==="
sudo python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
n = db.execute("DELETE FROM ip_bans WHERE expires_at < datetime(?, ?)", ("now", "0 minutes")).rowcount
db.commit()
print(f"已清理 {n} 条过期封禁")
PY

echo "=== 4. 关闭常驻强制升级横幅（清空 APP_FORCE_UPGRADE_URL）==="
CHANGED=0
if sudo grep -q "APP_FORCE_UPGRADE_URL" /etc/systemd/system/githubhot.service 2>/dev/null; then
  sudo sed -i "s|^Environment=APP_FORCE_UPGRADE_URL=.*|#&|" /etc/systemd/system/githubhot.service
  CHANGED=1
fi
for f in /etc/systemd/system/githubhot.service.d/*.conf; do
  [ -f "$f" ] || continue
  if grep -q "APP_FORCE_UPGRADE_URL" "$f"; then
    sudo sed -i "s|^Environment=APP_FORCE_UPGRADE_URL=.*|#&|" "$f"
    CHANGED=1
  fi
done
sudo grep -q "^APP_FORCE_UPGRADE_URL=" /opt/githubhot/.env 2>/dev/null && {
  sed -i "s|^APP_FORCE_UPGRADE_URL=.*|APP_FORCE_UPGRADE_URL=|" /opt/githubhot/.env
  echo "已清空 .env 的 APP_FORCE_UPGRADE_URL"
  CHANGED=1
}
[ "$CHANGED" = "1" ] && sudo systemctl daemon-reload

echo "=== 5. 固化 IP_GUARD_SECRET（防重启随机化）==="
if ! grep -q "^IP_GUARD_SECRET=" /opt/githubhot/.env 2>/dev/null; then
  echo "IP_GUARD_SECRET=$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 43)" >> /opt/githubhot/.env
  chmod 600 /opt/githubhot/.env
  echo "IP_GUARD_SECRET 已生成写入（不回显）"
else
  echo "已存在，跳过"
fi

echo "=== 6. 重启并验证 ==="
sudo systemctl daemon-reload
sudo systemctl restart githubhot
ok=""
i=0
while [ $i -lt 20 ]; do
  if curl -fsS http://127.0.0.1:8787/api/v1/healthz >/dev/null 2>&1; then ok=1; break; fi
  sleep 3
  i=$((i+1))
done
[ -n "$ok" ] || { echo "::error::重启后不健康"; sudo journalctl -u githubhot -n 30 --no-pager; exit 1; }

echo "--- 握手域（force_upgrade 应为空，banned 应 False）---"
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/site/config | python3 -c "
import json, sys
d = json.load(sys.stdin)
print('banned =', d.get('banned'), '| force_upgrade_url =', repr(d.get('force_upgrade_url')))
"
echo "--- 复测：旧种子签名请求（重启后 GRACE 生效应 200）---"
python3 - <<'PY'
import hashlib, hmac, os, time, urllib.request
BASE = "http://127.0.0.1:8787"
FP = "empirical-fp-0123456789ab"
seed = "gh-dev-seed-v1"
key = hmac.new((seed + ":" + FP).encode(), b"gh-session-v1", hashlib.sha256).digest()
ts = str(int(time.time()))
nonce = os.urandom(8).hex()
bh = hashlib.sha256(b"").hexdigest()
sig = hmac.new(key, f"{ts}\n{nonce}\nGET\n/api/v1/hot\n{bh}".encode(), hashlib.sha256).hexdigest()
token = hashlib.sha256(key + FP.encode()).hexdigest()[:40]
req = urllib.request.Request(BASE + "/api/v1/hot")
for k, v in [("X-App-Sign", sig), ("X-App-Ts", ts), ("X-App-Nonce", nonce),
             ("X-Session-Token", token), ("X-Device-Fingerprint", FP), ("X-Device-Model", "diag")]:
    req.add_header(k, v)
try:
    with urllib.request.urlopen(req, timeout=10) as resp:
        print(f"  旧种子签名请求 → {resp.status}（200=存量APP恢复）")
except urllib.error.HTTPError as e:
    print(f"  旧种子签名请求 → {e.code} {e.read()[:100]}")
PY
