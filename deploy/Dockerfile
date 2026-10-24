# syntax=docker/dockerfile:1.7
# norvels 多阶段构建：前端(node) → 后端(golang, CGO_ENABLED=0) → 运行(distroless/static, nonroot)
#
# 目标镜像 < 50MB（体积拆解见 docs/deploy.md）。
# 构建（构建上下文必须是仓库根目录）：
#   docker build -f deploy/Dockerfile -t norvels:0.1.0 .
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploy/Dockerfile -t norvels:0.1.0 .
#
# 说明：
#   - Go 版本取 go.mod 的 go 1.26：modernc.org/sqlite 要求 1.25，
#     解析器依赖的 golang.org/x/text v0.43 要求 1.26，因此默认 golang:1.26-alpine。
#   - 本文件使用 BuildKit 特性（RUN --mount、syntax 指令）。Docker 23+ 默认
#     启用 BuildKit；旧构建器可删除 --mount 段，只损失缓存速度。

# 全局参数：必须在第一个 FROM 之前声明，才能在 FROM 中引用。
ARG GO_VERSION=1.26
ARG PNPM_VERSION=9

# ---------- 阶段 1：前端构建（React + Vite + Tailwind） ----------
FROM node:22-alpine AS web
ARG PNPM_VERSION
WORKDIR /src/web
# 固定 pnpm 主版本，避免 corepack 在受限网络下协商版本失败。
RUN npm install -g pnpm@${PNPM_VERSION}

# 先只复制依赖清单（lock 文件可能尚未生成）：bind mount 读取 context，
# 避免 "COPY web/pnpm-lock.yaml*" 在文件不存在时直接失败。
COPY web/package.json ./
RUN --mount=type=bind,source=web,target=/tmp/web-src \
    if [ -f /tmp/web-src/pnpm-lock.yaml ]; then cp /tmp/web-src/pnpm-lock.yaml ./; fi \
 && if [ -f pnpm-lock.yaml ]; then \
      echo "==> pnpm install --frozen-lockfile"; pnpm install --frozen-lockfile; \
    else \
      echo "==> 未找到 pnpm-lock.yaml，改用 pnpm install（提交 lockfile 后自动走 frozen）"; pnpm install; \
    fi

# 再复制其余源码：源码变更不会击穿依赖缓存。
COPY web/ ./
RUN pnpm build

# ---------- 阶段 2：后端静态编译（CGO_ENABLED=0，纯 Go SQLite） ----------
FROM golang:${GO_VERSION}-alpine AS build
ARG VERSION=0.1.0
ARG COMMIT=dev
ARG BUILTAT=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG GOPROXY=https://goproxy.cn,direct

ENV CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    GOPROXY=${GOPROXY} \
    GOTOOLCHAIN=local

WORKDIR /src

# 依赖层：go.mod/go.sum 不变时复用缓存。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# 源码层：.dockerignore 已排除 web/node_modules、web/dist、internal/webui/dist、data。
COPY . .
# 把阶段 1 的构建产物放进 embed 目录（webui.go 用 //go:embed all:dist）。
COPY --from=web /src/web/dist ./web/dist
RUN rm -rf internal/webui/dist && mkdir -p internal/webui/dist && cp -R web/dist/. internal/webui/dist/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.builtAt=${BUILTAT}" \
      -o /out/norvels ./cmd/norvels \
 && go build -trimpath -ldflags "-s -w" \
      -o /out/healthcheck ./cmd/healthcheck \
 && mkdir -p /out/data \
 && touch /out/data/.keep \
 && chown -R 65532:65532 /out/data

# ---------- 阶段 3：运行（distroless/static，非 root，无 shell） ----------
FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=0.1.0
ARG COMMIT=dev
ARG BUILTAT=unknown

LABEL org.opencontainers.image.title="norvels" \
      org.opencontainers.image.description="本地中文小说库：导入、阅读、全文搜索" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILTAT}" \
      org.opencontainers.image.licenses="MIT"

ENV NORVELS_ADDR=:8080 \
    NORVELS_DB=/data/norvels.db \
    NORVELS_HEALTH_URL=http://127.0.0.1:8080/api/health \
    TZ=Asia/Shanghai

COPY --from=build /out/norvels /norvels
# distroless 内没有 shell/curl/wget，健康检查用编译好的静态小工具。
COPY --from=build /out/healthcheck /healthcheck
# /data 由 COPY --chown 创建，属主为 nonroot(65532)。注意顺序：WORKDIR 若在 COPY
# 之前声明会先以 root 创建 /data，导致非 root 用户无法写库。
COPY --from=build --chown=65532:65532 /out/data /data

WORKDIR /data
USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/healthcheck"]

ENTRYPOINT ["/norvels", "serve"]
