import json
import sqlite3

rows = json.load(open("/tmp/sources-export.json", encoding="utf-8"))
c = sqlite3.connect("/opt/githubhot/data/githubhot.db")
cols = [d[1] for d in c.execute("PRAGMA table_info(sources)")]
n = 0
for r in rows:
    r["last_fetched_at"] = None
    r["empty_streak"] = 0
    keys = [k for k in cols if k in r]
    col_sql = ", ".join(keys)
    val_sql = ", ".join("?" for _ in keys)
    sql = "INSERT OR IGNORE INTO sources (" + col_sql + ") VALUES (" + val_sql + ")"
    cur = c.execute(sql, [r[k] for k in keys])
    n += cur.rowcount
c.commit()
print("导入信源:", n)
print("现有总数:", c.execute("SELECT COUNT(*) FROM sources").fetchone()[0])
print("启用数:", c.execute("SELECT COUNT(*) FROM sources WHERE enabled=1").fetchone()[0])
