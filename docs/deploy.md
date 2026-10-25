# 部署指南

本文覆盖 norvels 的容器构建、体积构成、运行、SQLite 数据卷注意事项、资源建议与升级回滚。
接口与架构分别见 [api.md](api.md) 与 [architecture.md](architecture.md)。

## 1. 交付物一览

| 文件 | 作用 |
|---|---|
| `deploy/Dockerfile` | 三阶段构建：node 构建前端 → golang 静态编译 → distroless/static 运行 |
| `deploy/compose.yml` | 使用者版编排：直接拉 `bsfeng/norvels` 镜像，端口 8080、`../data` 数据卷、小说目录只读挂载、健康检查、自动重启 |
| `deploy/compose.dev.yml` | 本地开发覆盖文件：保留 `build:` 段从源码构建（两个 `-f` 叠加使用） |
| `scripts/import.sh` | 批量导入（本地二进制 / docker 两种模式），带校验、耗时统计、导入后 stats |
| `scripts/backup.sh` | SQLite 在线备份（`.backup` 或 `VACUUM INTO`）+ 时间戳命名 + 保留策略 |
| `cmd/healthcheck` | 2.1 MiB 静态健康检查器，供 distroless 镜像的 HEALTHCHECK 使用 |
| `.dockerignore` | 排除 node_modules、dist、data、testdata 等，构建上下文更小 |

## 2. 构建

> **使用者不需要看这一节**：`docker compose -f deploy/compose.yml up -d` 会直接从 Docker Hub 拉多架构镜像
> （见 §4.1）。本节面向**要从源码自己构建**的人（贡献者、离线环境、要改代码的部署）。

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
| `GOPROXY` | `https://proxy.golang.org,direct` | 默认走官方代理（CI 在 GitHub runner 上）。国内本地构建可覆盖为 `--build-arg GOPROXY=https://goproxy.cn,direct` |

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
3. **runtime（gcr.io/distroless/static-debian13:nonroot@sha256:…）**：只拷二进制、healthcheck 与 `/data` 目录，非 root（65532）运行，无 shell。

**交叉编译**：web 与 build 两个 stage 都声明 `--platform=$BUILDPLATFORM`，只在构建机自身架构上执行；
目标架构由 `ARG TARGETOS/TARGETARCH` 传给 `go build`（`CGO_ENABLED=0` 纯 Go，天然可交叉编译）。
不声明时 buildx 会把整个 stage 放在目标平台上跑：arm64 构建时 node/pnpm 与 go build 全在 QEMU
模拟下执行，耗时慢数倍。runtime stage 刻意不声明——它本来就该按目标架构拉基础镜像。

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

#### 基础镜像 digest 锁定

运行镜像写成 `gcr.io/distroless/static-debian13:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88`，
标签 + digest 双写。为什么锁：

- **tag 是可变的**：`nonroot` 是浮动标签，上游重建就会指向新镜像，同一份源码在不同时间可能构建出不同底座，"上周能跑这周不能跑"无法归因；
- **digest 不可变**：`@sha256:…` 指向的唯一内容，镜像层被替换时构建会直接失败而不是静默换底座；
- **升级可审计**：换 digest 是一次显式 diff（`.github/dependabot.yml` 的 `docker` 生态会以周更 PR 提出），评审看一眼 digest 行即可，不用反推"基镜像到底变没变"。

该 digest 是 manifest index（多架构索引），同一行同时覆盖 linux/amd64 与 linux/arm64，
因此多架构构建不需要按平台分别钉。取法（无需 docker）：

```bash
BASE=https://gcr.m.daocloud.io   # gcr.io 的只读代理；直连 gcr.io 时换成 https://gcr.io
TOKEN=$(curl -s "https://m.daocloud.io/auth/token?service=gcr.m.daocloud.io&scope=repository:distroless/static-debian13:pull" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -s "$BASE/v2/distroless/static-debian13/manifests/nonroot" \
  -H "Accept: application/vnd.oci.image.index.v1+json" \
  -H "Authorization: Bearer $TOKEN" -D- -o /dev/null | grep -i docker-content-digest
# docker-content-digest: sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88
```

> 为什么是 debian13：上游 README 只维护 `*-debian13`，debian12 系列已标注 deprecated 且不再更新；
> Debian 12 的安全支持窗口到 2026-07-11 结束。`static` 变体无 shell、无包管理器，体积约 2 MiB。

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
| distroless/static-debian13:nonroot 基础层 | **约 2.0–2.5 MiB** | 官方基镜像（无 shell、无包管理器，仅 ca-certificates、/etc/passwd、tzdata） |
| **合计（估算）** | **约 15.5–16.5 MiB**（前端合入后约 16.5–18 MiB） | 目标 < 50 MiB，余量充足 |

> 前端产物通过 `go:embed` 打进二进制，不额外占镜像层；上表合计 = 二进制 + healthcheck + 基础层。
> 实测命令：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o norvels ./cmd/norvels`（11,681,952 字节），healthcheck 2,248,864 字节。

体积控制手段：

- `CGO_ENABLED=0` + `modernc.org/sqlite`（纯 Go），静态链接，无需 libc；
- `-trimpath` 去掉构建路径，`-ldflags "-s -w"` 去掉符号表与 DWARF；
- distroless/static 无 shell、apt、curl，比 alpine 更小且攻击面更低；
- `.dockerignore` 排除 `node_modules`、`web/dist`、`internal/webui/dist`、`data/`、`testdata/`。

## 4. 运行

### 4.1 compose（推荐，直接拉镜像）

`deploy/compose.yml` 面向使用者，**只用 `image:` 拉 Docker Hub 上的多架构镜像**，本机不需要源码与构建工具链。

```bash
# 1) 先导入（服务尚未启动，避免单写者冲突）
NORVELS_BOOKS_DIR="/path/to/txt" \
  docker compose -f deploy/compose.yml run --rm --entrypoint /norvels \
  norvels import --dir /books --db /data/norvels.db

# 2) 启动
NORVELS_BOOKS_DIR="/path/to/txt" docker compose -f deploy/compose.yml up -d
```

版本用 `NORVELS_VERSION` 控制（默认 `latest`），**生产建议钉具体版本**：

```bash
NORVELS_VERSION=0.2.0 docker compose -f deploy/compose.yml up -d
```

本地开发从源码构建，叠加 dev 覆盖文件（它保留 `build:` 段，并把镜像打上 `-dev` 后缀）：

```bash
docker compose -f deploy/compose.yml -f deploy/compose.dev.yml up -d --build
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
  -e NORVELS_AUTH_TOKEN="$(openssl rand -hex 24)" \
  bsfeng/norvels:latest          # 生产建议钉具体版本，如 bsfeng/norvels:0.2.0
```

自己构建的镜像把最后一行的镜像名换成 `norvels:0.1.0`（见 §2）。

### 4.4 健康检查

```bash
# 容器内（distroless 无 shell，但 healthcheck 是独立可执行文件）
docker exec norvels /healthcheck

# 宿主机
curl -fsS http://127.0.0.1:8080/api/health
# {"status":"ok","db":"ok","version":"0.1.0","uptimeSeconds":12}
```

`/healthcheck` 判定规则：HTTP 200 且 JSON 中 `status == "ok"`，否则退出码 1。
`start_period: 60s` 给 SQLite 迁移留出时间；
`start_period` 内失败**不计入** `retries`，否则会出现「迁移中 → 判定 unhealthy → 被重启 → 迁移重来」的死循环。

> ⚠️ **升级到 v0.2（迁移 0006 + 0007）时请把 `start_period` 调到 30 分钟以上**。
> 0007 要把全库正文逐章压进压缩块并重建 `chapters` 表。
> **实测：838 本 / 422,715 章 / 13.8 GB 的库，完整迁移耗时 23 分 20 秒**（单机 SSD）；
> 期间进程没有监听端口，健康检查必然失败。容器被中途重启会让迁移从头再来（事务回滚后重跑），
> 看起来像「卡住了」。**更稳的做法是先用一次性容器跑完迁移**：
>
> ```bash
> docker compose -f deploy/compose.yml run --rm --entrypoint /norvels norvels \
>   migrate --db /data/norvels.db
> # 迁移完再 up -d，此时启动日志会是「应用 0 个迁移」
> ```
>
> 迁移完成后建议跑一次正文自查（见 §7.2），确认正文仍可读。
> **该迁移需要额外磁盘空间**：过程中旧正文与新压缩块并存，峰值约为「原库大小 + WAL 增长」。
> 实测 13.83 GB 的库迁移期间 WAL 涨到 **10.8 GB**，峰值约 **25 GB**；
> 再加上随后 VACUUM 需要的临时空间，**建议预留 35 GB 以上**。
>
> **迁移完成后推荐执行一次 VACUUM 回收空间**（见 §7.3）。

## 5. SQLite on volume 注意事项

1. **主库、WAL、SHM 必须同目录**：`norvels.db`、`norvels.db-wal`、`norvels.db-shm` 都在 `/data`。不要把 `-wal` 单独挂载/清理。
2. **单写者**：导入与服务不要并发写库。导入期间请停止 `serve`（compose 场景先 `docker compose stop`，或用 `scripts/import.sh` 的顺序导入）。SQLite 会以 `busy_timeout=10000` 等待，但并发导入/备份仍可能触发 BUSY。**容器场景特别注意：导入期间不要并发执行 `scripts/backup.sh`**（备份会读一致性快照，但 `VACUUM INTO` 需要写锁，且大库备份与导入叠加会放大磁盘 IO）。
3. **备份方式**：使用 `scripts/backup.sh`（`.backup` 在线备份或 `VACUUM INTO`）。直接 `cp norvels.db` 会漏掉 WAL 中未 checkpoint 的数据。
4. **checkpoint**：导入结束会执行 `PRAGMA wal_checkpoint(TRUNCATE)`，把 WAL 合并回主库并缩小体积；备份前也可先 checkpoint。
5. **卷性能**：数据库放本地 SSD 卷。网络盘（NFS/SMB）上的 SQLite 锁语义不可靠，会随机 `database is locked`。
6. **卷生命周期**：`docker compose down` 保留 bind mount 数据；若改用 named volume，`docker compose down -v` 会删除数据，操作前确认。
7. **空间**：
   - 旧形态（含字级分词镜像列 + FTS5）实测 3.3–3.4x：838 本 / 4.87 GB 正文 → **14.84 GB**，其中 65% 是正文索引。
   - 正文全文索引已移除（[ADR-0002](adr/0002-drop-fulltext-index.md)）：去掉 `content_seg` 镜像列与 `chapters_fts` 后每本约 6.3 MB，838 本约 **5.3 GB**（约 1.2x 原文）。
   - **正文按块压缩后实测 1.62 GB**（13.83 GB 库迁移 + VACUUM 后）：正文压成 5,130 块共 1.69 GB，
     原始 4.87 GB，压缩率 **34.7%**。每本约 1.93 MB（0.36x 原文）。
     算法与取舍见 [ADR-0003](adr/0003-block-compressed-content.md)。
   - 按 4.4 GB 原文规划：跑完迁移 + VACUUM 后 `/data` 约 1.6–2 GB，预留 **≥ 5 GB** 即可；
     **迁移当次**峰值要 35 GB 以上（见 §4 的说明）。以 `/api/stats` 的 `dbBytes` 为准。
   - 几万本的体积与延迟推算见 [data-profile.md §10](data-profile.md#10-容量推算几万本)。

## 6. 资源建议

| 场景 | CPU | 内存 | 磁盘 | 依据 |
|---|---|---|---|---|
| 浏览/阅读（serve） | 1 vCPU | 1 GB | 数据卷大小 | Go 服务 RSS 约 50–100 MB；`cache_size=-65536` 让 SQLite 页缓存上限约 64 MB（见 `internal/store/open.go`） |
| 导入 4.4 GB / 865 文件 | 2 vCPU | 2 GB | 源 4.4 GB + 库约 5–6 GB（移除正文索引后）+ 备份 | 解析是 CPU 密集（并发 `min(NumCPU, 8)`），SQLite 写入是 IO 密集；WAL 峰值数十 MB。1C1G 也能跑完，但耗时约翻倍 |
| 导入 + FTS 重建 | 2–4 vCPU | 2–4 GB | 同上 | `reindex` 需要额外临时页缓存；大库建议先导入再单独建索引 |

- 磁盘 IO：SSD 明显优于机械盘（估算 2–3 倍差异），4.4 GB 文本顺序读 + 随机写库。
- 网络：仅 API 与静态资源，浏览流量很小；导入不需要网络。
- 若在低配机器（1C512M）上运行，建议把 `cache_size` 调小（需改代码）或只用于浏览。

## 7. 升级与回滚

### 7.1 铁律：迁移只前进，回滚必须连数据库一起恢复

> **schema 迁移只前进，不支持降级。回滚镜像版本时，必须同时把数据库从备份恢复**——
> 先停容器、换回旧镜像 tag、用升级前那份备份覆盖 `/data/norvels.db`，再启动。
> 只回滚 tag 不动数据 = 让旧二进制去写由新版本迁移过的库。

**为什么不能只回滚 tag**：迁移是「单向」的结构变更（新列、新表、新索引）。旧二进制不认识这些结构，
它不会报错——它只是按自己那套更早的 schema 继续读写，并把 `PRAGMA user_version` /
`schema_migrations` 里本该更高的版本号覆盖回自己认识的数字。结果是**静默的数据损坏**：
新列被当成不存在、后续升级又被告知"已是最新版本"而跳过迁移，坏掉的结构再也不会被修复。

回滚三步（**顺序不能改**）：

```bash
# 0) 升级前先备份（升级动作的第一步，不是可选步骤）
scripts/backup.sh --keep 7            # 产物：backups/norvels-YYYYmmdd-HHMMSS.db

# 1) 停容器：不要在服务运行时覆盖数据库文件（WAL/SHM 会打架）
docker compose -f deploy/compose.yml down

# 2) 用升级前的备份覆盖数据库，并把库里残留的 WAL/SHM 一起清掉
docker run --rm -v "$PWD/data:/data" -v "$PWD/backups:/backup" alpine:3 \
  sh -c 'rm -f /data/norvels.db-wal /data/norvels.db-shm && \
         cp /backup/norvels-YYYYmmdd-HHMMSS.db /data/norvels.db && \
         chown 65532:65532 /data/norvels.db'

# 3) 换回旧版本镜像再启动（钉版本，不要用 latest）
NORVELS_VERSION=0.1.0 docker compose -f deploy/compose.yml up -d
```

- **WAL/SHM 必须一起删**：只覆盖主库而留下新版本的 `-wal`，旧版本启动时会把 WAL 回放进主库，
  等于把新结构又写回来一次。
- **回滚后核对**：`curl -fsS http://127.0.0.1:8080/api/health` 返回 `status":"ok"`，
  且 `/api/stats` 的书目/章节数与备份时一致；日志里应打印库版本号等于旧镜像支持的版本。
- **备份策略**：`scripts/backup.sh --keep 7` 保留最近 7 份；升级前那一次备份建议另存到
  数据卷之外（换机器也能恢复），并且在大版本升级前确认备份文件能打开（`sqlite3 <备份> 'PRAGMA integrity_check;'`）。
- **只回滚代码、不改 schema 的场景**（同一个 schema 版本内的补丁版本）可以直接换 tag，
  但前提是确认新版本没有应用任何迁移——启动日志会打印「库版本 N → 应用 M 个迁移」，
  日志里 `应用 0 个迁移` 才是可安全回滚 tag 的情形。

### 7.2 升级后自查：`check-content`

v0.2 起正文不再是 SQL 可直接查询的 `content` 列，而是压缩块（`chapter_blocks`）。
迁移之后**先跑一次正文自查**再放心用：

```bash
# 抽查 50 章（默认）——正常时最后一行是 OK
./bin/norvels check-content --db ./data/norvels.db

# 指定抽样数
./bin/norvels check-content --db ./data/norvels.db --sample 200

# 全量校验（42 万章，几分钟）——迁移后第一次升级建议跑这个
./bin/norvels check-content --db ./data/norvels.db --all
```

`--all` 与 `--sample` **互斥**，同时给会直接报错（避免「以为跑了全量、其实只抽样」）。

输出形如：

```text
全量校验 2531/2531 章，触及 72 个压缩块
正文总字节 67530158，单章最小 6939、最大 60227（平均 26681）
OK
```

判定口径（**只认存储完整性，不认内容质量**）：

- `OK` = 校验到的每一章都成功解压 + 章节区间落在块内不越界 + 能按区间切出字节。
  这三条成立就意味着「读回来的字节 == 当初写进去的字节」，存储是完好的；
- **正文不是合法 UTF-8 不算失败**，只打一行提示、退出码仍是 0。这类脏数据几乎总是
  源 txt 的编码问题（某个文件主体 GBK、夹了一段 UTF-8，解析器按整文件只能挑一个），
  迁移是逐字节搬运的，会把它原样保留——报成「迁移损坏」既冤枉又误导排障方向：

  ```text
  提示：20 章正文含非法 UTF-8（源文件编码问题，非迁移损坏，存储完整性不受影响）
    涉及章节 id（最多列 20 个）：[124657 124658 124667 ...]
    这些章节阅读时可能显示乱码；如需定位源文件，可用 --sample N 缩小范围后按 id 反查。
  ```

- 有坏块（解压失败 / 区间越界 / 块缺失）时才会 FAIL，且**列出全部损坏块**
  （不是遇到第一个就停），每行带 `book_id` / `block_seq` / `block_id` / 章节范围，
  并以非零码退出——**此时应从升级前的备份恢复**：

  ```text
  FAIL 发现 2 个损坏的压缩块：
    - 压缩块损坏：book_id=7 block_seq=3 block_id=21 章节 idx 70-104（触发章节 id=312）：... zstd: invalid input
    - 压缩块损坏：book_id=9 block_seq=1 block_id=28 章节 idx 0-34（触发章节 id=401）：... zstd: invalid input
  ```

- 「触及块数」明显小于校验数（例如 5000 章只触及 1 块）说明块切得过大，翻页会变慢，值得排查。

**块损坏是局部故障**：受影响的只是那一块覆盖的章节（约 1 MB / 约 35 章）。
接口层对这类请求返回 `503` + `{"error":{"code":"content_corrupted"}}`，
书架、目录、其他章节都不受影响——前端应只在当前章提示「本章读取失败」，不要整本禁用。

#### 升级与回滚完整示例

```bash
# 升级：备份 → 钉新版本 → 重建 → 验证
scripts/backup.sh --keep 7
NORVELS_VERSION=0.2.0 docker compose -f deploy/compose.yml up -d
docker compose -f deploy/compose.yml logs --tail 20 norvels   # 确认迁移日志与版本号
curl -fsS http://127.0.0.1:8080/api/health

# 回滚：见上面 7.1 的三步（停容器 → 恢复备份 → 换旧 tag 启动）
```

> **待补的启动期守卫**：理想的形态是二进制启动时读库版本，发现「库版本 > 本二进制支持的最高版本」
> 直接**拒绝启动**并提示「请升级镜像，或从备份恢复」，把静默损坏变成显式失败。
> 该守卫需要改 `internal/store/` 的启动路径，当前尚未实现——在那之前，**7.1 的流程是唯一防线**，
> 不要省略备份步骤。

**守卫落地时要补的回归用例**（可直接照着写成 `internal/store` 的单测，本文件只登记断言）：

| 用例 | 构造 | 期望 |
|---|---|---|
| 库版本 > 二进制版本 | 临时库执行 `INSERT INTO schema_migrations(version) VALUES(99)` | 打开返回错误；错误文案含「数据库由更新版本（v99）创建」与「当前版本（vN）」及「请升级镜像，或从备份恢复」；进程退出码非 0 |
| 库版本 = 二进制版本 | 正常迁移到最新 | 正常启动，不打印迁移日志 |
| 库版本 < 二进制版本 | 老库（`user_version=1`、无 `schema_migrations`） | 正常迁移到最新，且**不重跑建表**、数据条数不变 |
| 拒绝后库不变 | 用例 1 打开失败后重新读库 | `schema_migrations` 最大版本仍为 99，`user_version` 未被改写（守卫必须**只读**，失败路径不写库） |
| 空/损坏库 | 零字节文件或非 SQLite 文件 | 报错清晰（区分"不是数据库"与"版本过新"），不误判为老库 |

### 7.3 迁移后回收空间：`VACUUM`

迁移完成后推荐执行一次 `VACUUM` 回收空间：

```bash
norvels migrate --db /data/norvels.db && sqlite3 /data/norvels.db 'VACUUM;'
```

**为什么需要这一步**：0007 把正文从 `chapters.content` 搬到了 `chapter_blocks`，
但原来存正文的页并不会立刻归还给文件系统——它们变成空闲页留在库里。
所以迁移刚跑完时**文件大小几乎没变**，看起来像「迁移没生效」。

实测（13.83 GB / 838 本 / 422,715 章）：

| 阶段 | 库文件大小 | 说明 |
|---|---|---|
| 迁移前 | 13.83 GB | 含分词镜像与 FTS |
| 迁移完成 | 仍是 13.83 GB | `PRAGMA freelist_count` 实测 **2,860,134 页空闲**（约 10.9 GB） |
| 执行 `VACUUM` 后 | **1.62 GB** | 耗时 **24.7 秒** |

`VACUUM` 会重建整个库文件，需要额外约一份库大小的临时空间（SQLite 写临时文件再替换）。
13.83 GB 的库执行前建议确认有 15 GB 以上余量；小库可以忽略。

**顺序建议**：迁移 → `VACUUM` → 跑 `check-content` 自查（§7.2）。
`VACUUM` 只搬页不改数据，放在自查前后都可以；但放在自查之前能让自查跑得更快（文件更小）。

> 容器部署：`VACUUM` 需要写锁且会重建文件，请在**服务停止**的窗口执行。
> 镜像基于 distroless，里面没有 `sqlite3` 命令，所以用宿主机的 sqlite3 直接操作卷里的文件：
>
> ```bash
> docker compose -f deploy/compose.yml stop
> sqlite3 ./data/norvels.db 'VACUUM;'      # 宿主机的 sqlite3，操作 bind mount 的同一个文件
> docker compose -f deploy/compose.yml up -d
> ```
>
> 若用的是 named volume，先 `docker compose cp` 出来、VACUUM 完再拷回去，或临时起一个带
> sqlite3 的容器挂同一个卷。

### 7.4 版本化迁移（v0.2.0 起）

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
- **只回滚镜像 tag 而不恢复备份 = 静默数据损坏**，完整流程见 [§7.1](#71-铁律迁移只前进回滚必须连数据库一起恢复)。

| 版本 | 迁移 | 内容 |
|---|---|---|
| 1 | `0001_init.sql` | 初始结构（authors / books / volumes / chapters / tags / import_jobs / FTS5） |
| 3 | `0003_reading_progress.sql` | `reading_progress` 表 + 更新时间索引（阅读进度服务端化） |

> 版本 2 预留未用（历史上规划过又合并掉了），迁移版本号不要求连续。

### 7.5 鉴权

设置环境变量 `NORVELS_AUTH_TOKEN` 后启用鉴权（**公网部署必须设置**）：

```bash
NORVELS_AUTH_TOKEN="$(openssl rand -hex 24)" docker compose -f deploy/compose.yml up -d
```

- `/api/*` 与全部页面要求 `Authorization: Bearer <token>` 或登录后下发的 Cookie；
- `/api/health` 免鉴权（容器健康检查依赖它），`/login` 是登录表单页；
- Cookie 为 HttpOnly + SameSite=Lax + Path=/（30 天），比较令牌用恒定时间比较；
- 未设置 token = **开放模式**，启动日志会打印 `WARN: 未启用鉴权，请勿暴露到公网`。

### 7.6 配置优先级

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
| 回滚后数据异常 / 新列丢失 | 只换了镜像 tag 没恢复备份。按 §7.1 停容器 → 删 `-wal`/`-shm` → 从备份恢复 → 再用旧 tag 启动 |
| 启动后反复重启、日志停在迁移 | 首次迁移超过 healthcheck 的 `start_period`。调大 `start_period`（当前 60s）或先单独跑 `migrate` 再 `up` |

## 9. 相关文档

- 架构与数据模型：[architecture.md](architecture.md)
- REST API 契约：[api.md](api.md)
- 容器方案调研：[research.md](research.md)（由 research-docker 维护）
