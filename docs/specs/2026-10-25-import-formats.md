# 导入扩展：上传 · EPUB · 元数据（2026-10-25）

> 目标：让书以「文件」进去（浏览器上传 + EPUB），带出封面/分类/元数据；不支持的类型当场友好报错。
> 边界：只存封面这一种二进制；插图/字体/CSS/MOBI/AZW3/PDF/DOCX/DRM 解密不做。ZIP 批量、合集拆分、追更增量见 [2026-10-25-import-p1.md](2026-10-25-import-p1.md)。

## 1. 用户可见行为

- 导入页支持选择或拖拽文件，一次最多 20 个，单文件 ≤64 MiB，总量 ≤256 MiB。
- 支持 `.txt` 与 `.epub`（大小写不敏感）。其余扩展名**逐个**提示「暂不支持 .pdf，目前支持 TXT 与 EPUB」，同批合法文件继续导。
- EPUB 封面出现在书架与详情页；没有封面的书继续用首字占位。
- 书籍详情可改书名/作者/简介/标签/系列；保存后列表与详情立即反映。

## 2. 接口契约

### POST /api/import/upload
`multipart/form-data`：`files`（可重复）、`force`（文本 true/false）。
流式读取（`r.MultipartReader()`），不整体进内存。文件落 `<dbDir>/uploads/<jobId>/`，导入结束删除。

```json
202 { "jobId": 7,
      "accepted": [{ "name": "a.epub", "format": "epub" }],
      "rejected": [{ "name": "b.pdf", "code": "unsupported_format",
                     "message": "暂不支持 .pdf，目前支持 TXT 与 EPUB" }] }
```

| 情况 | HTTP | code |
|---|---|---|
| 全部文件都不支持 | 400 | `unsupported_format` |
| 单文件 >64 MiB 或总量 >256 MiB | 413 | `payload_too_large` |
| 文件数 >20 | 400 | `too_many_files` |
| 加密 EPUB（含 META-INF/encryption.xml） | 422 | `encrypted_epub` |
| 已有导入任务在跑 | 409 | `conflict` |

部分文件被拒时仍返回 202：`rejected` 列出原因，`accepted` 的文件建一个任务导入。
上传后 `source_path = upload://<sha256>/<文件名>`，`source_name` 为原文件名。

### GET /api/books/{id}/cover
200 `image/jpeg|png|webp|gif`，`Cache-Control: public, max-age=86400`；无封面 404 `not_found`。

### PATCH /api/books/{id}
body 为任意子集：`title`、`author`、`authors`（字符串数组，全量替换）、`intro`、`tags`（字符串数组，全量替换）、`series`、`seriesIndex`；`intro`/`series` 传空串即清空，`seriesIndex` 传 0 即清空（后续增量见 import-p1.md）。
成功 200 返回 BookDetail；空 body 400；id 不存在 404。作者变化要重算 `authors` 聚合列。

### GET /api/books/{id} 增量字段
`language`、`publisher`、`isbn`、`pubDate`、`series`、`seriesIndex`、`hasCover`、`format`。

## 3. 数据模型（迁移 0008，只加列/加表，不删数据，无需 VACUUM）

```sql
ALTER TABLE books ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN publisher TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN isbn TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN pub_date TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN source_format TEXT NOT NULL DEFAULT 'txt';
ALTER TABLE books ADD COLUMN has_cover INTEGER NOT NULL DEFAULT 0;
CREATE TABLE book_covers(
  book_id INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
  mime TEXT NOT NULL, bytes BLOB NOT NULL, sha256 TEXT NOT NULL,
  updated_at TEXT NOT NULL);
```

分类复用 `tags`/`book_tags`：EPUB `dc:subject` 逐个写入标签，PATCH 可全量替换。

## 4. 解析契约（internal/ebook）

```go
func Open(name string, data []byte) (*Book, error)
type Book struct { Title, Author, Intro, Language, Publisher, ISBN, PubDate, Series string
                   SeriesIndex int; Subjects []string; Cover *Cover; Chapters []Chapter; Warnings []string }
type Cover struct { MIME string; Bytes []byte }
type Chapter struct { Title, Content string }
var ErrEncrypted = errors.New("ebook: encrypted")
```

- `container.xml` → OPF → manifest/spine；spine 每个 XHTML 一章。
- 章节标题：首个 `h1/h2/h3` → `<title>` → 文件名；正文按块级元素分段、跳过 script/style、ruby 保留 base、实体解码。
- 封面：manifest `properties="cover-image"` → `meta name="cover"` → guide `cover` → 文件名 `cover.*`。
- 解包上限：条目 ≤2000、解压总量 ≤200 MiB、单条目 ≤16 MiB（Zip 炸弹防护）。
- spine ≤2 且总文本 >20000 字：把全文交给现有 parser 切章（复用 di-unit 等策略）。
- 元数据缺失：书名/作者回落现有文件名规则；封面 >2 MiB 或 mime 不在白名单则丢弃并记 warning。

importer 按扩展名分发：`.txt` → parser（现状不变）；`.epub` → ebook → `model.ParsedBook`。

## 5. 工程化决定

| 能力 | 决定 | 程度 | 依据 |
|---|---|---|---|
| 上传校验 | 做 | 扩展名 + 大小 + 数量 + epub 加密检测 | 跨信任边界输入 |
| 友好报错 | 做 | 逐文件 `code` + 人话 `message` | 用户明确要求 |
| 并发 | 省 | 复用现有单任务互斥 | 已有机制 |
| 重试 | 省 | 单用户本地、失败可重传 | 无外部依赖 |
| 日志 | 做 | 失败原因进 `import_errors` | 沿用现有口径 |

资源：临时目录峰值 ≤256 MiB（导入后删）；封面每本 ≤2 MiB；每本书一次事务，写放大与现状一致。

## 6. 验收

- 上传 txt → 入库，章节数 >0；重复上传同一文件 → skipped。
- 上传 epub → 书名/作者/简介/标签/封面正确；无封面书显示占位。
- 上传 `.pdf` + `.txt` 混批 → pdf 进 `rejected` 且 message 友好，txt 正常入库。
- 上传 >64 MiB → 413 `payload_too_large`，message 含上限。
- 加密 epub → `encrypted_epub`，不建任务、不写半本书。
- PATCH 改书名/标签 → GET /api/books/{id} 立即反映；作者变更后 /api/authors 聚合正确。
- 封面接口 200 + Content-Type；无封面 404。
- 回归：现有目录导入、dry-run、--force、单任务互斥行为不变。
