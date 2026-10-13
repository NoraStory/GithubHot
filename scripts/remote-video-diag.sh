#!/usr/bin/env bash
# 背景视频回源诊断（只读）：从服务器实测 x/y 两个源站的可达性，
# 检查磁盘缓存目录现状与磁盘空间——定位横屏视频 302 兜底、竖屏正常的差异根因。
set -u
UA="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

echo "===== 1. 磁盘空间与视频缓存目录 ====="
df -h / | tail -1
DIR=${GH_DIR:-/opt/githubhot}
ls -la "$DIR/data/videos/" 2>/dev/null | head -8
echo "--- cache/ 落盘文件 ---"
ls -la "$DIR/data/videos/cache/" 2>/dev/null | head -30
echo "--- 权威位置 videos/x|y ---"
ls -la "$DIR/data/videos/x/" "$DIR/data/videos/y/" 2>/dev/null | head -20

echo "===== 2. 主题作者源 pic.lololowe.com（x1-x6 的源） ====="
curl -s -o /tmp/v1.mp4 -w "x/1.mp4: HTTP %{http_code} | CT %{content_type} | %{size_download}B | %{time_total}s | 速度 %{speed_download}B/s\n" \
  -A "$UA" -H "Referer: https://www.githubhot.online/" -H "Accept: */*" --max-time 60 "https://pic.lololowe.com/video/x/1.mp4" || echo "curl 失败（连接层错误）"
head -c 120 /tmp/v1.mp4 | tr -d '\0' | head -c 120; echo
rm -f /tmp/v1.mp4

echo "===== 3. 自家 R2（x7+/y 的源，r2Base 从环境读） ====="
PID=$(systemctl show -p MainPID --value githubhot 2>/dev/null)
echo "MainPID=$PID"
sudo tr '\0' '\n' < "/proc/$PID/environ" 2>/dev/null | grep -iE "R2|VIDEO|CDN" | sed 's/\(KEY\|SECRET\|TOKEN\)=.*/\1***/' || echo "(读不到进程环境或无相关变量)"

echo "===== 4. 服务日志中的视频记录（近 500 行筛 video/302/缓存） ====="
sudo journalctl -u githubhot -n 500 --no-pager 2>/dev/null | grep -iE "video|x/1|y1" | tail -15 || echo "(无视频相关日志)"
