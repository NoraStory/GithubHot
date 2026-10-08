#!/usr/bin/env bash
# 远端回环豁免切换（由 infra-loopback-exempt.yml 经 "GHVAL=x bash -s" < 本文件执行）
# 环境变量：GHVAL=0|1、GHDIR=/opt/githubhot
set -e
case "$GHVAL" in 0|1) ;; *) echo "::error::值必须为 0 或 1"; exit 1;; esac
cd "$GHDIR"
if grep -q "^IP_GUARD_LOCAL=" .env 2>/dev/null; then
  sed -i "s|^IP_GUARD_LOCAL=.*|IP_GUARD_LOCAL=$GHVAL|" .env
else
  echo "IP_GUARD_LOCAL=$GHVAL" >> .env
fi
chmod 600 .env
echo "IP_GUARD_LOCAL 已设为 $GHVAL"
sudo systemctl restart githubhot
ok=""
i=0
while [ $i -lt 20 ]; do
  if curl -fsS http://127.0.0.1:8787/api/v1/healthz >/dev/null 2>&1; then ok=1; break; fi
  sleep 3
  i=$((i+1))
done
[ -n "$ok" ] || { echo "::error::重启后不健康"; sudo journalctl -u githubhot -n 30 --no-pager; exit 1; }
echo "--- 验证 ---"
grep "^IP_GUARD_LOCAL=" .env
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/ip/check | head -c 200
echo ""
