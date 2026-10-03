#!/usr/bin/env python3
"""服务器端：把 Actions 流水线产出的内容表合并进正式库（风控/会话表不动）。

用法: sudo python3 sync_merge.py /tmp/actions-ghot.db
"""
import sqlite3
import sys

SRC = sys.argv[1]
DST = "/opt/githubhot/data/githubhot.db"

# 内容表：Actions（国际出口，境外源可抓）与服务器（国内源）双向互补合并
CONTENT_TABLES = [
    "sources",
    "items",
    "projects",
    "project_snapshots",
    "stories",
    "story_history",
    "digests",
    "domestic_summaries",
    "link_images",
]
# 按主键覆盖的表（同日重跑以新结果为准）
REPLACE_TABLES = {"digests", "domestic_summaries"}


def main() -> int:
    d = sqlite3.connect(DST)
    d.execute("PRAGMA busy_timeout=20000")
    # 先校验来源库可读
    s = sqlite3.connect(f"file:{SRC}?mode=ro", uri=True)
    s.execute("SELECT 1")
    s.close()

    d.execute(f"ATTACH DATABASE '{SRC}' AS src")
    total = 0
    for t in CONTENT_TABLES:
        cols = [r[1] for r in d.execute(f"PRAGMA table_info({t})")]
        if not cols:
            print(f"[merge] 跳过不存在的表 {t}")
            continue
        col_sql = ", ".join(cols)
        verb = "INSERT OR REPLACE" if t in REPLACE_TABLES else "INSERT OR IGNORE"
        cur = d.execute(
            f"{verb} INTO {t} ({col_sql}) SELECT {col_sql} FROM src.{t}"
        )
        print(f"[merge] {t}: {cur.rowcount} 行")
        total += cur.rowcount
    d.commit()
    d.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    d.close()
    print(f"[merge] 完成，共 {total} 行")
    return 0


if __name__ == "__main__":
    sys.exit(main())
