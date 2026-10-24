# norvels REST API 契约 v1

Base：`/api`。全部返回 JSON（UTF-8），错误统一：

```json
{ "error": { "code": "not_found", "message": "book not found" } }
```

状态码：200 正常 / 400 参数错误 / 404 未找到 / 409 冲突（导入任务进行中）/ 500 内部错误。

分页参数统一：`page`（从 1 开始，默认 1）、`size`（默认 20，最大 200）。分页响应统一：

```json
{ "items": [], "total": 123, "page": 1, "size": 20 }
```

## 1. 元信息

### GET /api/health
```json
{ "status": "ok", "db": "ok", "version": "0.1.0", "uptimeSeconds": 12 }
```

### GET /api/stats
```json
{ "books": 865, "chapters": 1234567, "authors": 412, "words": 2200000000, "dbBytes": 5100000000, "lastImportAt": "2026-10-24T21:30:00+08:00" }
```

## 2. 书籍

### GET /api/books
查询参数：`q`（书名/作者模糊）、`authorId`、`tag`、`sort` = `updated|title|chapters|words|imported`（默认 updated）、`page`、`size`。

```json
{
  "items": [{
    "id": 1, "title": "山居杂记", "authorId": 12, "authorName": "示例作者",
    "chapterCount": 1300, "wordCount": 2800000, "volumeCount": 7,
    "intro": "……", "status": "parsed", "importedAt": "2026-10-24T21:30:00+08:00"
  }],
  "total": 865, "page": 1, "size": 20
}
```

### GET /api/books/{id}
```json
{
  "id": 1, "title": "山居杂记", "authorId": 12, "authorName": "示例作者",
  "chapterCount": 1300, "wordCount": 2800000, "intro": "…", "encoding": "gb18030",
  "sourceName": "☆文笔好10%△《山居杂记（七改）》.txt", "fileSize": 5639497,
  "volumes": [
    { "id": 1, "idx": 0, "title": "第一卷", "chapters": [ { "id": 10, "idx": 1, "title": "第1章 穿越" } ] },
    { "id": 0, "idx": -1, "title": "未分卷", "chapters": [ { "id": 9, "idx": 0, "title": "序章" } ] }
  ]
}
```
> 未归入任何卷的章节放在 `volume.id = 0, idx = -1` 的伪卷中，前端按 idx 升序渲染。

### GET /api/books/{id}/chapters/{chapterId}
```json
{
  "bookId": 1, "bookTitle": "山居杂记",
  "id": 1051, "idx": 42, "title": "第42章 风起",
  "content": "正文……",
  "wordCount": 3210,
  "prevId": 1050, "nextId": 1052
}
```
> `prevId`/`nextId` 为 0 表示到头。章节按 `(book_id, idx, id)` 排序。

### GET /api/books/{id}/chapters?from=1&limit=50
轻量章节列表（不带正文），用于目录虚拟滚动：
```json
{ "items": [{ "id": 10, "idx": 1, "title": "第1章 穿越", "volumeId": 1, "wordCount": 3210 }], "total": 1300 }
```

## 3. 作者

### GET /api/authors
参数：`q`、`page`、`size`、`sort`=`books|name`。
```json
{ "items": [{ "id": 12, "name": "示例作者", "bookCount": 3, "wordCount": 5600000 }], "total": 412, "page": 1, "size": 20 }
```

### GET /api/authors/{id}
```json
{ "id": 12, "name": "示例作者", "bookCount": 3, "books": [ /* BookSummary，同 /api/books items */ ] }
```

## 4. 搜索

### GET /api/search
参数：`q`（必填，≥1 字符）、`type`=`chapter|book|all`（默认 all）、`page`、`size`。

```json
{
  "q": "临川市",
  "books": { "items": [ /* BookSummary */ ], "total": 2 },
  "chapters": {
    "items": [{
      "bookId": 1, "bookTitle": "风流名将", "chapterId": 88, "chapterTitle": "第88章 夜宴",
      "snippet": "……华夏国<b>临川市</b>，午后的秋日……", "score": 12.3
    }],
    "total": 57, "page": 1, "size": 20
  }
}
```
> `snippet` 含 `<b>` 高亮标记（其余 HTML 字符已转义）。前端必须做**受限渲染**：按 `<b>…</b>` 拆段 → 逐段转义 → 再拼回 `<b>`，不要直接把后端字符串塞进 `dangerouslySetInnerHTML`。

**检索约束（实现现状，前端必须处理）**

- 全文检索是**字级分词**（FTS5 `unicode61` + 汉字逐字分段，查询串过同一个 `Segment` 函数）：
  - `q` trim 后 **少于 2 个字符** → `400` `{"error":{"code":"query_too_short","message":"关键词至少 2 个字"}}`；
  - 中文两字词正常工作（`临川市`、`秋蝉`），前端不需要自己分词。
- 响应可能带可选字段 `notice`（string），说明降级原因（如关键词过于常见被截断）；前端有则提示、无则忽略。

**错误码一览**

| code | HTTP | 含义 |
|---|---|---|
| `invalid_param` | 400 | 参数缺失/格式错（page/size/id/dir/percent 等） |
| `unauthorized` | 401 | 启用鉴权后未提供或提供了错误的令牌 |
| `query_too_short` | 400 | 搜索关键词少于 2 字符 |
| `not_found` | 404 | 书籍/章节/作者/任务不存在，或未知 `/api/*` 路径 |
| `method_not_allowed` | 405 | 路径存在但方法不对（带 `Allow` 头） |
| `conflict` | 409 | 已有导入任务在运行 |
| `unavailable` | 503 | 数据库不可用（`/api/health`） |
| `internal` | 500 | 未预期错误（已脱敏） |

## 5. 导入管理

### POST /api/import
```json
{ "dir": "/books", "recursive": true, "force": false, "workers": 4, "dryRun": false }
```
→ 202 `{ "jobId": 7 }`；已有运行中的任务时返回 409。

### GET /api/import/jobs
```json
{ "items": [{ "id": 7, "sourceDir": "/books", "status": "running", "total": 865, "imported": 120, "skipped": 3, "failed": 1, "startedAt": "…", "finishedAt": null, "message": "" }] }
```

### GET /api/import/jobs/{id}
同上单条 + `errors: [{ "filePath": "…", "reason": "…" }]`（最多 200 条）。

## 6. 其它

- `GET /api/tags` → `{ "items": [{ "id": 1, "name": "武侠", "bookCount": 120 }] }`
- `GET /api/version` → `{ "version": "0.1.0", "commit": "abc123", "builtAt": "…" }`
- 前端 SPA 路由：非 `/api/*` 且非静态资源命中时，返回 `index.html`（200）。
- 静态资源带内容哈希，`/assets/*` 设置 `Cache-Control: public, max-age=31536000, immutable`。

## 7. 前端约定

- 阅读进度：**服务端为准**（见 §8 `/api/reading/progress`），`localStorage` 降级为离线缓存。
- 章节正文为纯文本，前端按段渲染（`\n` 分段），不做富文本。
- 搜索高亮：后端返回的 `snippet` 中只有 `<b>` 标签合法，其余 HTML 字符已转义。

## 8. 驾驶舱端点（已实现）

驾驶舱（仅 PC 可见）的配套端点，2026-10 起已实现（迁移 `0003_reading_progress.sql`）。

### GET /api/reading/progress
最近在读（按更新时间倒序，默认 5 条）。
```json
{ "items": [{ "bookId": 1, "bookTitle": "…", "chapterId": 42, "chapterIdx": 42, "chapterTitle": "第42章 …", "percent": 0.36, "updatedAt": "…" }] }
```

### PUT /api/reading/progress
body `{ "bookId": 1, "chapterId": 42, "percent": 0.36 }`，upsert，204。
无账号体系：单用户，进度直接挂 book/chapter（见 [ADR-0001](adr/0001-public-release.md)）。

### GET /api/health/report
库健康状况（人话指标，供驾驶舱状态卡与直方图）。
```json
{
  "totalBooks": 865, "thinBooks": 3, "missingAuthor": 12, "missingIntro": 40, "hugeChapters": 0,
  "chapterBuckets": [{ "label": "1", "count": 0 }, { "label": "2-9", "count": 12 }],
  "chapterLengthBuckets": [{ "label": "<1千字", "count": 120 }],
  "warnings": []
}
```
单项统计失败返回 `null` 并写入 `warnings`，不让整个接口 500。

### GET /api/stats/import-trend?days=14
近 N 天导入量（无数据的日期补 0，前端需要连续 x 轴）。
```json
[{ "date": "2026-10-24", "books": 12, "chapters": 1234 }]
```

### GET /api/authors?sort=words&size=10
在现有作者列表上加 `sort=words`，用于「字数 Top 10 作者」横条图。

### GET /api/stats（扩展）
增加体积构成估算（**估算值**，非精确页级拆解）：

```json
{ "books": 865, "chapters": 23675, "authors": 120, "words": 123456789, "dbBytes": 155648,
  "textBytes": 131072, "indexBytes": 16384, "otherBytes": 8192, "lastImportAt": "…" }
```

- `textBytes`：各表 TEXT 列字节数汇总（正文 + 标题 + 字级分词镜像列 + 简介/路径等）；
- `indexBytes`：FTS5 影子表（`chapters_fts_data` / `_idx` / `_docsize` / `_config`）字节数；
- `otherBytes` = `dbBytes` − 前两者（含 btree 页填充、表头、空闲页）；**三者之和恒等于 `dbBytes`**，
  可直接画堆叠条，不会出现负值。

### 实现要点

- 阅读进度存 `reading_progress(book_id PK, chapter_id, chapter_idx, percent, updated_at)`，
  `book_id` 外键 `ON DELETE CASCADE`（删书自动清进度），按 `updated_at` 倒序取最近在读；
- `PUT /api/reading/progress` 校验顺序：JSON 形状 → `bookId`/`chapterId` 为正 → `percent ∈ [0,1]`
  → 章节存在且属于该书。分别返回 400 / 400 / 400 / 404，不做静默截断；
- `GET /api/health/report` 的每项统计**独立查询**：单项失败该字段为 `null` 并追加进 `warnings`，
  整体仍 200；分桶边界写死在 `internal/store/health.go`：
  - 章节数：1 / 2-9 / 10-49 / 50-199 / 200-999 / 1000+
  - 章节字数：<1千 / 1千-5千 / 5千-2万 / 2万-10万 / 10万+
- `GET /api/stats/import-trend` 章节按**所属书的 `imported_at`** 归日（章节无独立导入时间），
  时区取服务器本地时区，无数据日期补 0；`days` 默认 14、上限 366；
- `GET /api/authors?sort=words` 复用列表接口，`ORDER BY SUM(word_count) DESC`，`size=10` 即「字数 Top 10」；
- 新增端点的 `limit` / `days` 越界一律 400（不静默夹紧）。

```bash
# curl 示例（开放模式；启用鉴权时加 -H 'Authorization: Bearer <token>'）
curl -s 'http://127.0.0.1:8080/api/reading/progress?limit=5'
curl -s -X PUT -d '{"bookId":1,"chapterId":42,"percent":0.36}' \
     http://127.0.0.1:8080/api/reading/progress -w '%{http_code}\n'   # → 204
curl -s 'http://127.0.0.1:8080/api/health/report'
curl -s 'http://127.0.0.1:8080/api/stats/import-trend?days=14'
curl -s 'http://127.0.0.1:8080/api/authors?sort=words&size=10'
curl -s 'http://127.0.0.1:8080/api/stats'
```

## 9. 鉴权（可选）

默认开放模式（单人本地/内网使用）。设置环境变量 `NORVELS_AUTH_TOKEN` 后：

- 所有 `/api/*` 与页面要求 `Authorization: Bearer <token>` 或 Cookie
- 免鉴权路径：`/api/health`（容器健康检查）、`GET|POST /login`、`POST /logout`
- 未通过时：`/api/*` 返回 `401 {"error":{"code":"unauthorized",…}}` 并带 `WWW-Authenticate`；
  页面请求 303 跳到 `/login?next=<原路径>`
- `GET /login` 是服务端渲染的内联 HTML 表单；`POST /login` 用 `token` 字段提交，
  成功后下发 `norvels_auth` Cookie（HttpOnly + SameSite=Lax + Path=/，30 天）并跳回 `next`
- 令牌用 `crypto/subtle.ConstantTimeCompare` 恒定时间比较；`next` 只接受站内相对路径（防开放重定向）
- 未设置时启动日志打印 `WARN: 未启用鉴权，请勿暴露到公网`

**公开部署（公网 VPS）必须先设 `NORVELS_AUTH_TOKEN`**，详见 [ADR-0001](adr/0001-public-release.md)。
