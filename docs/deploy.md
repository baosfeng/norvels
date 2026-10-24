# 部署指南

本文覆盖 norvels 的容器构建、体积构成、运行、SQLite 数据卷注意事项、资源建议与升级回滚。
接口与架构分别见 [api.md](api.md) 与 [architecture.md](architecture.md)。

## 1. 交付物一览

| 文件 | 作用 |
|---|---|
| `deploy/Dockerfile` | 三阶段构建：node 构建前端 → golang 静态编译 → distroless/static 运行 |
| `deploy/compose.yml` | 单服务编排：端口 8080、`./data` 数据卷、小说目录只读挂载、健康检查、自动重启 |
| `scripts/import.sh` | 批量导入（本地二进制 / docker 两种模式），带校验、耗时统计、导入后 stats |
| `scripts/backup.sh` | SQLite 在线备份（`.backup` 或 `VACUUM INTO`）+ 时间戳命名 + 保留策略 |
| `cmd/healthcheck` | 2.1 MiB 静态健康检查器，供 distroless 镜像的 HEALTHCHECK 使用 |
| `.dockerignore` | 排除 node_modules、dist、data、testdata 等，构建上下文更小 |

## 2. 构建

构建上下文必须是**仓库根目录**（Dockerfile 在 `deploy/`，但需要 `web/`、`go.mod`、`internal/`）：

```bash
# 单架构（amd64，默认）
docker build -f deploy/Dockerfile -t norvels:0.1.0 .

# 多架构（buildx）
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f deploy/Dockerfile \
  -t norvels:0.1.0 --load .
```

### 2.1 构建参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `GO_VERSION` | `1.26` | Go 工具链镜像 tag。`go.mod` 声明 `go 1.26.0`：`modernc.org/sqlite` 要求 go>=1.25，解析器使用的 `golang.org/x/text v0.43` 要求 go>=1.26，因此默认 1.26 |
| `PNPM_VERSION` | `9` | 前端包管理器主版本 |
| `VERSION` | `0.1.0` | 注入 `main.version`，显示在 `/api/version` 与 `norvels version` |
| `COMMIT` | `dev` | 注入 `main.commit` |
| `BUILTAT` | `unknown` | 注入 `main.builtAt`（RFC3339） |
| `TARGETOS` / `TARGETARCH` | `linux` / `amd64` | buildx 自动传入，交叉编译目标 |
| `GOPROXY` | `https://goproxy.cn,direct` | 国内代理；海外构建可改为 `https://proxy.golang.org,direct` |

示例：

```bash
docker build -f deploy/Dockerfile \
  --build-arg VERSION=0.2.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILTAT=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t norvels:0.2.0 .
```

### 2.2 各阶段职责

1. **web（node:22-alpine）**：先只复制 `web/package.json`（及 `pnpm-lock.yaml`，若存在），通过 bind mount 探测 lock 文件后执行 `pnpm install --frozen-lockfile`（缺 lock 时退回 `pnpm install`），再复制源码执行 `pnpm build`。
2. **build（golang:1.26-alpine）**：`CGO_ENABLED=0`、`-trimpath`、`-ldflags "-s -w"`；把 `web/dist` 同步进 `internal/webui/dist`（`//go:embed all:dist` 的目标），编译 `./cmd/norvels` 与 `./cmd/healthcheck`。
3. **runtime（gcr.io/distroless/static-debian12:nonroot）**：只拷二进制、healthcheck 与 `/data` 目录，非 root（65532）运行，无 shell。

### 2.3 已知限制（重要）

> **本机未实测镜像构建。** 当前开发机（macOS）没有 docker / podman / colima / nerdctl，
> 因此本目录的镜像体积来自「交叉编译实测二进制 + 基镜像官方体积」，不是 `docker images` 实测值。
> 已完成的验证：
>
> - `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w"` 实测通过，二进制 **11.1 MiB**（含 `golang.org/x/text` 的 GB18030/Big5 码表；`internal/webui/dist` 目前只有占位文件，前端产物合入后预计 12–13 MiB）；
> - `go vet ./cmd/healthcheck` + `go test ./cmd/healthcheck` 通过，二进制 **2.1 MiB**；
> - 端到端实测：8 个真实 txt（5.09 MiB）导入成功（397 章 / 1,733,881 字），30 个真实 txt（13.2 MiB）导入成功（1072 章 / 4,380,382 字）；重复导入全部 skipped（幂等）；`serve` + `/healthcheck` 对正常与不可达地址分别返回 0 / 1；
> - Dockerfile 做静态自查（阶段、ARG 作用域、路径与 Makefile 一致、引号与行连续）。
>
> 在有 Docker 的机器上，请以 `docker images norvels:0.1.0` 的实际输出为准，并回来更新下表。
> 另外 `web/` 前端仍在并行开发，`web/dist` 体积按 Vite + React 19 + Tailwind 的常见区间估算。

### 2.4 镜像内二进制与构建参数

最终镜像只包含两个静态可执行文件，均在第 2 阶段编译、第 3 阶段 COPY 进运行镜像：

| 文件 | 来源 | 构建参数 | 体积 |
|---|---|---|---|
| `/norvels` | `./cmd/norvels` | 继承阶段 ENV `CGO_ENABLED=0`、`GOOS/GOARCH`；`-trimpath`；`-ldflags "-s -w -X main.version=... -X main.commit=... -X main.builtAt=..."` | **11.1 MiB**（linux/amd64 本机实测，含 x/text；前端合入后预计 12–13 MiB） |
| `/healthcheck` | `./cmd/healthcheck` | 继承 `CGO_ENABLED=0`、`GOOS/GOARCH`；`-trimpath`；`-ldflags "-s -w"`（无版本注入） | 2.1 MiB（实测） |

`/healthcheck` 是手写的最小 HTTP/1.1 客户端（只用 `net`、`bufio`、`net/url`，不引入 `net/http`/`crypto/tls`），
因此镜像里不需要 shell、curl 或 CA 证书；它请求回环地址 `http://127.0.0.1:8080/api/health`，按 JSON 中 `status == "ok"` 判定健康。

## 3. 镜像体积拆解

| 组成 | 大小 | 来源 |
|---|---:|---|
| `/norvels`（Go 二进制） | **11.1 MiB** | 本机实测（linux/amd64、完整实现、含 x/text；`web/dist` 目前为占位，前端合入后预计 12–13 MiB） |
| `/healthcheck` | **2.1 MiB** | 本机实测（手写最小 HTTP 客户端，不引入 net/http） |
| distroless/static-debian12:nonroot 基础层 | **约 2.0–2.5 MiB** | 官方基镜像（无 shell、无包管理器，仅 ca-certificates、/etc/passwd、tzdata） |
| **合计（估算）** | **约 15.5–16.5 MiB**（前端合入后约 16.5–18 MiB） | 目标 < 50 MiB，余量充足 |

> 前端产物通过 `go:embed` 打进二进制，不额外占镜像层；上表合计 = 二进制 + healthcheck + 基础层。
> 实测命令：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o norvels ./cmd/norvels`（11,681,952 字节），healthcheck 2,248,864 字节。

体积控制手段：

- `CGO_ENABLED=0` + `modernc.org/sqlite`（纯 Go），静态链接，无需 libc；
- `-trimpath` 去掉构建路径，`-ldflags "-s -w"` 去掉符号表与 DWARF；
- distroless/static 无 shell、apt、curl，比 alpine 更小且攻击面更低；
- `.dockerignore` 排除 `node_modules`、`web/dist`、`internal/webui/dist`、`data/`、`testdata/`。

## 4. 运行

### 4.1 compose（推荐）

```bash
# 1) 构建镜像
docker build -f deploy/Dockerfile -t norvels:0.1.0 .

# 2) 先导入（服务尚未启动，避免单写者冲突）
NORVELS_BOOKS_DIR="/path/to/txt" \
  docker compose -f deploy/compose.yml run --rm --entrypoint /norvels \
  norvels import --dir /books --db /data/norvels.db

# 3) 启动
NORVELS_BOOKS_DIR="/path/to/txt" docker compose -f deploy/compose.yml up -d
```

- 端口：宿主机 `8080`（`NORVELS_PORT` 可改）→ 容器 `8080`；
- 数据卷：宿主 `./data` → 容器 `/data`，数据库固定 `/data/norvels.db`；
- 小说目录：`NORVELS_BOOKS_DIR` → 容器 `/books:ro`，**只读**，导入器不会修改源文件；
- 环境变量：`NORVELS_ADDR=:8080`、`NORVELS_DB=/data/norvels.db`、`NORVELS_HEALTH_URL`；
- 重启策略：`restart: unless-stopped`；健康检查：`/healthcheck` 请求 `/api/health`。

### 4.2 首次挂载后的权限

运行阶段是 `nonroot`（uid/gid 65532）。`./data` 由 Docker 首次自动创建时属主通常是 root，
在 Linux 上会因不可写而启动失败。二选一：

```bash
# 方式 A：把数据目录交给 65532（推荐）
mkdir -p data && sudo chown -R 65532:65532 data

# 方式 B：以宿主当前用户运行（compose 里取消注释 user 行，或命令行覆盖）
docker run --user "$(id -u):$(id -g)" ...
```

Docker Desktop（macOS/Windows）的 bind mount 会做 UID 映射，通常无需处理。

### 4.3 只用 docker run

```bash
docker run -d --name norvels \
  -p 8080:8080 \
  -v "$PWD/data:/data" \
  -v "/path/to/txt:/books:ro" \
  norvels:0.1.0
```

### 4.4 健康检查

```bash
# 容器内（distroless 无 shell，但 healthcheck 是独立可执行文件）
docker exec norvels /healthcheck

# 宿主机
curl -fsS http://127.0.0.1:8080/api/health
# {"status":"ok","db":"ok","version":"0.1.0","uptimeSeconds":12}
```

`/healthcheck` 判定规则：HTTP 200 且 JSON 中 `status == "ok"`，否则退出码 1。
`start_period: 10s` 给 SQLite 迁移留出时间；大库首次迁移更久时按需调大。

## 5. SQLite on volume 注意事项

1. **主库、WAL、SHM 必须同目录**：`norvels.db`、`norvels.db-wal`、`norvels.db-shm` 都在 `/data`。不要把 `-wal` 单独挂载/清理。
2. **单写者**：导入与服务不要并发写库。导入期间请停止 `serve`（compose 场景先 `docker compose stop`，或用 `scripts/import.sh` 的顺序导入）。SQLite 会以 `busy_timeout=10000` 等待，但并发导入/备份仍可能触发 BUSY。**容器场景特别注意：导入期间不要并发执行 `scripts/backup.sh`**（备份会读一致性快照，但 `VACUUM INTO` 需要写锁，且大库备份与导入叠加会放大磁盘 IO）。
3. **备份方式**：使用 `scripts/backup.sh`（`.backup` 在线备份或 `VACUUM INTO`）。直接 `cp norvels.db` 会漏掉 WAL 中未 checkpoint 的数据。
4. **checkpoint**：导入结束会执行 `PRAGMA wal_checkpoint(TRUNCATE)`，把 WAL 合并回主库并缩小体积；备份前也可先 checkpoint。
5. **卷性能**：数据库放本地 SSD 卷。网络盘（NFS/SMB）上的 SQLite 锁语义不可靠，会随机 `database is locked`。
6. **卷生命周期**：`docker compose down` 保留 bind mount 数据；若改用 named volume，`docker compose down -v` 会删除数据，操作前确认。
7. **空间（实测倍率 3.3–3.4x，比按原文 1.76x 的初估更高）**：
   - 本机实测：30 个真实 txt（13.2 MiB）导入后库 **44.3 MB**（3.34x）；8 个（5.09 MiB）导入后 **17.4 MB**（3.42x）。
   - 原因：`chapters` 同时保存正文 `content` 与字级分词镜像列 `content_seg`（逐字加空格），FTS5 再为分词文本建索引（见 `internal/store/schema.sql`）。
   - 按 4.4 GB 原文外推，主库约 **14–15 GB**；`/data` 卷建议预留 **≥ 20 GB**，再保留一份备份建议 **≥ 35 GB**。
   - 若后续把字级分词改为 trigram（调研建议路线，见 [research.md](research.md)）或去掉 `content_seg` 镜像列，倍率会回落，但 trigram 对 2 字查询需要 LIKE 回退。以 `/api/stats` 的 `dbBytes` 为准。
   - **v2 计划**：FTS 改为 contentless、去掉 `content_seg` 镜像列，主库倍率预计回落到约 2x（4.4 GB 原文约 8–9 GB），届时本表与 README 同步更新。

## 6. 资源建议

| 场景 | CPU | 内存 | 磁盘 | 依据 |
|---|---|---|---|---|
| 浏览/阅读（serve） | 1 vCPU | 1 GB | 数据卷大小 | Go 服务 RSS 约 50–100 MB；`cache_size=-65536` 让 SQLite 页缓存上限约 64 MB（见 `internal/store/open.go`） |
| 导入 4.4 GB / 865 文件 | 2 vCPU | 2 GB | 源 4.4 GB + 库 14–15 GB + 备份 | 解析是 CPU 密集（并发 `min(NumCPU, 8)`），SQLite 写入是 IO 密集；WAL 峰值数十 MB。1C1G 也能跑完，但耗时约翻倍 |
| 导入 + FTS 重建 | 2–4 vCPU | 2–4 GB | 同上 | `reindex` 需要额外临时页缓存；大库建议先导入再单独建索引 |

- 磁盘 IO：SSD 明显优于机械盘（估算 2–3 倍差异），4.4 GB 文本顺序读 + 随机写库。
- 网络：仅 API 与静态资源，浏览流量很小；导入不需要网络。
- 若在低配机器（1C512M）上运行，建议把 `cache_size` 调小（需改代码）或只用于浏览。

## 7. 升级与回滚

```bash
# 升级：先备份 → 拉新镜像 → 重建 → 验证
scripts/backup.sh --keep 7
docker build -f deploy/Dockerfile -t norvels:0.2.0 .
docker compose -f deploy/compose.yml up -d --no-build   # 镜像 tag 指向新版本

# 回滚：切回旧 tag（数据目录不动）
docker compose -f deploy/compose.yml down
docker tag norvels:0.1.0 norvels:current
docker compose -f deploy/compose.yml up -d
```

- schema 迁移是**版本化**的（编号迁移 + `schema_migrations` 记录表），升级只前进不回滚；升级前仍建议备份。
- 回滚只需换镜像 tag；如果新版本写入过旧版本不认识的列，回滚前用备份恢复更稳妥。
- 数据库文件格式向后兼容，但**不要**在旧版本上打开已被新版本迁移过的库并写入，先恢复备份。

### 7.1 版本化迁移（v0.2.0 起）

启动时自动把库升级到镜像内的最新版本，失败**拒绝启动**（不带着半迁移的库继续跑）：

```bash
# 只看将要应用哪些迁移（不写库，可对线上库安全执行）
docker compose -f deploy/compose.yml run --rm --entrypoint /norvels   norvels migrate --db /data/norvels.db --dry-run

# 启动日志会打印「库版本 N → 应用 M 个迁移 → 版本 M」
docker compose -f deploy/compose.yml logs --tail 20 norvels
```

- 迁移源文件在镜像内的 `internal/store/migrations/NNNN_*.sql`，按文件名的版本号升序应用；
- 每条迁移在**独立事务**内执行，失败整体回滚且进程退出码非 0；已成功的前序迁移保持已应用；
- **老库兼容**：v0.1.x 的库（`PRAGMA user_version=1`、有业务表、无 `schema_migrations`）
  首次升级会被识别并直接把版本 1 记为已应用，**不重跑建表**，数据条数不变；
- 迁移版本号只增不减。若需要回退某个结构变更，请从备份恢复，不要删 `schema_migrations` 记录。

| 版本 | 迁移 | 内容 |
|---|---|---|
| 1 | `0001_init.sql` | 初始结构（authors / books / volumes / chapters / tags / import_jobs / FTS5） |
| 3 | `0003_reading_progress.sql` | `reading_progress` 表 + 更新时间索引（阅读进度服务端化） |

> 版本 2 预留未用（历史上规划过又合并掉了），迁移版本号不要求连续。

### 7.2 鉴权

设置环境变量 `NORVELS_AUTH_TOKEN` 后启用鉴权（**公网部署必须设置**）：

```bash
NORVELS_AUTH_TOKEN="$(openssl rand -hex 24)" docker compose -f deploy/compose.yml up -d
```

- `/api/*` 与全部页面要求 `Authorization: Bearer <token>` 或登录后下发的 Cookie；
- `/api/health` 免鉴权（容器健康检查依赖它），`/login` 是登录表单页；
- Cookie 为 HttpOnly + SameSite=Lax + Path=/（30 天），比较令牌用恒定时间比较；
- 未设置 token = **开放模式**，启动日志会打印 `WARN: 未启用鉴权，请勿暴露到公网`。

### 7.3 配置优先级

命令行 flag > 环境变量 > 默认值。环境变量统一 `NORVELS_` 前缀：

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `NORVELS_ADDR` | `:8080` | 监听地址 |
| `NORVELS_DB` | `./data/norvels.db`（镜像内 `/data/norvels.db`） | 数据库路径 |
| `NORVELS_AUTH_TOKEN` | 空 | 空 = 开放模式 |
| `NORVELS_LOG_LEVEL` | `info` | debug/info/warn/error（非法值启动即报错） |
| `NORVELS_BOOKS_DIR` | `/books` | 导入源目录（只读挂载点） |

## 8. 排障

| 现象 | 排查 |
|---|---|
| 容器启动即退出 | `docker logs norvels`；确认 `/data` 可写（见 4.2）、`NORVELS_DB` 路径正确 |
| 健康检查一直 unhealthy | `docker exec norvels /healthcheck`；看 `/api/health` 的 `db` 字段；调大 `start_period` |
| 导入报 BUSY | 停掉 `serve`；确认没有第二个 `import`；避免同时 `backup.sh` |
| 镜像比预期大 | `docker history norvels:0.1.0`；确认 `-s -w` 与 `CGO_ENABLED=0` 生效，`web/node_modules` 未被 COPY |
| 中文乱码 | 见 README FAQ；编码探测结果可在 `/api/books/{id}` 的 `encoding` 字段查看 |
| 备份文件很小 | 检查是否只拷了主库而漏 WAL；用 `scripts/backup.sh` 而非 `cp` |

## 9. 相关文档

- 架构与数据模型：[architecture.md](architecture.md)
- REST API 契约：[api.md](api.md)
- 容器方案调研：[research.md](research.md)（由 research-docker 维护）
