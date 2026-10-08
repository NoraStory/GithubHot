#!/usr/bin/env bash
# 远端信源诊断（由 infra-source-diag.yml 经 stdin 喂给 bash -s 执行）
# 1) 列出全部信源（名称/URL/启用）2) 从服务器实测每个 feed URL 的可达性
# 3) 额外测 hf-mirror 镜像可达性（huggingface 的境内替换）
set -e
DB=/opt/githubhot/data/githubhot.db

python3 - "$DB" <<'PY'
import json
import sqlite3
import subprocess
import sys

db = sqlite3.connect(sys.argv[1])
rows = db.execute("SELECT id, name, kind, config, enabled FROM sources ORDER BY enabled DESC, name").fetchall()
urls = []
print("=== 信源清单 ===")
for sid, name, kind, config, enabled in rows:
    url = ""
    try:
        cfg = json.loads(config)
        url = cfg.get("url") or cfg.get("feed_url") or ""
    except Exception:
        pass
    flag = "启用" if enabled else "停用"
    print(f"  [{flag}] {name} ({kind}) {url}")
    if enabled and url.startswith("http"):
        urls.append((name, url))

print("\n=== 服务器实测可达性（8s 超时）===")
for name, url in urls:
    r = subprocess.run(
        ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code} %{time_total}s",
         "--max-time", "8", "-L", "-A", "Mozilla/5.0 (compatible; GithubHot/1.0)", url],
        capture_output=True, text=True)
    out = r.stdout.strip() or "FAIL"
    mark = "✓" if out.startswith("2") or out.startswith("3") else "✗"
    print(f"  {mark} {out:<18} {name}  {url}")

print("\n=== 镜像/中转可达性 ===")
for label, u in [
    ("hf-mirror.com（huggingface 境内镜像）", "https://hf-mirror.com/blog/feed.xml"),
]:
    r = subprocess.run(
        ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code} %{time_total}s", "--max-time", "8", "-L", u],
        capture_output=True, text=True)
    print(f"  {r.stdout.strip() or 'FAIL':<18} {label}")
PY
