# norvels 架构：组件划分

> 目标：把 4.4GB / 865 个中文 txt 一次性结构化入库，提供「书架 → 目录 → 阅读 → 找书」的完整体验；
> 同时保持 **单二进制 + 单 SQLite 文件 + 轻量容器镜像（目标 < 50MB）**。

## 1. 组件全景

```
                 ┌────────────────────────── norvels (单个 Go 二进制) ──────────────────────────┐
  txt 目录 ──►   │  importer  ──►  parser  ──►  store(SQLite)    api(HTTP)  ◄── webui(embed 前端) │
   (865 files)   │  扫描/编排       编码+元数据+切章     写入/查询/迁移      REST/JSON     静态资源     │
                 └───────────────────────────────────────────────────────────────────────────────┘
                                                        │
                                                   data/norvels.db  (单文件 SQLite)
```

| 组件 | 路径 | 职责 | 对外契约 |
|---|---|---|---|
| **parser** | `internal/parser` | 纯函数：字节流 → `*parser.Book`。编码探测（UTF-8/BOM/GB18030/UTF-16/Big5/Shift-JIS/Big5-HKSCS）、清洗广告行、书名/作者抽取（文件名 + 正文元信息块）、章节切分（第X章/节/回/卷/部、数字行、Chapter N、序章/楔子/番外；无标记短篇兜底）、合集拆分、字数统计 | `parser.ParseFile(path) (*Book, error)`、`parser.ParseBytes(name, b) (*Book, error)` |
| **store** | `internal/store` | SQLite 打开/PRAGMA/**版本化迁移**、所有写读查询、事务批次 | `store.Open(dsn)`、`Migrate()` / `MigrateTo(target)`、`*store.Store` 上的方法（见 `internal/store/types.go`） |
| **importer** | `internal/importer` | 目录扫描（递归）、并发 worker、去重（文件 SHA-256 + 书名作者指纹）、导入任务记录、失败清单、断点续跑、进度回调 | `importer.Run(ctx, store, importer.Options)` |
| **api** | `internal/api` | REST API、中间件（日志/gzip/CORS/panic 恢复）、分页、搜索、静态前端 `http.Handler` | `api.New(store, embedFS, opts) http.Handler` |
| **webui** | `internal/webui` | `go:embed` 前端构建产物（`web/dist` → `internal/webui/dist`） | `webui.FS() fs.FS` |
| **web** | `web/` | React 19 + TypeScript + Vite + Tailwind 前端：书架 / 书籍详情目录 / 阅读器 / 搜索 / 作者 / 导入管理 | 调 `/api/*`（见 `docs/api/README.md`） |
| **cmd/norvels** | `cmd/norvels` | 子命令分发：`import` / `serve` / `stats` / `check-content` / `migrate` / `version` | — |
| **config** | `internal/config` | 运行配置收敛：flag > env > 默认，统一 `NORVELS_` 前缀 | `config.FromEnv()` / `config.Config` |
| **deploy** | `deploy/` | 多阶段 Dockerfile（node 构建前端 → Go 静态编译 → distroless/static）、compose、健康检查 | — |

## 2. 关键约束

1. **CGO-free**：使用 `modernc.org/sqlite`（纯 Go），才能 `CGO_ENABLED=0` 静态编译进 `scratch`/distroless。
2. **数据与代码分离**：数据库放 `/data/norvels.db`（容器 volume），镜像内不含任何小说内容。
3. **前端只经 API**：不引入 SSR，构建产物全部 embed，运行时无需 Node。
4. **导入可重复执行**：同一目录重复 `import` 必须幂等（文件 SHA-256 命中则跳过，除非 `--force`）。
5. **大文件友好**：单文件最大 42MB、单库 4.4GB 文本；导入使用事务分批（每 N 章或每文件提交），WAL 模式，`synchronous=OFF`；聚合口径在导入时同事务预计算进 `store_metrics`，不回填全表。

## 2.1 单用户前提与鉴权边界

norvels 是**单用户**书库：没有账号体系，不做多租户隔离，不存 `user_id`。

| 状态 | 谁能访问 | 适用场景 |
|---|---|---|
| 默认（`NORVELS_AUTH_TOKEN` 为空） | 能连上端口的所有人 | 本机 / 家庭内网 |
| 设置 `NORVELS_AUTH_TOKEN` | 持有共享口令的人 | 公网 VPS、共享主机 |

- 鉴权实现：`Authorization: Bearer <token>` 或登录后下发的 Cookie（HttpOnly + SameSite=Lax + Path=/）；
- 令牌比较用 `crypto/subtle.ConstantTimeCompare`；`/api/health` 永远免鉴权（容器健康检查）；
- 开放模式启动会打印 `WARN: 未启用鉴权，请勿暴露到公网`；
- 只有一个共享口令，**权限模型等价于全有或全无**：拿到口令即可导入、读取、改进度。
  需要多用户 / 权限分级时必须先引入账号体系（新开 ADR）。

## 2.2 升级模型（版本化迁移）

数据库结构变更走**编号迁移**，不再每次启动跑整份 `schema.sql`：

```
internal/store/migrations/
  0001_init.sql              初始结构（v1，原 schema.sql 原样搬入）
  0003_reading_progress.sql  阅读进度表
```

| 规则 | 说明 |
|---|---|
| 记录表 | `schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)` |
| 应用顺序 | 按文件名版本号升序，只应用「大于当前版本」的迁移 |
| 事务粒度 | **一条迁移一个事务**；失败整体回滚并让进程启动失败（不带半迁移的库继续跑） |
| 打包方式 | `//go:embed migrations/*.sql`，随二进制分发，镜像内无需额外文件 |
| 版本号 | 只增不减，允许不连续（如 1 → 3）；改名或删除已发布的迁移会破坏已升级的库 |
| 回滚 | 迁移不提供 down 脚本；回退靠备份恢复（见 [deploy.md](deploy.md) §7） |
| dry-run | `norvels migrate --db <path> --dry-run` 只读打印待应用迁移 |

**老库兼容**（v0.1.x → 新版本）：老库的特征是 `PRAGMA user_version=1`、业务表已存在、
且没有 `schema_migrations` 表。首次升级时会识别这种状态，直接把版本 1 记为已应用
（**不重跑 0001 的建表 DDL**），然后只应用其后的迁移；数据条数不变。

**带数据修正的迁移**（如 0013 的卷名去重）：修正与建约束在同一个事务里、由 Go 步骤完成
（先改数据再建索引，SQLite 没有条件 DDL）；改了哪些行由步骤打印在迁移日志里
（`迁移 0013：同书卷名去重改名 N 条`），便于核对这台机器到底改了几条。

## 3. 数据模型（SQLite）

```
authors(id, name UNIQUE, book_count)
books(id, title, author_id, source_path UNIQUE, source_name, file_size, file_sha256 UNIQUE,
      encoding, word_count, chapter_count, intro, status, imported_at, updated_at)
volumes(id, book_id, idx, title)                 -- 卷/部；UNIQUE(book_id, idx)，同书 TRIM(title) 唯一（0013）
chapters(id, book_id, volume_id, idx, title, content, word_count)
-- 检索范围：books.title / authors.name / chapters.title 的 LIKE（无正文全文索引）
tags(id, name) / book_tags(book_id, tag_id)
import_jobs(id, source_dir, status, total, imported, skipped, failed, started_at, finished_at, message)
import_errors(id, job_id, file_path, reason, created_at)
```

完整 DDL 见 `internal/store/migrations/0001_init.sql`（v1 快照仍保留在 `internal/store/schema.sql` 供阅读，启动不再执行它）；逐版本变更见 [deploy.md](deploy.md) §7.1 的迁移表。

检索范围是**书名 / 作者 / 章节标题**，不含正文全文索引（理由与后果见 [ADR-0002](adr/0002-drop-fulltext-index.md)）。
几万本的容量与延迟推算见 [data-profile.md §10](data-profile.md#10-容量推算几万本)。

## 4. 导入流水线

```
扫描目录 ─► 逐文件: 读字节
                 ├─ 编码探测（多候选解码 + CJK 命中率打分，选最优）
                 ├─ 文本规范化（BOM 去除、CRLF→LF、全角空格、广告/水印行清洗）
                 ├─ 元数据抽取（文件名规则 → 正文头部「作者：」「书名：」→ 兜底用文件名）
                 ├─ 章节切分（多级正则 + 卷/章嵌套 + 短篇兜底 + 单文件多书合集拆分）
                 └─ 写入（books/authors/volumes/chapters，单文件一事务）
                              └─ 预计算 metrics（store_metrics，同事务累加）
```

## 5. 端口与运行

- 默认监听 `0.0.0.0:8080`，DB 路径 `/data/norvels.db`（本地默认 `./data/norvels.db`）。
- 健康检查 `GET /api/health` 返回 `{"status":"ok","db":"ok","version":"..."}`。
