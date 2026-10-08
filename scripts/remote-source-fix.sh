#!/usr/bin/env bash
# 远端被墙信源换镜像（由 infra-source-fix.yml 经 stdin 喂给 bash -s 执行）
# 对每个被墙源测试候选镜像 URL，首个 2xx/3xx 者写回 sources.config；全部不通则保留原状并报告。
set -e
DB=/opt/githubhot/data/githubhot.db

python3 - "$DB" <<'PY'
import json
import sqlite3
import subprocess
import sys

db = sqlite3.connect(sys.argv[1])

# 被墙源 → 候选镜像（按优先级）
fixes = [
    ("Hugging Face Blog", ["https://hf-mirror.com/blog/feed.xml"]),
    ("TechCrunch AI", ["https://feeds.feedburner.com/TechCrunch"]),
    ("Google Research", ["https://feeds.feedburner.com/blogspot/gJZg"]),
    ("Import AI", ["https://rsshub.rssforever.com/substack/importai"]),
    ("Mistral AI", []),
]

def test(url):
    r = subprocess.run(
        ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code} %{time_total}s",
         "--max-time", "8", "-L", "-A", "Mozilla/5.0 (compatible; GithubHot/1.0)", url],
        capture_output=True, text=True)
    code = (r.stdout.strip() or "000").split()[0]
    return code.startswith(("2", "3")), r.stdout.strip()

print("=== 换源结果 ===")
for name, candidates in fixes:
    cur = db.execute("SELECT config FROM sources WHERE name = ?", (name,)).fetchone()
    if not cur:
        print(f"  [跳过] {name}: 信源不存在")
        continue
    cfg = json.loads(cur[0])
    old_url = cfg.get("url", "")
    swapped = None
    for cand in candidates:
        ok, info = test(cand)
        print(f"  测试 {cand} → {info} {'✓' if ok else '✗'}")
        if ok:
            swapped = cand
            break
    if swapped:
        cfg["url"] = swapped
        db.execute("UPDATE sources SET config = ? WHERE name = ?", (json.dumps(cfg, ensure_ascii=False), name))
        print(f"  [换源] {name}: {old_url} → {swapped}")
    else:
        print(f"  [未修] {name}: 无可用候选（原 {old_url} 保持，等待代理或人工替换）")
db.commit()

print("=== 换源后复测（全部启用源）===")
rows = db.execute("SELECT name, config FROM sources WHERE enabled = 1").fetchall()
fails = 0
for name, config in rows:
    url = ""
    try:
        url = json.loads(config).get("url", "")
    except Exception:
        pass
    if not url.startswith("http"):
        continue
    r = subprocess.run(
        ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "8", "-L", url],
        capture_output=True, text=True)
    code = r.stdout.strip() or "000"
    if not code.startswith(("2", "3")):
        fails += 1
        print(f"  ✗ {code}  {name}")
print(f"复测结论：仍不可达 {fails} 个")
PY
