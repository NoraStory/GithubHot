#!/usr/bin/env bash
# 背景视频断点续传预热：x1-x6 只在主题作者源（pic.lololowe.com），
# 该 CDN 对数据中心连接中途断流（实测 ~3MB 处断），单次拉取永远拿不到完整文件
# → 生产上这 6 个视频只能 302 兜底给浏览器直连慢源。
# 解法：curl -C - 循环断点续传（依赖 CDN Range 支持），Content-Length 校验完整性，
# 落到 data/videos/x/<n>.mp4 权威位置（serveVideoCached 优先直出，无需重启）。
set -u
DIR=${GH_DIR:-/opt/githubhot}
UA="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
WORK=/tmp/video-warm
mkdir -p "$WORK/x"

fetch_expect() { # HEAD 拿 Content-Length
  curl -sI -A "$UA" -e "https://www.githubhot.online/" --max-time 20 "$1" \
    | grep -i '^content-length' | tail -1 | awk '{print $2}' | tr -d '\r'
}

ok=0; fail=0
for n in 1 2 3 4 5 6; do
  dest="$WORK/x/$n.mp4"
  url="https://pic.lololowe.com/video/x/$n.mp4"
  expect=$(fetch_expect "$url")
  if [ -z "$expect" ]; then echo "x/$n.mp4: HEAD 拿不到长度，跳过"; fail=$((fail+1)); continue; fi
  rm -f "$dest"
  done_flag=""
  prev=0
  for attempt in $(seq 1 30); do
    curl -s -C - -A "$UA" -e "https://www.githubhot.online/" --max-time 300 -o "$dest" "$url" || true
    size=$(stat -c%s "$dest" 2>/dev/null || echo 0)
    if [ "$size" = "$expect" ] && head -c 12 "$dest" | grep -q ftyp; then
      echo "x/$n.mp4 完成：$size bytes（续传 $attempt 轮）"
      done_flag=1; break
    fi
    if [ "$size" -le "$prev" ]; then
      echo "  x/$n.mp4 第 $attempt 轮无进展（$size/$expect），退避后重试"
      sleep 5
    else
      echo "  x/$n.mp4 第 $attempt 轮：$size/$expect"
    fi
    prev=$size
    sleep 2
  done
  if [ -n "$done_flag" ]; then ok=$((ok+1)); else echo "x/$n.mp4 预热失败（30 轮未拉完）"; fail=$((fail+1)); fi
done

echo "=== 结果：成功 $ok / 失败 $fail，安装到 $DIR/data/videos/x/ ==="
if [ "$ok" -gt 0 ]; then
  sudo mkdir -p "$DIR/data/videos/x"
  for n in 1 2 3 4 5 6; do
    if head -c 12 "$WORK/x/$n.mp4" 2>/dev/null | grep -q ftyp; then
      sudo install -m 0644 "$WORK/x/$n.mp4" "$DIR/data/videos/x/$n.mp4" && echo "已安装 x/$n.mp4"
    fi
  done
fi
ls -la "$DIR/data/videos/x/" 2>/dev/null || true
rm -rf "$WORK"
