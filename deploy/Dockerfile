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
# pnpm 版本必须与 web/pnpm-workspace.yaml 的特性匹配：
# 该文件用了 allowBuilds（pnpm 10+ 才有），pnpm 9 会直接报
# "packages field missing or empty"。本地与 CI 都用 11，这里保持一致。
ARG PNPM_VERSION=11

# 交叉编译约定（重要）：
#   构建 stage 一律声明 --platform=$BUILDPLATFORM，只在「跑在构建机上」的处理器架构执行；
#   目标架构由 ARG TARGETOS/TARGETARCH 传给 go build（CGO_ENABLED=0 纯 Go，天然可交叉编译）。
#   不声明时 buildx 会把整个 stage 跑在目标平台上：arm64 构建时 node/pnpm 与 go build
#   全在 QEMU 模拟下执行，耗时慢数倍。运行 stage 不声明，才会按目标架构拉基础镜像。
#   参照：docs/deploy.md §2.2 与 https://docs.docker.com/build/building/multi-platform/

# ---------- 阶段 1：前端构建（React + Vite + Tailwind） ----------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
ARG PNPM_VERSION
WORKDIR /src/web
# 固定 pnpm 主版本，避免 corepack 在受限网络下协商版本失败。
RUN npm install -g pnpm@${PNPM_VERSION}

# 先只复制依赖清单（lock 文件可能尚未生成）：bind mount 读取 context，
# 避免 "COPY web/pnpm-lock.yaml*" 在文件不存在时直接失败。
#
# 注意：pnpm-workspace.yaml 必须在 install 之前复制。它承载 allowBuilds
# （放行 esbuild 等依赖的构建脚本）。缺了它 pnpm 会以
# [ERR_PNPM_IGNORED_BUILDS] 失败退出——本地有该文件所以复现不出来，
# 只在干净容器里暴露（已由 scripts/check-build-parity.sh 纳入门禁）。
COPY web/package.json ./
RUN --mount=type=bind,source=web,target=/tmp/web-src \
    if [ -f /tmp/web-src/pnpm-lock.yaml ]; then cp /tmp/web-src/pnpm-lock.yaml ./; fi \
 && if [ -f /tmp/web-src/pnpm-workspace.yaml ]; then cp /tmp/web-src/pnpm-workspace.yaml ./; fi \
 && if [ -f pnpm-lock.yaml ]; then \
      echo "==> pnpm install --frozen-lockfile"; pnpm install --frozen-lockfile; \
    else \
      echo "==> 未找到 pnpm-lock.yaml，改用 pnpm install（提交 lockfile 后自动走 frozen）"; pnpm install; \
    fi

# 再复制其余源码：源码变更不会击穿依赖缓存。
COPY web/ ./
RUN pnpm build

# ---------- 阶段 2：后端静态编译（CGO_ENABLED=0，纯 Go SQLite） ----------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG VERSION=0.1.0
ARG COMMIT=dev
ARG BUILTAT=unknown
# buildx 会按目标平台自动填充这两个 ARG；单架构构建时用默认值。
ARG TARGETOS=linux
ARG TARGETARCH=amd64
# 默认走官方代理（CI 在 GitHub runner 上）；国内本地构建可覆盖为
# --build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY=https://proxy.golang.org,direct

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
# 基础镜像：distroless static-debian13（上游只维护 debian13；debian12 已停更，
# 其安全支持窗口到 2026-07-11 结束）。
# 锁 digest 的理由：仓库里写 tag 只能证明「当时拉到的是某个镜像」，锁 @sha256 才能让
# 同一份源码在任何时间、任何机器上构建出同一层基础镜像，也让升级基镜像变成一次可审计的
# 显式改动（Dependabot 会以 PR 形式提出 digest 更新，见 .github/dependabot.yml）。
FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
ARG VERSION=0.1.0
ARG COMMIT=dev
ARG BUILTAT=unknown

LABEL org.opencontainers.image.title="norvels" \
      org.opencontainers.image.description="本地中文小说库：导入、阅读、全文搜索" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILTAT}" \
      org.opencontainers.image.source="https://github.com/baosfeng/norvels" \
      org.opencontainers.image.documentation="https://github.com/baosfeng/norvels/blob/main/docs/deploy.md" \
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

# start-period 给足 60s：大库首次迁移（上千万章节的 FTS 建索引）可能远超 10s，
# 期间健康检查失败会触发 restart 循环，看起来像「启动即崩」。
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD ["/healthcheck"]

ENTRYPOINT ["/norvels", "serve"]
