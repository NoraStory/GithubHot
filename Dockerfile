# 多阶段构建：产物为约 20MB 的静态二进制（CGO=0，纯 Go SQLite）
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/githubhot ./cmd/githubhot

FROM alpine:3.20
RUN adduser -D -u 10001 ghot
WORKDIR /data
COPY --from=build /out/githubhot /usr/local/bin/githubhot
USER ghot
ENV DATA_DIR=/data PORT=8787
EXPOSE 8787
# serve 模式：API + 双榜页 + 内置定时调度
#
# ⚠️ serve 模式强制要求 APP_SIGN_SEED（防出厂默认种子上线，见 README 配置表）：
# 未配置或仍为出厂默认 gh-dev-seed-v1 时拒绝启动（fail-fast，属设计而非缺陷）。
# 正确启动姿势（一行示例）：
#   SEED=$(docker run --rm <镜像> admin seed)
#   docker run -e APP_SIGN_SEED="$SEED" -e LLM_BASE_URL=... -e LLM_API_KEY=... -e LLM_MODEL=... -v githubhot-data:/data -p 8787:8787 <镜像>
# 数据目录 /data 需挂卷（SQLite、日报、GeoIP 库均在其中）。
CMD ["githubhot", "serve"]
