# norvels

**把一大堆中文 txt / EPUB 小说变成一个可浏览、可搜索的私人书库**：单二进制 + 两个 SQLite 文件（元数据主库 + 正文库）+ 轻量容器镜像，无外部依赖、不联网。

> 4.4 GB / 865 个文件的中文 txt，一次导入后得到「书架 → 目录 → 阅读 → 找书」的完整使用体验。

## 功能特性

- **多格式导入**：`.txt` / `.epub` / `.zip`（浏览器上传或目录扫描）。EPUB 自动带出书名/作者/简介/分类/系列、封面与目录层级（卷）；zip 包内逐条导入，条目名带 `包名.zip:包内路径`。不支持的格式与包内条目当场提示原因，不静默跳过。
- **编码自适应**：UTF-8 / UTF-8 BOM / GB18030 / UTF-16 / Big5 多候选解码 + CJK 命中率打分，乱码文件也能正常入库。
- **智能切章**：识别「第X章/节/回/卷/部」「数字行」「Chapter N」「序章/楔子/番外」，支持卷/部嵌套、无标记短篇兜底；单文件多书合集按章号重置自动拆成多本。
- **幂等导入**：按文件 SHA-256 去重，重复导入自动跳过；支持 `--force`、`--update`（追更只追加新章）、`--dry-run`、`--workers`、`--min-chapters`。
- **AI 选书（可选）**：`--ai` 让外部模型逐本判「值不值得入库」（规则先筛：章节门槛、判重、质量分；判不准的按不入库处理）。判定口径与降级见 [AI 选书设计](docs/specs/2026-11-01-ai-curation-design.md)。
- **书名/作者检索**：`LIKE` 子串匹配，毫秒级返回；章节标题与正文都不参与找书，不建全文索引（见 [ADR-0002](docs/adr/0002-drop-fulltext-index.md)）。
- **封面与元数据可编辑**：有封面的书在书架与详情页显示真实封面；书名/作者/简介/标签/系列可在详情页直接改。
- **零运行时依赖**：前端构建产物 `go:embed` 进二进制，容器里只有两个静态可执行文件。
- **轻量镜像**：distroless/static + 非 root，约 21.7 MiB（实测二进制 16.29 MiB，已含前端产物 + 健康检查 2.14 MiB + 基镜像解压后 3.27 MiB，详见 [docs/deploy.md](docs/deploy.md)）。
- **数据即文件**：主库 `/data/norvels.db` + 正文库 `/data/norvels-content.db`（各带 WAL），备份/迁移就是复制这一对文件
  （`scripts/backup.sh` 成对处理，含 WAL）。

## 架构

```mermaid
flowchart LR
  A["txt 目录<br/>865 文件 / 4.4 GB"] --> B["importer<br/>扫描 · 并发 · 幂等"]
  B --> C["parser<br/>编码探测 · 清洗 · 切章"]
  C --> D["store<br/>SQLite 双库（主库 + 正文库）"]
  D --> E["api<br/>REST /api/*"]
  F["web（React 19 · Vite · Tailwind）<br/>构建产物 embed"] --> E
  E --> G["浏览器<br/>书架 · 目录 · 阅读器 · 搜索"]
```

部署形态（Docker）：

```mermaid
flowchart LR
  U["浏览器 :8080"] --> C["norvels 容器<br/>distroless · nonroot"]
  C --> V["/data 卷<br/>norvels.db + norvels-content.db"]
  S["小说目录（只读）<br/>挂载到 /books"] -.导入时.-> C
```

## 目录结构

```text
norvels/
├── cmd/
│   ├── norvels/            # 主程序：import / serve / stats / check-content / reconcile-content / migrate / version
│   └── healthcheck/        # 镜像内健康检查（distroless 无 curl，用它请求 /api/health）
├── internal/
│   ├── api/                # REST 路由、中间件、SPA 静态资源
│   ├── ebook/              # EPUB 解包（zip → OPF → spine → 纯文本/元数据/封面）
│   ├── importer/           # 目录扫描、并发解析、幂等入库、上传落盘、进度
│   ├── model/              # 领域类型（ParsedBook / Chapter ...）
│   ├── parser/             # 编码探测、文本清洗、章节切分
│   ├── store/              # SQLite 打开/迁移/查询
│   └── webui/              # go:embed 前端产物
├── web/                    # React 19 + TypeScript + Vite + Tailwind
├── deploy/
│   ├── Dockerfile          # 三阶段：node → golang → distroless
│   └── compose.yml         # 服务编排（端口/卷/健康检查/重启策略）
├── scripts/
│   ├── import.sh           # 批量导入（本地 / docker 两种模式）
│   ├── backup.sh           # SQLite 备份 + 保留策略
│   ├── preview.sh          # 一条命令起预览：后端 :8080 + vite HMR :5173（已在跑则复用）
│   ├── shot.sh             # 一次拍完多个视口 + DOM 量测（agent-browser 独立会话），配 preview 用
│   ├── wait-for.sh         # 等一个条件成立（本地/远端，一次调用阻塞到结果，替代 sleep+轮询）
│   └── tidy-pending-collection.py  # 待整理合集包：去重 → 按分类抓取到导入目录
├── docs/
│   ├── architecture.md     # 组件划分与数据模型
│   ├── api.md              # REST API 契约
│   └── deploy.md           # 构建、体积、SQLite on volume、资源建议
├── data/                   # 运行时生成（已 gitignore）
├── Makefile
├── go.mod
└── README.md
```

## 快速开始

### 本地（3 条命令）

```bash
make preview                                                        # 起预览：后端 :8080 + vite HMR :5173（缺二进制会先编译）
./bin/norvels import --dir "/path/to/txt" --db ./data/norvels.db     # 整目录或单个 txt
./bin/norvels import --dir "/path/to/txt" --ai                        # 同上，入库前先让 AI 判该不该收
make build                                                          # 生产产物 bin/norvels（前端先同步进 go:embed 目录）
```

`--dir` 同时接受目录和单个 `.txt` / `.epub` / `.zip` 文件：给目录就批量扫描（`--recursive` 控制是否进子目录），
给文件就只导这一本。传入其他扩展名的文件会直接报错，不会静默导 0 本。

改前端看 <http://127.0.0.1:5173>（HMR 即时生效，`/api` 由 vite 代理到 :8080）；看生产行为走 <http://127.0.0.1:8080>，接口在 `/api/stats`。
`make preview` 会**复用**已经在 :8080 / :5173 上的服务（不 kill、不占第二个端口）；要后台跑用 `scripts/preview.sh --detach`，收尾 `--stop`（只收它自己起的）。
只改后端用 `make build-server`（复用已有 `internal/webui/dist`，跳过前端构建）；截个图：
`make shot URL=http://127.0.0.1:5173 OUT=.shots/home.png VIEWPORT=390x844,1280x800`（多视口一次拍完，
加 `EVAL='document.querySelectorAll(".page").length'` 顺带把量测打到终端）。等长任务别自己 sleep 轮询：
`scripts/wait-for.sh --host nas --until 'pgrep -f run_import.py' --gone --timeout 3600` 一次调用阻塞到条件成立。

### Docker（3 条命令）

```bash
docker build -f deploy/Dockerfile -t norvels:0.9.0 .          # 1) 构建镜像
NORVELS_BOOKS_DIR="/path/to/txt" docker compose -f deploy/compose.yml \
  run --rm --entrypoint /norvels norvels import --dir /books --db /data/norvels.db   # 2) 先导入
NORVELS_BOOKS_DIR="/path/to/txt" docker compose -f deploy/compose.yml up -d         # 3) 启动
```

- 端口：宿主 `8080` → 容器 `8080`；
- 数据：宿主 `./data/norvels.db` → 容器 `/data/norvels.db`；
- 源目录：`/path/to/txt` 挂载到容器 `/books`（**只读**，不会被修改）。

## 导入 4.4 GB / 865 个文件

以下命令使用本机真实数据目录（路径含中文与空格，**所有路径都要加引号**）。源目录只读，脚本不会修改其中任何文件。

### 本地二进制

```bash
# 1) 构建（Makefile 用 -trimpath -ldflags "-s -w" 并注入版本）
make build-server

# 2) 导入（workers 默认 min(NumCPU, 8)；--min-chapters 0 表示全收）
time ./bin/norvels import \
  --dir "/path/to/txt" \
  --db ./data/norvels.db \
  --workers 4

# 3) 校验
./bin/norvels stats --db ./data/norvels.db
```

### Docker（脚本一键）

```bash
# 源目录只读挂载到 /books，数据库写入宿主 ./data（容器内 /data/norvels.db）
docker build -f deploy/Dockerfile -t norvels:0.9.0 .
scripts/import.sh --mode docker \
  --dir "/path/to/txt" \
  --db ./data/norvels.db --workers 4
```

等价的原始 `docker run`：

```bash
docker run --rm --entrypoint /norvels \
  -v "/path/to/txt:/books:ro" \
  -v "$PWD/data:/data" \
  norvels:0.9.0 \
  import --dir /books --db /data/norvels.db --workers 4
```

### 参考耗时与空间

| 指标 | 参考值 | 说明 |
|---|---|---|
| 文本总量 | 4.1–4.4 GB | `du -sh` 实测 4.1 GB；`find -iname '*.txt'` 计数 865 个 |
| 导入耗时（单机 SSD，并发 4） | 约 10 分钟 | 838 个文件 / 4.1 GB 实测 10 分 25 秒；解析 CPU 密集，1C1G 约翻倍 |
| 导入耗时（单文件） | 数十毫秒–数秒 | 取决于文件大小（单文件最大约 42 MB） |
| 库体积 | **29.0 GB** → 跑完 0017 约 **22 GB**；0018 之后拆成主库（元数据）+ 正文库（约 21 GB） | 20,110 本 / 791 万章 / 79.49 GB 正文；正文按 **8 MB 块 + 内置字码本 zstd** 压缩，26.45%（200 本抽样，[ADR-0007](docs/adr/0007-codebook-compressed-content.md)）；老库的 1 MB 块 / 34.67% 要迁移后重压 |
| 磁盘预留 | ≥ 库体积的 2 倍 | `0017` 迁移期间新旧两份块并存（约 27.5 GB + 21 GB，另加 WAL）；`0018` 搬库期间主库旧块 + 正文库新块同时存在（约 22 GB + 21 GB），迁移后两个库各要 `VACUUM` 才真正回落 |

导入后可重复执行：内容未变的文件按 SHA-256 命中直接跳过；需要重解析时加 `--force`。

## 常用命令

| 命令 | 作用 |
|---|---|
| `make build` | 构建前端并同步到 `internal/webui/dist`，再编译 `bin/norvels` |
| `make build-server` | 只编译后端（要求 `internal/webui/dist` 已有前端产物） |
| `make run` | 本地启动服务（`:8080`，`./data/norvels.db`） |
| `make preview` | 一条命令起预览：后端 `:8080` + vite HMR `:5173`；已在跑则复用（不占第二个端口） |
| `make shot URL=… OUT=…` | 用 agent-browser 截一张图（`VIEWPORT=390x844` 可传），默认落 `.shots/` |
| `make gate-fast` | 快环：gofmt + go vet + 前端 `tsc --noEmit`（秒级；通过 ≠ 门禁通过） |
| `make gate` | 提交前完整门禁（9 项并行执行，见 [quality-gates](docs/quality-gates.md)） |
| `make privacy-changed` | 隐私扫描：只扫相对 HEAD 的改动（提交前仍用 `make privacy` 全量） |
| `make test` | 运行全部 Go 测试 |
| `make fmt` | gofmt |
| `make docker` | 构建容器镜像 |
| `scripts/import.sh --dir <目录>` | 批量导入（本地二进制模式） |
| `scripts/backup.sh --keep 7` | 备份数据库并保留最近 7 份 |
| `make bench-capacity` | 容量压测（500 本 x 500 章合成库，不碰生产库） |
| `./bin/norvels version` | 版本、commit、构建时间 |

## API 一览

完整契约见 [docs/api.md](docs/api.md)。Base 为 `/api`，响应均为 JSON。

每个版本改了什么，见 [发版说明](internal/releases/CHANGELOG.md)——同一个文件用 `go:embed` 打进二进制，
应用内的「更新日志」页（`/changelog`）读的就是它，所以网页与仓库不会是两份。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查（`status`、`db`、`version`、`uptimeSeconds`） |
| GET | `/api/stats` | 库级统计（书籍/章节/作者/字数/库大小） |
| GET | `/api/version` | 版本信息 |
| GET | `/api/releases` | 发版说明（内嵌 CHANGELOG，按版本倒序；网页「更新日志」/changelog 的数据源） |
| GET | `/api/books` | 书籍列表（`q`、`authorId`、`tag`、`series`、`rating`、`collection`、`sort`、分页） |
| GET | `/api/books/{id}` | 书籍详情 + 卷/章目录 |
| GET | `/api/books/random` | 随机一本（可按作者/标签/书单缩范围；无书 404） |
| PATCH | `/api/books/{id}` | 改元信息（含评分与出版信息，支持清空） |
| POST | `/api/books/batch` | 批量改元信息（≤200 本；`updated`/`failed`） |
| DELETE | `/api/books/{id}` | 删书（单事务级联） |
| GET | `/api/books/{id}/chapters` | 轻量章节列表（目录虚拟滚动） |
| GET | `/api/books/{id}/chapters/{cid}` | 章节正文（含 prevId/nextId） |
| POST | `/api/books/{id}/chapters` | 新建章节（`title`+`content`，可 `afterChapterId`） |
| POST | `/api/books/{id}/ack-new-chapters` | 追更角标清零 |
| GET | `/api/books/{id}/cover` | 封面图片（无封面 404） |
| GET | `/api/series` | 系列聚合列表（书数/字数/最近更新） |
| GET | `/api/series/{name}` | 系列详情（URL 编码系列名） |
| GET | `/api/tags` | 标签列表（含 `bookCount`） |
| POST | `/api/tags` | 新建标签（同名返回已有 200） |
| PATCH | `/api/tags/{id}` | 标签改名（重名 409） |
| DELETE | `/api/tags/{id}` | 删标签（全库解绑） |
| GET | `/api/collections` | 书单列表 |
| POST | `/api/collections` | 新建书单（重名 409） |
| PATCH | `/api/collections/{id}` | 书单改名 |
| DELETE | `/api/collections/{id}` | 删书单（不动书） |
| PUT | `/api/collections/{id}/books` | 全量替换书单书目（`[]` 清空） |
| GET | `/api/reading/progress` | 最近在读 |
| PUT | `/api/reading/progress` | 保存阅读进度（含章内 `paraIndex`） |
| POST | `/api/reading/sessions` | 上报阅读时长（1–86400 秒） |
| GET | `/api/reading/history` | 阅读历史（按书聚合、分页） |
| GET | `/api/stats/reading` | 阅读统计与热力图（`days`/`tz`） |
| GET | `/api/authors` | 作者列表（`q`、分页） |
| GET | `/api/authors/{id}` | 作者详情 + 作品 |
| POST | `/api/auth/register` | 注册账号（注册即登录，下发 `norvels_session`） |
| POST | `/api/auth/login` | 按账号名登录（无密码；未注册返回 404） |
| POST | `/api/auth/logout` | 撤销当前会话（幂等 204） |
| GET | `/api/auth/me` | 当前账号（开放模式返回默认账号 `reader`） |
| GET | `/api/shelf` | 我的书架（分页，按加入时间倒序） |
| GET | `/api/shelf/ids` | 我的书架全部 book_id（不分页，供列表打标） |
| PUT | `/api/shelf/{bookId}` | 加入我的书架（幂等 204） |
| DELETE | `/api/shelf/{bookId}` | 移出我的书架（幂等 204） |
| GET | `/api/search` | 书名 / 作者检索（`q`、`type=book|all`） |
| POST | `/api/import` | 触发导入（202 + jobId；并发导入返回 409） |
| POST | `/api/import/upload` | 浏览器上传 txt/epub/zip（multipart，逐文件 accepted/rejected） |
| GET | `/api/import/jobs` | 导入任务列表 |
| GET | `/api/import/jobs/{id}` | 任务详情 + 失败明细 |

## 容器资源建议

| 场景 | CPU | 内存 | 磁盘 | 依据 |
|---|---|---|---|---|
| 日常浏览 | 1 vCPU | 1 GB | 数据卷大小 | Go 服务 RSS 约 50–100 MB；SQLite 页缓存上限约 64 MB（`cache_size=-65536`） |
| 导入 4.4 GB | 2 vCPU | 2 GB | ≥ 5 GB 空闲 | 解析 CPU 密集（并发上限 8）、SQLite 写入 IO 密集、库最终约 1.6 GB、WAL 峰值数十 MB |
| 容量压测 | 1 vCPU | 1 GB | 临时目录 ≥ 2 GB | `make bench-capacity` 默认小规模，库建在系统临时目录、跑完自动删 |

- 磁盘用本地 SSD；网络盘（NFS/SMB）上的 SQLite 锁语义不可靠。
- 导入期间不要同时备份（见下）；导入结束会 `wal_checkpoint(TRUNCATE)` 收敛体积。
- 详细说明与估算来源：[docs/deploy.md](docs/deploy.md#6-资源建议)。

## 常见问题

### 中文乱码怎么办？

服务端按 UTF-8 输出 JSON，终端 locale 不影响数据。入库时 parser 会做多候选解码：

1. `./bin/norvels import --dry-run --dir "<目录>"` 先试跑，观察解析告警；
2. `GET /api/books/{id}` 的 `encoding` 字段会显示探测结果（`utf-8`、`gb18030`、`big5`、`utf-16le` 等）；
3. 若个别文件仍乱码，多为「GBK 主文 + 少量 UTF-8 广告行」或文件损坏，可先用系统工具确认原始编码，再反馈样本。

### 章节没有切开？

章节切分是启发式规则，已覆盖：`第X章/节/回/卷/部`、`Chapter N`、纯数字行、`序章/楔子/番外/尾声`，以及无标记短篇的整篇兜底。排查顺序：

1. 用 `--dry-run` 看该文件的 `Warnings`；
2. 确认标题行是否被广告清洗规则误删（parser 会去掉含网址/水印的行）；
3. 若文件是「多书合集」：章号重置点会被自动识别并拆成多本（每段至少 3 章，书名取段内书名标记）；拆不开时按单本导入并在 `Warnings` 里说明。

### 镜像为什么这么小？

三阶段构建 + `CGO_ENABLED=0`（纯 Go SQLite）+ `-trimpath -ldflags "-s -w"` + distroless/static（无 shell、apt、curl）。
体积拆解见 [docs/deploy.md](docs/deploy.md#3-镜像体积拆解)：二进制 **16.29 MiB**（linux/amd64 实测，含 x/text 与 embed 的前端产物 —— 不含前端时为 15.38 MiB）、healthcheck **2.14 MiB**、distroless 基镜像解压后 **3.27 MiB**（压缩传输 0.82 MiB），合计约 **21.7 MiB**。

### 备份时要连 WAL 一起备吗？

**不要直接 `cp norvels.db`**：WAL 模式下未 checkpoint 的数据在 `norvels.db-wal` 里，单拷主库会丢数据。正确做法：

```bash
scripts/backup.sh --keep 7          # 内部用 sqlite3 .backup 或 VACUUM INTO，自动包含 WAL
```

- `.backup` 是 SQLite 在线备份 API，服务运行中也能拿到一致快照；
- `VACUUM INTO` 会拿写锁，导入期间避免执行；
- **容器场景：导入期间不要并发备份**（compose 先 `docker compose stop`，导入完成、`wal_checkpoint(TRUNCATE)` 结束后再备份）；
- 恢复：停服务 → 用备份文件替换 `data/norvels.db` → 删除遗留的 `-wal`/`-shm` → 启动。

## 设计取舍与调研结论

几条经过实测的取舍：

- **最接近的参考实现是 zreader**：Go 单二进制 + `modernc.org/sqlite` + React SPA `go:embed`，作者自述约 23 MB 镜像，证明「Go + SQLite + 中文 txt」能在 20 MB 量级落地。
- **正文检索已主动放弃**：早期用「字级分词镜像列 + FTS5」支持 2 字及以上正文检索，但实测它占掉全库 65% 的体积（838 本 14.84 GB 里 9.58 GB 是索引）。用户不需要按正文内容检索，因此移除正文索引，检索收敛为书名 / 作者的 `LIKE`（5 万本实测 18.4 ms）。决策与后果见 [ADR-0002](docs/adr/0002-drop-fulltext-index.md)。
- **体积不是瓶颈，磁盘与导入耗时才是**：镜像约 21.7 MiB，远低于 50 MB；4.4 GB 语料的 `/data` 需要 ≥ 20 GB。
- 调研与本文都确认：本机没有容器运行时，镜像体积需在装好 Docker 的机器上用 `docker images` 对账（见 [docs/deploy.md](docs/deploy.md#23-已知限制重要)）。

## 隐私与仓库卫生

本仓库只包含**工具本身**，不含任何书库内容：原始 txt 与数据库文件都不入库，文档与原型示例也已脱敏
（本机路径统一写成 `/path/to/txt`，书名/作者用虚构名）。提交前跑一次扫描：

```bash
make privacy      # scripts/privacy-scan.sh --all：本机路径 / 内网 IP / 令牌 / 本地黑名单书名
make verify       # 完整门禁 9 项（scripts/gate-all.sh，编号与阈值见 docs/quality-gates.md）
```

细则（哪些进仓库、脱敏规则、历史清理）见 [docs/privacy.md](docs/privacy.md)。

## 文档

- [架构与数据模型](docs/architecture.md)
- [语料画像与容量推算](docs/data-profile.md)
- [ADR-002：移除正文全文索引](docs/adr/0002-drop-fulltext-index.md)
- [ADR-007：正文压缩（8 MB 块 + 内置字码本）](docs/adr/0007-codebook-compressed-content.md)
- [REST API 契约](docs/api.md)
- [部署指南（构建参数、体积、SQLite on volume、资源建议、升级回滚）](docs/deploy.md)
- [运行时性能观测（分位 / 慢请求 / SQL 模板 / 开关）](docs/observability.md)
- [隐私与仓库卫生](docs/privacy.md)

## 许可证

MIT
