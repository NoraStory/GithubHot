#!/usr/bin/env bash
# 远端强制升级门禁清除（由 infra-app-fix2.yml 经 stdin 喂给 bash -s 执行）
# 1) 定位 APP_FORCE_UPGRADE_URL 的真实定义位置（unit/override/.env，容各种引号格式）并整行删除
# 2) daemon-reload + 重启
# 3) 验证：site/config force 为空 + 旧种子签名实证（GRACE 生效应 200）
set -e

echo "=== 1. 定位并删除 APP_FORCE_UPGRADE_URL 定义 ==="
HITS=$(sudo grep -rln "APP_FORCE_UPGRADE_URL" /etc/systemd/system/ 2>/dev/null || true)
echo "命中文件: ${HITS:-（systemd 无）}"
for f in $HITS; do
  sudo grep -n "APP_FORCE_UPGRADE_URL" "$f" | while IFS=: read -r ln rest; do
    echo "  删除 $f:$ln"
  done
  sudo sed -i "/APP_FORCE_UPGRADE_URL/d" "$f"
done
grep -q "APP_FORCE_UPGRADE_URL" /opt/githubhot/.env 2>/dev/null && {
  sed -i "/APP_FORCE_UPGRADE_URL/d" /opt/githubhot/.env
  echo "  删除 .env 中的定义"
}
sudo systemctl daemon-reload

echo "=== 2. 重启 ==="
sudo systemctl restart githubhot
ok=""
i=0
while [ $i -lt 20 ]; do
  if curl -fsS http://127.0.0.1:8787/api/v1/healthz >/dev/null 2>&1; then ok=1; break; fi
  sleep 3
  i=$((i+1))
done
[ -n "$ok" ] || { echo "::error::重启后不健康"; sudo journalctl -u githubhot -n 30 --no-pager; exit 1; }

echo "=== 3. 验证：握手域（force 应为空）==="
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/site/config | python3 -c "
import json, sys
d = json.load(sys.stdin)
print('banned =', d.get('banned'), '| force_upgrade_url =', repr(d.get('force_upgrade_url')))
"

echo "=== 4. 实证：旧种子（gh-dev-seed-v1）签名请求（GRACE 生效应 200）==="
python3 - <<'PY'
import hashlib, hmac, os, time, urllib.request
BASE = "http://127.0.0.1:8787"
FP = "empirical-fp-0123456789ab"
seed = "gh-dev-seed-v1"
key = hmac.new((seed + ":" + FP).encode(), b"gh-session-v1", hashlib.sha256).digest()
ts = str(int(time.time() * 1000))  # 服务器 UnixMilli：毫秒时间戳
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
        body = resp.read()[:60]
        print(f"  旧种子签名请求 → {resp.status} {body}")
except urllib.error.HTTPError as e:
    print(f"  旧种子签名请求 → {e.code} {e.read()[:120]}")
PY
