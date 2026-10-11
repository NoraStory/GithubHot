#!/usr/bin/env bash
# 远端立即运行一轮完整流水线（由 infra-pipeline-run.yml 经 stdin 喂给 bash -s 执行）
# 用途：新信源入库（SeedSources 在流水线开头导入缺失 ID）+ 全源立即抓取一轮。
# 说明：与每日 07:30 的 HOT_CRON 同一入口；会消耗一次 LLM 管线（预算守卫在位）。
set -e
DIR=${GH_DIR:-/opt/githubhot}
cd "$DIR"

BIN="$DIR/githubhot"
if [ ! -x "$BIN" ]; then
  echo "错误：未找到 $BIN"; exit 1
fi
echo "=== 触发完整流水线（导入新信源 + 全源抓取 + LLM 精选 + 日报）==="
sudo "$BIN" run 2>&1 | tail -40
echo "=== 流水线退出码 $? ==="

echo "=== 服务自检 ==="
curl -s -o /dev/null -w "healthz: %{http_code}\n" --max-time 5 http://127.0.0.1:8787/healthz
