# norvels 架构：组件划分

> 目标：把 4.4GB / 865 个中文 txt 一次性结构化入库，提供「书架 → 目录 → 阅读 → 全文搜索」的完整体验；
> 同时保持 **单二进制 + 单 SQLite 文件 + 轻量容器镜像（目标 < 50MB）**。

## 1. 组件全景

```
                 ┌────────────────────────── norvels (单个 Go 二进制) ──────────────────────────┐
  txt 目录 ──►   │  importer  ──►  parser  ──►  store(SQLite)    api(HTTP)  ◄── webui(embed 前端) │
   (865 files)   │  扫描/编排       编码+元数据+切章     写入/查询/迁移      REST/JSON     静态资源     │
                 └───────────────────────────────────────────────────────────────────────────────┘
                                                        │
                                                   data/norvels.db  (SQLite + FTS5)
```

| 组件 | 路径 | 职责 | 对外契约 |
|---|---|---|---|
| **parser** | `internal/parser` | 纯函数：字节流 → `*parser.Book`。编码探测（UTF-8/BOM/GB18030/UTF-16/Big5）、清洗广告行、书名/作者抽取（文件名 + 正文元信息块）、章节切分（第X章/节/回/卷/部、数字行、Chapter N、序章/楔子/番外；无标记短篇兜底）、合集拆分、字数统计 | `parser.ParseFile(path) (*Book, error)`、`parser.ParseBytes(name, b) (*Book, error)` |
| **store** | `internal/store` | SQLite 打开/PRAGMA/迁移、所有写读查询、FTS5 索引维护、事务批次 | `store.Open(dsn)`、`*store.Store` 上的方法（见 `internal/store/types.go`） |
| **importer** | `internal/importer` | 目录扫描（递归）、并发 worker、去重（文件 SHA-256 + 书名作者指纹）、导入任务记录、失败清单、断点续跑、进度回调 | `importer.Run(ctx, store, importer.Options)` |
| **api** | `internal/api` | REST API、中间件（日志/gzip/CORS/panic 恢复）、分页、搜索、静态前端 `http.Handler` | `api.New(store, embedFS, opts) http.Handler` |
| **webui** | `internal/webui` | `go:embed` 前端构建产物（`web/dist` → `internal/webui/dist`） | `webui.FS() fs.FS` |
| **web** | `web/` | React 19 + TypeScript + Vite + Tailwind 前端：书架 / 书籍详情目录 / 阅读器 / 搜索 / 作者 / 导入管理 | 调 `/api/*`（见 `docs/api.md`） |
| **cmd/norvels** | `cmd/norvels` | 子命令分发：`import` / `serve` / `stats` / `reindex` / `version` | — |
| **deploy** | `deploy/` | 多阶段 Dockerfile（node 构建前端 → Go 静态编译 → distroless/static）、compose、健康检查 | — |

## 2. 关键约束

1. **CGO-free**：使用 `modernc.org/sqlite`（纯 Go），才能 `CGO_ENABLED=0` 静态编译进 `scratch`/distroless。
2. **数据与代码分离**：数据库放 `/data/norvels.db`（容器 volume），镜像内不含任何小说内容。
3. **前端只经 API**：不引入 SSR，构建产物全部 embed，运行时无需 Node。
4. **导入可重复执行**：同一目录重复 `import` 必须幂等（文件 SHA-256 命中则跳过，除非 `--force`）。
5. **大文件友好**：单文件最大 42MB、单库 4.4GB 文本；导入使用事务分批（每 N 章或每文件提交），WAL 模式，`synchronous=OFF`，最后再建/重建 FTS 索引。

## 3. 数据模型（SQLite）

```
authors(id, name UNIQUE, book_count)
books(id, title, author_id, source_path UNIQUE, source_name, file_size, file_sha256 UNIQUE,
      encoding, word_count, chapter_count, intro, status, imported_at, updated_at)
volumes(id, book_id, idx, title)                 -- 卷/部，可为空
chapters(id, book_id, volume_id, idx, title, content, word_count)
chapters_fts (FTS5, external content = chapters)  -- 全文检索
tags(id, name) / book_tags(book_id, tag_id)
import_jobs(id, source_dir, status, total, imported, skipped, failed, started_at, finished_at, message)
import_errors(id, job_id, file_path, reason, created_at)
```

完整 DDL 见 `internal/store/schema.sql`。

## 4. 导入流水线

```
扫描目录 ─► 逐文件: 读字节
                 ├─ 编码探测（多候选解码 + CJK 命中率打分，选最优）
                 ├─ 文本规范化（BOM 去除、CRLF→LF、全角空格、广告/水印行清洗）
                 ├─ 元数据抽取（文件名规则 → 正文头部「作者：」「书名：」→ 兜底用文件名）
                 ├─ 章节切分（多级正则 + 卷/章嵌套 + 短篇兜底 + 单文件多书合集拆分）
                 └─ 写入（books/authors/volumes/chapters，单文件一事务）
                              └─ FTS 索引（触发器同步 or 批量 rebuild）
```

## 5. 端口与运行

- 默认监听 `0.0.0.0:8080`，DB 路径 `/data/norvels.db`（本地默认 `./data/norvels.db`）。
- 健康检查 `GET /api/health` 返回 `{"status":"ok","db":"ok","version":"..."}`。
