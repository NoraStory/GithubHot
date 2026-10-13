#!/usr/bin/env bash
# 博客回填续闸：还有缺正文的 blog 条目时，把 csdn-blog 信源强制置为到期
# （last_fetched_at=NULL）并临时缩短采集间隔，让下一轮流水线重新抓 feed 触发
# 回填通道；全部有正文后自动恢复 240 分钟间隔。配合流水线 workflow 反复触发。
set -eu
DIR=${GH_DIR:-/opt/githubhot}
DB="$DIR/data/githubhot.db"
sudo python3 -c "
import sqlite3
con = sqlite3.connect('$DB')
total, missing = con.execute(\"SELECT COUNT(*), SUM(LENGTH(content)=0) FROM items WHERE state='blog'\").fetchone()
print(f'blog 条目 {total} 篇，缺正文 {missing} 篇')
if missing:
    con.execute(\"UPDATE sources SET last_fetched_at=NULL, interval_minutes=10 WHERE id='csdn-blog'\")
    print('csdn-blog 已重置为到期（间隔临时 10 分钟）')
else:
    con.execute(\"UPDATE sources SET interval_minutes=240 WHERE id='csdn-blog'\")
    print('全部正文齐备，间隔已恢复 240 分钟')
con.commit()
con.close()
"
