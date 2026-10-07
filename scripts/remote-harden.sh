#!/usr/bin/env bash
# 远端管理端鉴权加固脚本（由 infra-admin-auth.yml 经 stdin 喂给 bash -s 执行）
# 环境变量（远端 argv 注入）：B64PW=base64(管理员密码)、GHDIR=/opt/githubhot
set -e
ADMIN_PASSWORD=$(printf %s "$B64PW" | base64 -d)
[ -n "$ADMIN_PASSWORD" ] || { echo "::error::密码传输为空"; exit 1; }
cd "$GHDIR"

echo "--- 管理端鉴权状态（加固前）---"
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/admin/session | head -c 120 || true
echo ""

# 强制覆盖（open 模式收口 + 换密均走这里）：stdout 捕获进变量，不回显
HASH=$(./githubhot admin hash "$ADMIN_PASSWORD")
[ -n "$HASH" ] || { echo "::error::哈希生成失败"; exit 1; }
if grep -q "^ADMIN_PASSWORD_HASH=" .env 2>/dev/null; then
  sed -i "s|^ADMIN_PASSWORD_HASH=.*|ADMIN_PASSWORD_HASH=$HASH|" .env
  echo "ADMIN_PASSWORD_HASH 已覆盖"
else
  echo "ADMIN_PASSWORD_HASH=$HASH" >> .env
  echo "ADMIN_PASSWORD_HASH 已写入"
fi
chmod 600 .env
rm -f ADMIN_INITIAL_PASSWORD.txt
unset HASH

echo "--- 行为 LR 模型补传 ---"
sudo mkdir -p models
sudo tar xzf /tmp/model.tar.gz -C models
rm -f /tmp/model.tar.gz
ls -la models/

echo "--- 重启并验证 ---"
sudo systemctl restart githubhot
ok=""
i=0
while [ $i -lt 20 ]; do
  if curl -fsS http://127.0.0.1:8787/api/v1/healthz >/dev/null 2>&1; then ok=1; break; fi
  sleep 3
  i=$((i+1))
done
[ -n "$ok" ] || { echo "::error::重启后不健康"; sudo journalctl -u githubhot -n 30 --no-pager; exit 1; }

echo '--- 登录回验（指定密码应 200；只打印状态码）---'
LCODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 \
  -X POST -H "Content-Type: application/json" \
  -d "{\"password\": \"$ADMIN_PASSWORD\"}" \
  http://127.0.0.1:8787/api/v1/admin/login)
echo "login 状态码: $LCODE"
if [ "$LCODE" != "200" ]; then
  echo "::error::指定密码登录失败（$LCODE）"
  exit 1
fi
unset ADMIN_PASSWORD

echo '--- 加固后鉴权探测（应 401/未登录，绝不能再是 open）---'
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/admin/session | head -c 120
echo ""
echo '--- 行为模型加载确认 ---'
sudo journalctl -u githubhot -n 40 --no-pager | grep -i "行为模型\|behavior" | tail -3 || echo "(未找到行为模型日志行)"
