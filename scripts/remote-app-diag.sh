#!/usr/bin/env bash
# 远端 APP 故障诊断（由 infra-app-diag.yml 经 stdin 喂给 bash -s 执行）
# 只读：封禁/事件、APK 目录、握手响应字段、今日 403/404 日志、相关 env 键名
set -e
DB=/opt/githubhot/data/githubhot.db

echo "=== 1. 当前封禁 ==="
python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
rows = db.execute("SELECT ip, strikes, level, reason, banned_at, expires_at FROM ip_bans ORDER BY banned_at DESC").fetchall()
if not rows:
    print("(无封禁)")
for r in rows:
    print(f"  IP={r[0]} strikes={r[1]} level={r[2]} reason={r[3]} to={r[5]}")
PY

echo "=== 2. 今日事件 Top 25 ==="
python3 - "$DB" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
rows = db.execute(
    "SELECT ip, kind, detail, score, created_at FROM ip_events"
    " WHERE created_at >= datetime(?, ?) ORDER BY id DESC LIMIT 25",
    ("now", "-1 day"),
).fetchall()
if not rows:
    print("(无事件)")
for r in rows:
    print(f"  {r[4]}  {r[0]}  [{r[1]}] +{r[3]}  {str(r[2])[:70]}")
PY

echo "=== 3. APK 目录 ==="
ls -la /opt/githubhot/apks/ 2>/dev/null || echo "(apks/ 不存在)"
sudo systemctl show githubhot -p Environment 2>/dev/null | tr ' ' '\n' | sed 's/=.*//' | grep -E "^APP_" || echo "(systemd 无 APP_ 环境变量)"
grep -oE "^APP_[A-Z_]*" /opt/githubhot/.env 2>/dev/null || echo "(.env 无 APP_ 键)"

echo "=== 4. 握手响应关键域（banned/force_upgrade）==="
curl -s --max-time 5 http://127.0.0.1:8787/api/v1/site/config | python3 -c "
import json, sys
d = json.load(sys.stdin)
print('banned =', d.get('banned'))
print('force_upgrade_url =', repr(d.get('force_upgrade_url')))
"

echo "=== 5. APK 下载端点实测 ==="
curl -s -o /dev/null -w "/app/githubhot-latest.apk -> %{http_code}\n" --max-time 5 http://127.0.0.1:8787/app/githubhot-latest.apk

echo "=== 6. 今日 403/404/appguard 日志 ==="
sudo journalctl -u githubhot --since today --no-pager 2>/dev/null | grep -iE "403|appguard|签名|force|app-sign" | tail -12 || echo "(无相关日志)"
