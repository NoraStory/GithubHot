#!/usr/bin/env bash
# 博客正文抓取诊断（只读）：从服务器实测 CSDN 文章页/RSS 的可达性，
# 复现 fetcher 的请求形态（同 UA / Accept），并检查服务进程的代理环境变量。
set -u
UA="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

echo "===== 1. 服务进程环境（HTTPS_PROXY/代理相关） ====="
PID=$(systemctl show -p MainPID --value githubhot 2>/dev/null)
echo "MainPID=$PID"
if [ -n "$PID" ] && [ "$PID" != "0" ]; then
  tr '\0' '\n' < "/proc/$PID/environ" 2>/dev/null | grep -iE 'proxy|llm_base' | sed 's/\(KEY=\).*/\1***/' || echo "(无代理相关环境变量)"
fi

echo "===== 2. 服务 systemd 单元代理配置 ====="
systemctl cat githubhot 2>/dev/null | grep -iE 'proxy' || echo "(单元无 proxy 配置)"

echo "===== 3. RSS feed（rss.csdn.net，已知成功） ====="
curl -s -o /tmp/diag_rss.xml -w "HTTP %{http_code} size %{size_download} time %{time_total}s\n" \
  -A "$UA" -H "Accept: application/rss+xml, application/xml, text/xml, */*" --max-time 20 \
  "https://rss.csdn.net/storyseekee/rss/map" || true

echo "===== 4. 文章页（blog.csdn.net，生产疑似失败） ====="
for u in "https://blog.csdn.net/storyseekee/article/details/166009937" "https://blog.csdn.net/storyseekee/article/details/164334713"; do
  code=$(curl -s -o /tmp/diag_art.html -w "%{http_code}" -A "$UA" -H "Accept: text/html" --max-time 25 "$u" || echo "curl-fail")
  size=$(wc -c < /tmp/diag_art.html 2>/dev/null || echo 0)
  cv=$(grep -c "content_views" /tmp/diag_art.html 2>/dev/null || echo 0)
  title=$(grep -oE "<title>[^<]*" /tmp/diag_art.html 2>/dev/null | head -1 | cut -c8-80)
  echo "HTTP $code | size $size | content_views=$cv | title: $title"
  head -c 200 /tmp/diag_art.html | tr -d '\n' | head -c 200; echo
done

echo "===== 5. DNS 解析对比 ====="
python3 -c "import socket; print('blog.csdn.net ->', sorted({a[4][0] for a in socket.getaddrinfo('blog.csdn.net', 443)}))" 2>/dev/null || getent hosts blog.csdn.net
python3 -c "import socket; print('rss.csdn.net ->', sorted({a[4][0] for a in socket.getaddrinfo('rss.csdn.net', 443)}))" 2>/dev/null || getent hosts rss.csdn.net

echo "===== 6. 服务日志中的 collect 记录（近 200 行筛 csdn/blog） ====="
sudo journalctl -u githubhot -n 400 --no-pager 2>/dev/null | grep -iE "csdn|blog|collect" | tail -20 || true
rm -f /tmp/diag_rss.xml /tmp/diag_art.html
