#!/bin/bash
# 把 overrides.conf 里的 ${DOUBAO_API_KEY} 引用替换为真实密钥值
set -e
DIR=/etc/systemd/system/githubhot.service.d
KEY=$(grep -h "^Environment=DOUBAO_API_KEY=" "$DIR"/override.conf | head -1 | cut -d= -f3-)
[ -n "$KEY" ] || { echo "找不到 DOUBAO_API_KEY 值"; exit 1; }
sed -i "s|Environment=LLM_API_KEY=\${DOUBAO_API_KEY}|Environment=LLM_API_KEY=$KEY|; s|Environment=LLM_EMBED_API_KEY=\${DOUBAO_API_KEY}|Environment=LLM_EMBED_API_KEY=$KEY|" "$DIR"/overrides.conf
grep -c "DOUBAO_API_KEY}" "$DIR"/overrides.conf || echo "替换完成：无剩余引用"
