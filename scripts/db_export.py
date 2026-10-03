#!/usr/bin/env python3
"""服务器端：WAL checkpoint 后把数据库副本放到 /tmp 供 Actions 拉取。

用法: sudo python3 db_export.py
"""
import shutil
import sqlite3

SRC = "/opt/githubhot/data/githubhot.db"
TMP = "/tmp/ghot-base.db"


def main() -> int:
    c = sqlite3.connect(SRC)
    c.execute("PRAGMA busy_timeout=20000")
    c.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    c.close()
    shutil.copyfile(SRC, TMP)
    # ubuntu 用户可读，供 scp 拉取
    import os
    os.chmod(TMP, 0o644)
    print(f"[export] {TMP} 就绪")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
