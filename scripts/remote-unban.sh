#!/usr/bin/env bash
# 远端误伤解除（由 infra-unban.yml 经 stdin 喂给 bash -s 执行）
# 流程：导出封禁列表 + 近 24h 事件（定位误伤原因）→ 清空封禁 → 服务自检
set -e
DB=/opt/githubhot/data/githubhot.db

echo "=== 当前封禁列表 ==="
python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
rows = db.execute(
    "SELECT ip, strikes, level, reason, banned_at, expires_at FROM ip_bans ORDER BY banned_at DESC"
).fetchall()
if not rows:
    print("(无封禁)")
for r in rows:
    print(f"  IP={r[0]} strikes={r[1]} level={r[2]} reason={r[3]} from={r[4]} to={r[5]}")
PY

echo "=== 近 24h 事件 Top 30（定位误伤原因）==="
python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
rows = db.execute(
    "SELECT ip, kind, detail, score, created_at FROM ip_events"
    " WHERE created_at >= datetime(?, ?) ORDER BY id DESC LIMIT 30",
    ("now", "-1 day"),
).fetchall()
if not rows:
    print("(24h 内无事件)")
for r in rows:
    print(f"  {r[4]}  {r[0]}  [{r[1]}] +{r[3]}  {str(r[2])[:80]}")
PY

echo "=== 清理弱证据事件（ALTCHA 误报残留）==="
sudo python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
n = db.execute("DELETE FROM ip_events WHERE kind IN ('altcha-missing', 'altcha-failed', 'altcha-replayed')").rowcount
db.commit()
print(f"已清理 {n} 条 ALTCHA 弱证据事件")
PY

echo "=== 解除全部封禁（DB 为 root 属主，DELETE 走 sudo）==="
sudo python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
n = db.execute("DELETE FROM ip_bans").rowcount
db.commit()
print(f"已删除 {n} 条封禁")
PY

echo "=== 服务自检 ==="
curl -s -o /dev/null -w "healthz: %{http_code}\n" --max-time 5 http://127.0.0.1:8787/api/v1/healthz
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/ip/check | head -c 200
echo ""
