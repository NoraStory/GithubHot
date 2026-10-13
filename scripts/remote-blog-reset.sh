#!/usr/bin/env bash
# 一次性维护：清空博客条目的存量正文（转换器升级后由回填通道重抓干净版）。
# 回填速率受 CSDN WAF 节流限制（每轮流水线约 5 篇），清空后由后续流水线轮次补齐。
set -eu
DIR=${GH_DIR:-/opt/githubhot}
DB="$DIR/data/githubhot.db"
echo "清空前状态："
sudo python3 -c "
import sqlite3
con = sqlite3.connect('$DB')
n = con.execute(\"SELECT COUNT(*), SUM(LENGTH(content)>0) FROM items WHERE state='blog'\").fetchone()
print(f'blog 条目 {n[0]} 篇，其中有正文 {n[1]} 篇')
con.execute(\"UPDATE items SET content='' WHERE state='blog'\")
con.commit()
n2 = con.execute(\"SELECT COUNT(*) FROM items WHERE state='blog' AND content=''\").fetchone()
print(f'已全部清空（{n2[0]} 篇待回填）')
con.close()
"
