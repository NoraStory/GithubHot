#!/usr/bin/env bash
# 远端清除指定 IP 计分污染（由 infra-clear-scores.yml 经 stdin 喂给 bash -s 执行）
# 环境变量：GH_TARGET_IP（必填）、GH_KINDS（可选逗号列表，空 = 全部类型）
# 流程：先打印该 IP 现状（先诊断后动手）→ 删事件/封禁 → 重启服务（清内存账本）→ 自检
set -e
DB=/opt/githubhot/data/githubhot.db

if [ -z "$GH_TARGET_IP" ]; then
  echo "错误：GH_TARGET_IP 未设置"; exit 1
fi

echo "=== 目标 IP：$GH_TARGET_IP  类型过滤：${GH_KINDS:-（全部）} ==="

echo "=== [诊断] 该 IP 当前封禁 ==="
python3 - "$DB" "$GH_TARGET_IP" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
rows = db.execute(
    "SELECT ip, strikes, level, reason, banned_at, expires_at FROM ip_bans WHERE ip = ?", (sys.argv[2],)
).fetchall()
if not rows:
    print("(无封禁)")
for r in rows:
    print(f"  IP={r[0]} strikes={r[1]} level={r[2]} reason={r[3]} from={r[4]} to={r[5]}")
PY

echo "=== [诊断] 该 IP 近 7 天事件（Top 50）==="
python3 - "$DB" "$GH_TARGET_IP" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
rows = db.execute(
    "SELECT kind, detail, score, created_at FROM ip_events"
    " WHERE ip = ? AND created_at >= datetime(?, ?) ORDER BY id DESC LIMIT 50",
    (sys.argv[2], "now", "-7 days"),
).fetchall()
if not rows:
    print("(7 天内无事件)")
for r in rows:
    print(f"  {r[3]}  [{r[0]}] +{r[1]}  {str(r[2])[:80]}")
PY

echo "=== [执行] 删除该 IP 事件（DB 为 root 属主，走 sudo）==="
sudo python3 - "$DB" "$GH_TARGET_IP" "$GH_KINDS" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
ip, kinds = sys.argv[2], sys.argv[3]
if kinds.strip():
    ks = [k.strip() for k in kinds.split(",") if k.strip()]
    qmarks = ",".join("?" * len(ks))
    n = db.execute(f"DELETE FROM ip_events WHERE ip = ? AND kind IN ({qmarks})", [ip] + ks).rowcount
else:
    n = db.execute("DELETE FROM ip_events WHERE ip = ?", (ip,)).rowcount
db.commit()
print(f"已删除 {n} 条事件")
nb = db.execute("DELETE FROM ip_bans WHERE ip = ?", (ip,)).rowcount
db.commit()
print(f"已删除 {nb} 条封禁")
PY

echo "=== [执行] 重启服务（清内存 banCache/soft 账本，DB 已是干净基线）==="
sudo systemctl restart githubhot
sleep 3

echo "=== 服务自检 ==="
curl -s -o /dev/null -w "healthz: %{http_code}\n" --max-time 5 http://127.0.0.1:8787/healthz
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/ip/check | head -c 200
echo ""
