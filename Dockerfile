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
CMD ["githubhot", "serve"]
