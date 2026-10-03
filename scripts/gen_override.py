# 从本机 .env 提取流水线所需配置，生成服务器 systemd override 片段
# 用法: python gen_override.py  输出到 override.conf 片段文件（不打印密钥值）
import re
from pathlib import Path

env = Path(r"E:\桌面管理\GithubHot\.env").read_text(encoding="utf-8")
want_prefixes = (
    "LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_MODEL_B", "LLM_THINKING",
    "LLM_EMBED_BASE_URL", "LLM_EMBED_API_KEY", "LLM_EMBED_MODEL", "LLM_EMBED_STYLE",
    "LLM_EMBED_DIMENSIONS", "GITHUB_TOKEN",
)
lines = []
for raw in env.splitlines():
    raw = raw.strip()
    if not raw or raw.startswith("#") or "=" not in raw:
        continue
    k, v = raw.split("=", 1)
    if k in want_prefixes and v:
        lines.append(f"Environment={k}={v}")

out = Path(r"E:\桌面管理\GithubHot\scripts\override-fragment.conf")
out.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"生成 {out}，共 {len(lines)} 行配置（密钥未打印）")
