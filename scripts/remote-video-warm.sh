#!/usr/bin/env bash
# 背景视频预热（GitHub Release 中转通道）：
# pic.lololowe.com 对数据中心连接 ~3MB 处断流且拒绝 Range 续传（断点续传实测
# 30 轮零进展），服务器直拉永远拿不到完整文件 → 生产只能 302 兜底给浏览器直连慢源。
# 本脚本从仓库 Release（bgvideo-x1x6，由本地完整拉取后上传）经 gh-proxy 镜像
# 下载 6 个横屏视频，校验后安装到 data/videos/x/ 权威位置——serveVideoCached
# 优先直出，无需重启。
set -u
DIR=${GH_DIR:-/opt/githubhot}
REL="https://gh-proxy.com/https://github.com/NoraStory/GithubHot/releases/download/bgvideo-x1x6"
UA="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
WORK=/tmp/video-warm
mkdir -p "$WORK/x"

ok=0; fail=0
for n in 1 2 3 4 5 6; do
  dest="$WORK/x/$n.mp4"
  curl -sL --retry 3 --max-time 600 -A "$UA" -o "$dest" "$REL/x$n.mp4" || true
  size=$(stat -c%s "$dest" 2>/dev/null || echo 0)
  if [ "$size" -gt 1048576 ] && head -c 12 "$dest" | grep -aq ftyp; then
    echo "x$n.mp4 下载完成: $size bytes"
    ok=$((ok+1))
  else
    echo "x$n.mp4 下载失败（$size bytes）"
    fail=$((fail+1))
  fi
done
echo "=== 下载成功 $ok / 失败 $fail ==="

if [ "$ok" -gt 0 ]; then
  sudo mkdir -p "$DIR/data/videos/x"
  for n in 1 2 3 4 5 6; do
    if [ -s "$WORK/x/$n.mp4" ] && head -c 12 "$WORK/x/$n.mp4" | grep -aq ftyp; then
      sudo install -m 0644 "$WORK/x/$n.mp4" "$DIR/data/videos/x/$n.mp4" && echo "已安装 x/$n.mp4"
    fi
  done
fi
ls -la "$DIR/data/videos/x/" 2>/dev/null || true
rm -rf "$WORK"
