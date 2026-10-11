#!/usr/bin/env bash
# 远端立即运行一轮完整流水线（由 infra-pipeline-run.yml 经 stdin 喂给 bash -s 执行）
# 用途：新信源入库（SeedSources 在流水线开头导入缺失 ID）+ 全源立即抓取一轮。
# 环境加载：LLM 配置在 systemd 单元（Environment=/EnvironmentFile=）而非 .env，
# 直接 sudo 裸跑会丢 LLM_API_KEY（实测：采集入库成功但 LLM 阶段中断）——
# 这里从单元定义 + .env 三路兜底合并环境。
set -e
DIR=${GH_DIR:-/opt/githubhot}
cd "$DIR"

BIN="$DIR/githubhot"
if [ ! -x "$BIN" ]; then
  echo "错误：未找到 $BIN"; exit 1
fi

# 合并环境：systemd 单元的 Environment="K=V" 行 + EnvironmentFile + 目录 .env
TMPENV=$(mktemp)
trap 'rm -f "$TMPENV"' EXIT
systemctl cat githubhot 2>/dev/null | grep -E '^Environment="' | sed 's/^Environment="//; s/"$//' >> "$TMPENV" || true
EF=$(systemctl cat githubhot 2>/dev/null | grep -oE '^EnvironmentFile=-?\S+' | head -1 | sed 's/^EnvironmentFile=-*//')
[ -n "$EF" ] && [ -f "$EF" ] && cat "$EF" >> "$TMPENV"
[ -f "$DIR/.env" ] && cat "$DIR/.env" >> "$TMPENV"
if grep -q "LLM_API_KEY" "$TMPENV"; then
  echo "=== 环境就绪（LLM_API_KEY 已加载，来源：systemd 单元/.env）==="
else
  echo "=== 警告：LLM_API_KEY 未在 systemd 单元或 .env 中找到，LLM 阶段会降级 ==="
fi

echo "=== 触发完整流水线（导入新信源 + 全源抓取 + LLM 精选 + 日报）==="
sudo bash -c "set -a; . '$TMPENV'; cd '$DIR'; exec ./githubhot run" 2>&1 | tail -40

echo "=== 服务自检 ==="
curl -s -o /dev/null -w "healthz: %{http_code}\n" --max-time 5 http://127.0.0.1:8787/healthz
