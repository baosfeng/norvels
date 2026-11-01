# norvels 架构：组件划分

> 目标：把 4.4GB / 865 个中文 txt 一次性结构化入库，提供「书架 → 目录 → 阅读 → 找书」的完整体验；同时保持 **单二进制 + 两个 SQLite 文件（主库 + 正文库）+ 轻量容器镜像（目标 < 50MB）**。

## 1. 组件全景

```
                 ┌────────────────────────── norvels (单个 Go 二进制) ──────────────────────────┐
  txt 目录 ──►   │  importer  ──►  parser  ──►  store(SQLite)    api(HTTP)  ◄── webui(embed 前端) │
   (865 files)   │  扫描/编排       编码+元数据+切章     写入/查询/迁移      REST/JSON     静态资源     │
                 └───────────────────────────────────────────────────────────────────────────────┘
                                        │                              │
                             data/norvels.db            data/norvels-content.db
                          (元数据/用户/聚合，几十 MB)        (chapter_blocks，GB 级)
```

| 组件 | 路径 | 职责 | 对外契约 |
|---|---|---|---|
| **parser** | `internal/parser` | 纯函数：字节流 → `*parser.Book`。编码探测（UTF-8/BOM/GB18030/UTF-16/Big5/Shift-JIS/Big5-HKSCS）、清洗广告行、书名/作者抽取（文件名 + 正文元信息块）、章节切分（第X章/节/回/卷/部、数字行、Chapter N、序章/楔子/番外；无标记短篇兜底）、合集拆分、字数统计 | `parser.ParseFile(path) (*Book, error)`、`parser.ParseBytes(name, b) (*Book, error)` |
| **source** | `internal/source` | 书源规则包模型与跨边界校验：规则是**数据**不是代码；导入时由 `importer` 编译进 `parser.Options` 并按 `match` 选一份 | `source.Normalize` / `source.CheckAll` |
| **store** | `internal/store` | SQLite 打开/PRAGMA/**版本化迁移**、所有写读查询、事务批次 | `store.Open(dsn)`、`Migrate()` / `MigrateTo(target)`、`*store.Store` 上的方法（见 `internal/store/types.go`） |
| **importer** | `internal/importer` | 目录扫描（递归）、并发 worker、去重（文件 SHA-256 + 书名作者指纹）、导入任务记录、失败清单、断点续跑、进度回调 | `importer.Run(ctx, store, importer.Options)` |
| **api** | `internal/api` | REST API、中间件（日志/gzip/CORS/panic 恢复）、分页、搜索、静态前端 `http.Handler` | `api.New(store, embedFS, opts) http.Handler` |
| **webui · web** | `internal/webui` · `web/` | React 19 + TS + Vite 前端（书架/目录/阅读器/搜索/作者/导入），`go:embed` 进二进制 | `webui.FS() fs.FS`，调 `/api/*` |
| **cmd/norvels** | `cmd/norvels` | 子命令分发：`import` / `serve` / `stats` / `check-content` / `reconcile-content` / `migrate` / `version` | — |
| **config · deploy** | `internal/config` · `deploy/` | 运行配置收敛（flag > env > 默认，统一 `NORVELS_` 前缀）；多阶段 Dockerfile + compose + 健康检查 | `config.FromEnv()` |

## 2. 关键约束

1. **CGO-free**：使用 `modernc.org/sqlite`（纯 Go），才能 `CGO_ENABLED=0` 静态编译进 `scratch`/distroless。
2. **数据与代码分离**：主库 `/data/norvels.db` + 正文库 `/data/norvels-content.db`（同一个容器 volume），镜像内不含任何小说内容。
3. **前端只经 API**：不引入 SSR，构建产物全部 embed，运行时无需 Node。
4. **导入可重复执行**：同一目录重复 `import` 必须幂等（文件 SHA-256 命中则跳过，除非 `--force`）。
5. **大文件友好**：单文件最大 42MB、单库 4.4GB 文本；导入使用事务分批（每 N 章或每文件提交），WAL 模式，`synchronous=NORMAL`；聚合口径在导入时同事务预计算进 `store_metrics`，不回填全表。
6. **正文独立成库**（0018）：块表 `chapter_blocks` 只在正文库里，`chapters.block_id` 是**跨库弱引用**（没有外键）；两库各自一个 `*sql.DB`，`Open` 由主库路径推导兄弟文件、`Close` 双关。

## 2.1 鉴权边界与账号归属

**书库共享，个人记录按账号隔离**（迁移 0016）：books / authors / chapters / tags 是同一份书库，
不做多租户；`user_id` 只出现在个人数据上（进度 / 会话 / 书签 / 书单 / 我的书架）。

- **默认**（`NORVELS_AUTH_TOKEN` 为空）：能连上端口的所有人，个人记录归默认账号 `reader`（本机 / 家庭内网）。
- **设置口令**：持有口令的人，个人记录按会话 Cookie 归属（公网 VPS、共享主机）。
- 口令管「能不能进门」（`Bearer` 或登录 Cookie，HttpOnly + SameSite=Lax，`crypto/subtle` 恒定时间比较），
  会话 `norvels_session` 管「记录属于谁」，两者并存；只注册 + 输账号登录，**没有密码**。
- `/api/health` 永远免鉴权（容器健康检查）；共享口令是**全有或全无**，不是权限分级——要真正的权限模型必须换掉这套鉴权（新开 ADR，见 [api-account](api/api-account.md)）。

## 2.2 升级模型（版本化迁移）

数据库结构变更走**编号迁移**，不再每次启动跑整份 `schema.sql`：全部在 `internal/store/migrations/NNNN_名称.sql`
（如 `0018_content_db.sql`、`0019_chapter_fingerprints.sql`），版本号只增不补。

| 规则 | 说明 |
|---|---|
| 记录表 | `schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)` |
| 应用顺序 | 按文件名版本号升序，只应用「大于当前版本」的迁移 |
| 事务粒度 | **一条迁移一个事务**；失败整体回滚并让进程启动失败（不带半迁移的库继续跑） |
| 打包方式 | `//go:embed migrations/*.sql`，随二进制分发，镜像内无需额外文件 |
| 版本号 | 只增不减，允许不连续（如 1 → 3）；改名或删除已发布的迁移会破坏已升级的库 |
| 回滚 | 迁移不提供 down 脚本；回退靠备份恢复（见 [deploy.md](deploy.md) §7） |
| dry-run | `norvels migrate --db <path> --dry-run` 只读打印待应用迁移 |

**老库兼容**：`PRAGMA user_version=1` + 业务表已存在 + 无 `schema_migrations` 的库，首次升级直接记版本 1 为已应用（不重跑 0001 的建表 DDL）。

**带数据修正的迁移**（如 0013 卷名去重、0018 正文搬库）：修正与建约束由 Go 步骤在同一事务后段完成，改了哪些行打印在迁移日志里，便于核对该机器实际改了几条。

## 2.3 双库布局与一致性

WAL 下 SQLite 的原子提交只覆盖单个文件，**不依赖跨库事务**，改用「顺序 + 补偿 + 对账」：写路径统一走
`store.writeThrough`（正文库写块 → 主库写 books/volumes/chapters → 主库失败按块 id 删这次新写的块）；删书与块回收
**延后到主库提交之后**（真删而主库失败会留悬空引用，留孤儿块只是体积虚高）；漂移由 `ReconcileContent` 对账入 `/api/health/report`。

**内容去重（章级指纹，0019）+ 版本择优（质量分，0020）+ AI 审核（0021）**：导入前按**正文**判重（与文件名/编码无关）——正文归一化（删空白、
全角半角标点统一）后取 64 位 FNV-1a，指纹存正文库 `chapter_fingerprints(hash, book_id, bytes)`，命中率按字节加权；≥ 阈值（settings 的
`dedupe.threshold`，0019 预置 0.95）按 Skipped 跳过，≥ 40% 备注「疑似相关」后照常导入，`force` / `update`（追更）一律跳过判定。命中率到
阈值时再比质量分（books 七列，0020）：噪声率降 ≥50% / 乱码率降 ≥80% / 结构分升 ≥0.1 里至少两项显著更优、且无一项显著变差 → 「升级候选」。
**升级候选（0021）**：新版本照常入库但立刻 `superseded_by = 旧书 id`（书架/搜索/详情/阅读只认 `superseded_by = 0`），并落一条
`import_reviews`；后台 worker（单线程，`ai.enabled` 默认关）按章 idx 对齐两版正文、抽 ≤50 段差异段交给 `internal/ai` 判两次，
不一致 / 超时 / 无 key 一律降级 `needs_manual`；替换 / 保留 / 回滚只改这两个整数字段，**绝不自动替换**。质量分在 `SaveBook` 同事务写入，
没有质量分的书（EPUB / 合集子书）写「未计算」而非 0 分；`backfill-quality` / `backfill-fingerprints` 补历史书。细则见 [设计定稿](specs/2026-11-01-content-db-and-dedupe-design.md) §4–§7。

## 3. 数据模型（SQLite）

```
authors(id, name UNIQUE, book_count)
books(id, title, author_id, source_path UNIQUE, source_name, file_size, file_sha256 UNIQUE, encoding, word_count,
      chapter_count, intro, status, imported_at, updated_at, superseded_by)  -- superseded_by=0 是当前版本（0021）
volumes(id, book_id, idx, title)                 -- 卷/部；UNIQUE(book_id, idx)，同书 TRIM(title) 唯一（0013）
chapters(id, book_id, volume_id, idx, title, word_count, block_id, block_off, block_len)
                                                 -- block_id 指向**正文库**的块（跨库弱引用，无外键）
tags(id, name) / book_tags(book_id, tag_id)       -- 检索范围只有书名/作者（LIKE），无全文索引
import_jobs(...) / import_errors(...) / import_reviews(...)  -- 导入任务、失败明细与 AI 审核记录（0021）

-- 以下一张表在**正文库**（data/norvels-content.db），与主库同目录、同寿命
chapter_blocks(id, book_id, seq, first_idx, last_idx, raw_bytes, packed)
                                                 -- 每块约 8 MB 原始正文，0x03 码本 + zstd
chapter_fingerprints(hash, book_id, bytes)       -- 章级内容指纹（0019，WITHOUT ROWID，主键 (hash, book_id)）
```

完整 DDL 见 `internal/store/migrations/0001_init.sql`（v1 快照仍保留在 `internal/store/schema.sql` 供阅读，启动不再执行它）；逐版本变更见 [deploy.md](deploy.md) §7.1 的迁移表。

检索范围是**书名 / 作者**（章节标题与正文都不参与，无全文索引，理由见 [ADR-0002](adr/0002-drop-fulltext-index.md)）；几万本的容量推算见 [data-profile.md §10](data-profile.md#10-容量推算几万本)。

## 4. 导入流水线

```
来源规则包（source）─► 编译一次 → 按 match 选一份 → 注入 parser Options（命中才用，未命中走内置）
扫描目录 ─► 逐文件: 读字节
                 ├─ 编码探测（多候选解码 + CJK 命中率打分，选最优）
                 ├─ 文本规范化（BOM 去除、CRLF→LF、全角空格、广告/水印行清洗）
                 ├─ 元数据抽取（文件名规则 → 正文头部「作者：」「书名：」→ 兜底用文件名）
                 ├─ 章节切分（多级正则 + 卷/章嵌套 + 短篇兜底 + 单文件多书合集拆分）
                 └─ 写入（块 → 正文库事务；books/volumes/chapters → 主库事务，见 §2.3）
                              └─ 预计算 metrics（store_metrics，主库同事务累加）
```

## 5. 端口与运行

- 默认监听 `0.0.0.0:8080`；主库 `/data/norvels.db`、正文库 `/data/norvels-content.db`（本地默认 `./data/norvels.db`，正文库由主库路径推导，可用 `NORVELS_CONTENT_DB` 覆盖）。
- 健康检查 `GET /api/health` 返回 `{"status":"ok","db":"ok","version":"..."}`。
