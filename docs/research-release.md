# norvels 公开发布 + 可持续升级：调研结论

取源：Docker 官方文档、GitHub docs、上游项目源码。本机直连 github.com / gcr.io 被重置，内容经 GitHub API 读取，均为原文。标「未验证」处未实测。

## 1. Tag 策略

**推荐组合**：`1.4.2`（SemVer 锚点）+ `1.4` / `1`（移动 tag）+ `latest`（仅 stable）+ `sha-abc1234`（每次 push）+ `edge`（main 尝鲜）。这正是 docker/metadata-action 的默认行为（优先级 semver 900 > edge 700 > ref 600 > sha 100，`latest=auto` 在 tag/semver 事件下自动生成），用官方 action 即得，无需手写。<https://github.com/docker/metadata-action>
**v0 阶段不要发 `0` tag**：官方明确「Docker tag `0` should not be generated」，因 0.y.z 允许任意破坏性变更；v1 后再开 `{{major}}`。同上（Major version zero 节）

**坑**
1. 只发 `latest`：用户无法固定/回滚/对照 release notes。
2. `latest` 指向 main：默认安装路径变不稳定通道；latest 与 edge 必须分离。
3. compose 写死 `app:latest` 又想不升级：规范原文「The latest tag is always pulled even when the missing pull policy is used」→ 用户 `up` 就意外升级。这是推荐精确 tag 的理由。<https://docs.docker.com/reference/compose-file/services/>
4. 移动 tag + Docker Hub immutable tags 全局开启：直接推不上去（"You cannot push a new image with the same tag name"）。要用就按 regex 只锁精确版本，移动 tag 例外。<https://docs.docker.com/docker-hub/repos/manage/hub-images/immutable-tags/>（Beta；本账号可用性未验证）

## 2. 多架构（amd64 + arm64）

**结论：不需要 QEMU，走交叉编译。** 官方：QEMU「can be much slower than native builds, especially for compute-heavy tasks like compilation」，并建议改用交叉编译或多原生节点。
关键动作：给 **web 和 build 两个 stage 都加** `FROM --platform=$BUILDPLATFORM`，编译发生在 runner 原生架构，靠 GOOS/GOARCH 出目标二进制。本项目 `CGO_ENABLED=0` + modernc sqlite 无 cgo 依赖，是理想场景。<https://docs.docker.com/build/building/multi-platform/>（Cross-compilation 节）
**现状差距**：`deploy/Dockerfile` 两个构建 stage 都没写 `--platform=$BUILDPLATFORM`，所以 arm64 时整个 node+pnpm build、go build 都在 QEMU 下跑，前端（rollup/esbuild）尤其慢。
**并行方案**：官方 `docker/github-builder` 的 `distribute: true` 默认把 arm 派给 `ubuntu-24.04-arm`、其余给 `ubuntu-24.04` 再聚合 manifest。<https://docs.docker.com/build/ci/github-actions/multi-platform/>；arm64 原生 runner 对公开仓库免费（2025-08-07 GA）：<https://github.blog/changelog/2025-08-07-arm64-hosted-runners-for-public-repositories-are-now-generally-available/>
**具体耗时倍数：未验证**。官方只给定性判断，我不编数字；建议先按交叉编译跑一次真实计时。

**缓存**：`cache-from: type=gha` + `cache-to: type=gha,mode=max`；多镜像用 `scope=` 隔离（默认 scope=`buildkit` 会互相覆盖）。额度：每仓库默认 10 GB、未被访问 7 天后清理。<https://docs.docker.com/build/cache/backends/gha/>、<https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching>

## 3. 体积与可复现性

**distroless 必须锁 digest**。官方：「even if a publisher updates the `3.21` tag, your builds would still use the pinned image version」，并指出不锁则「no audit trail of the exact image versions」。写法 `FROM ...:nonroot@sha256:...` + Dependabot `package-ecosystem: "docker"` 定期 PR —— 官方推荐这组合。<https://docs.docker.com/build/building/best-practices/>（Pin base images 节）
**更要紧**：`static-debian12` **已不在上游支持列表**（README 只列 `*-debian13`，并写「Any other tags are considered deprecated and are no longer updated」），且 Debian 12 安全支持已于 2026-07-11 结束。<https://github.com/GoogleContainerTools/distroless>、<https://endoflife.date/debian>（debian12 tag 是否仍推送：未验证，本机无法访问 gcr.io）
→ 迁 `static-debian13:nonroot` 并锁 digest；保留 `-debian13` 后缀（README 提醒裸 tag 将来会漂到更新的 Debian）。体积上 static 基础层约 2 MiB，当前 16–18 MiB 主要来自 Go 二进制 + 2.1 MiB healthcheck，<30 MiB 余量充足。可复现性可选 `SOURCE_DATE_EPOCH`。<https://docs.docker.com/build/ci/github-actions/reproducible-builds/>

## 4. SQLite 升级模式（同类项目）

| 项目 | 机制 | 可抄的点 |
|---|---|---|
| Navidrome (Go) | 启动 `goose.UpContext` 跑内嵌 SQL；先探测 pending migration 再打日志；迁移后 `ANALYZE` | 迁移与「是否真变了」分离，日志可观测 |
| Vaultwarden (Rust) | `embed_migrations!` + `run_pending_migrations`；备份用 `VACUUM INTO`（只读连接）；支持 `kill -USR1` 触发备份 | 备份用 `VACUUM INTO` 而非 `cp`，避开 WAL 一致性坑 |
| Immich | 启动自动迁移；文档写明「Downgrading ... is not supported」；提供 `:v3` metatag | 把"不支持降级"写成明确契约 |
| Mealie | 读 release notes → 备份 → `down` → 改 tag → `pull` → `up -d` | 升级=停-换 tag-起，迁移在启动时自动发生 |

源码：<https://github.com/navidrome/navidrome/blob/master/db/db.go>、<https://github.com/dani-garcia/vaultwarden/blob/main/src/db/mod.rs>、<https://docs.immich.app/install/upgrading/>、<https://mealie.io/documentation/getting-started/updating/>
**共同模式**：启动时自动迁移 + 版本化迁移表 + 备份命令 + 文档写清「不支持降级」。

**本项目现状的问题**：每次启动执行整份 `schema.sql`、全 `IF NOT EXISTS`、硬编码 `PRAGMA user_version = 1`。`IF NOT EXISTS` 只能加表/索引，**不能 ALTER 列、改约束、回填数据** —— docs 里已规划的 v2（去 `content_seg`、FTS 改 contentless）现在做不了。更危险的是**没有版本守卫**：新版本迁移过的库，旧二进制照样能打开写入（还会把 user_version 写回 1）。公开分发后用户一回滚 tag 就静默损坏数据。→ 改为按 `user_version` 递增的步骤表；若 `db.user_version > 程序支持版本` 直接拒绝启动并给明确错误。

## 5. 健康检查（distroless 无 shell）

- Dockerfile：shell 形式 `CMD curl ...` 需要 `/bin/sh`，distroless 没有 → 必须 exec 数组 `CMD ["/healthcheck"]`。默认 interval 30s / timeout 30s / start-period 0s / retries 3。<https://docs.docker.com/reference/dockerfile/#healthcheck>
- **compose 最容易踩的坑**：`test:` 写成**字符串**等价于 `CMD-SHELL`，又去找 shell；**必须写列表** `test: ["CMD", "/healthcheck"]`。compose 的 healthcheck「works in the same way, and has the same default values」并可覆盖镜像设置；`disable: true` / `test: ["NONE"]` 可关掉镜像内 HEALTHCHECK。<https://docs.docker.com/reference/compose-file/services/>
- **本项目两处都写对了**（Dockerfile `CMD ["/healthcheck"]`、compose `["CMD","/healthcheck"]`），是少数已达标项。
- 建议：`start_period: 10s` 对大库首次迁移偏短，改 60s，并加 `start_interval: 5s`（需 Engine 25+），start 期间失败不占 retries 预算。

## 6. 用户升级体验

**主流做法就是「镜像更新后容器重启时自动迁移」，不让用户手动跑迁移** —— 上面四个项目全部如此。手动 `docker run --rm app migrate` 在公开分发里几乎没人用（用户会忘）。
回滚三段式：
1. **迁移前自动备份**：把 `scripts/backup.sh` 的 `VACUUM INTO` 挂到「检测到 pending migration」分支，落到 `/data/backups/`。
2. **迁移失败即退出**：非零码 + 明确日志（别让 `restart: unless-stopped` 无限重启），用户看到 unhealthy 而非静默。
3. **回滚 = 换回旧 tag + 恢复备份**，README 写明"降级不支持，请用备份"（照 Immich 表述）。
`docs/deploy.md` §7 已有雏形，缺「迁移已自动发生」「不要降级」两句。

## 7. 安全基线

**必须**：nonroot（已有 65532；官方要求在非 root 时显式 `USER`，<https://docs.docker.com/build/building/best-practices/>）；基础镜像锁 digest（§3）；Docker Hub Access Token 而非密码；workflow `permissions: contents: read`。
**建议**：只读根文件系统 `read_only: true` + `tmpfs: ["/tmp"]`（本项目只写 `/data`，distroless/static 自带 /tmp，成本极低）；SBOM + provenance —— build-push-action@v4+ 对**公开仓库默认已加 max 级 provenance**，SBOM 需显式 `sbom: true`；同页警告 **provenance 会记录 build-arg 值**，密钥一律走 secret mount 不要塞 ARG。<https://docs.docker.com/build/ci/github-actions/attestations/> 补 label `org.opencontainers.image.source`（便于反查仓库）。
**可以不做**：cosign 签名（provenance 已覆盖大部分可追溯需求；Docker Hub 侧展示体验未验证）；GHCR 双发（Docker Hub 限流未认证 100 次/6h、Personal 认证 200 次/6h 时的兜底，<https://docs.docker.com/docker-hub/usage/pulls/>）。

## 8. 差距清单

**必须做**
1. 没有 `.github/workflows/` —— 发布链路（tag → build → push → release notes）整条缺失，现在只有本机 `make docker`。
2. 两个 build stage 缺 `--platform=$BUILDPLATFORM` —— arm64 会在 QEMU 下跑完 node + go 构建。
3. `static-debian12:nonroot` 已停更（上游只发 debian13，Debian 12 安全支持 2026-07-11 结束），且未锁 digest。
4. schema 无版本守卫（§4）：`user_version = 1` 硬编码、`IF NOT EXISTS` 无法表达 ALTER/回填，旧版本能静默打开新库写入。
5. compose 用本地 build tag（`image: norvels:${VERSION}` + `build:` 段）—— 发布需改 registry 全名，`build` 段移入 dev override 文件。

**建议做**
6. 迁移前自动备份（§6-1）。7. 版本单一来源（现在 Makefile / Dockerfile ARG / compose 三处各写 0.1.0）。8. CI 里把 `GOPROXY` 换成 `https://proxy.golang.org,direct`（默认 goproxy.cn）。9. `sbom: true` + `cache-to: type=gha,mode=max` + `org.opencontainers.image.source`。10. compose 注释形式加 `read_only` + `tmpfs`，`start_period` 提到 60s。11. README / deploy.md 补「升级自动迁移」「不支持降级」。

**可以不做**
12. 把 2.1 MiB 的 `/healthcheck` 合并进主程序子命令（省 <1 MiB）。13. `SOURCE_DATE_EPOCH`、cosign、GHCR 双发。14. arm64 真机冒烟（modernc sqlite 纯 Go，**预期**无差异但**未验证**）。

## 附：workflow 骨架要点

```yaml
on: { push: { tags: ["v*.*.*"], branches: [main] } }
permissions: { contents: read }
steps:
  - uses: docker/metadata-action@v5
    id: meta
    with:
      images: <hub-user>/norvels
      tags: |
        type=semver,pattern={{version}}
        type=semver,pattern={{major}}.{{minor}}
        type=semver,pattern={{major}},enable=${{ !startsWith(github.ref,'refs/tags/v0.') }}
        type=edge,branch=main
        type=sha,prefix=sha-
  - uses: docker/setup-buildx-action@v3        # 交叉编译方案下不需要 setup-qemu
  - uses: docker/build-push-action@v6
    with:
      platforms: linux/amd64,linux/arm64
      push: true
      tags: ${{ steps.meta.outputs.tags }}
      labels: ${{ steps.meta.outputs.labels }}
      provenance: mode=max
      sbom: true
      cache-from: type=gha
      cache-to: type=gha,mode=max
      build-args: |
        VERSION=${{ steps.meta.outputs.version }}
        COMMIT=${{ github.sha }}
        GOPROXY=https://proxy.golang.org,direct
```
`{{major}}` 的 `enable` 表达式来自 metadata-action 官方「Major version zero」示例。