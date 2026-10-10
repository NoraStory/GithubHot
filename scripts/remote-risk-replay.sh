#!/usr/bin/env bash
# 生产数据风控参数回放评估（由 infra-risk-replay.yml 经 stdin 喂给 bash -s 执行）
# 流程：定位二进制 → risk replay --sweep（读生产 DB 全量事件/封禁，输出参数对比表）
set -e
DIR=${GH_DIR:-/opt/githubhot}
cd "$DIR"

BIN=""
for c in "$DIR/githubhot" "$DIR/githubhot-new" "$(ls -t "$DIR"/githubhot* 2>/dev/null | head -1)"; do
  if [ -n "$c" ] && [ -x "$c" ]; then BIN="$c"; break; fi
done
if [ -z "$BIN" ]; then
  echo "错误：未找到 githubhot 二进制"; ls -la "$DIR" | head -20; exit 1
fi
echo "=== 使用二进制：$BIN ==="
# DB 为 root 属主，走 sudo；config.Load 自行加载目录内 .env
sudo "$BIN" risk replay --sweep
