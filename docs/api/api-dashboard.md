# 元信息与驾驶舱

## 元信息

### GET /api/health
```json
{ "status": "ok", "db": "ok", "version": "0.1.0", "uptimeSeconds": 12 }
```

### GET /api/stats
```json
{ "books": 865, "chapters": 1234567, "authors": 412, "words": 2200000000, "dbBytes": 5100000000, "lastImportAt": "2026-10-24T21:30:00+08:00" }
```

## 驾驶舱端点（已实现）

驾驶舱（仅 PC 可见）的配套端点，2026-10 起已实现（迁移 `0003_reading_progress.sql`）。

### GET /api/reading/progress
最近在读（按更新时间倒序，默认 5 条）。
```json
{ "items": [{ "bookId": 1, "bookTitle": "…", "chapterId": 42, "chapterIdx": 42, "chapterTitle": "第42章 …", "percent": 0.36, "paraIndex": 12, "updatedAt": "…" }] }
```

### PUT /api/reading/progress
body `{ "bookId": 1, "chapterId": 42, "percent": 0.36, "paraIndex": 12 }`（`paraIndex` 缺省 0），upsert，204。
同事务内若该书 `readStatus = ""` 则置为 `reading`；`want` / `done` 不被覆盖（唯一自动转移）。
无账号体系：单用户，进度直接挂 book/chapter（见 [ADR-0001](../adr/0001-public-release.md)）。

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

- `textBytes`：正文压缩块（`chapter_blocks.packed`）与书目文本列之和，即**压缩后**的实际占用；
- `textBytesRaw`：正文压缩前的原文逻辑字节，只用于说明压缩比，不参与占比；
- `indexBytes`：库文件大小减去正文与书目文本后的近似值（btree 页、表头等）；
- `otherBytes` = `dbBytes` − 前两者（页头与空闲页，通常只有几 KB）；**三者之和恒等于 `dbBytes`**，
  可直接画堆叠条，不会出现负值。

### 实现要点

- 阅读进度存 `reading_progress(book_id PK, chapter_id, chapter_idx, percent, updated_at)`，
  `book_id` 外键 `ON DELETE CASCADE`（删书自动清进度），按 `updated_at` 倒序取最近在读；
- `PUT /api/reading/progress` 校验顺序：JSON 形状 → `bookId`/`chapterId` 为正 → `percent ∈ [0,1]`
  → `paraIndex ≥ 0` → 章节存在且属于该书。分别返回 400 / 400 / 400 / 400 / 404，不做静默截断；
- `GET /api/health/report` 的每项统计**独立查询**：单项失败该字段为 `null` 并追加进 `warnings`，
  整体仍 200；分桶边界写死在 `internal/store/health.go`：
  - 章节数：1 / 2-9 / 10-49 / 50-199 / 200-999 / 1000+
  - 章节字数：<1千 / 1千-3千 / 3千-5千 / 5千-1万 / 1万+
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

## 其它端点

- `GET /api/tags` → `{ "items": [{ "id": 1, "name": "武侠", "bookCount": 120 }] }`
- `GET /api/version` → `{ "version": "0.1.0", "commit": "abc123", "builtAt": "…" }`
- 前端 SPA 路由：非 `/api/*` 且非静态资源命中时，返回 `index.html`（200）。
- 静态资源带内容哈希，`/assets/*` 设置 `Cache-Control: public, max-age=31536000, immutable`。
