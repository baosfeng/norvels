# 元信息与驾驶舱

## 元信息

### GET /api/health
```json
{ "status": "ok", "db": "ok", "version": "0.1.0", "uptimeSeconds": 12 }
```

### GET /api/stats
```json
{ "books": 865, "chapters": 1234567, "authors": 412, "words": 2200000000, "dbBytes": 5100000000,
  "textBytes": 131072, "textBytesRaw": 402000, "indexBytes": 16384, "otherBytes": 8192, "compositionEstimated": false,
  "lastImportAt": "2026-10-24T21:30:00+08:00" }
```

- `dbBytes` 是**主库 + 正文库**（0018 起两个文件，各自含 WAL）的合计，即真实占用；
  `textBytes` 是压缩块与书目文本列之和（**压缩后**），`textBytesRaw` 是压缩前原文逻辑字节（只看压缩比），
  `indexBytes` 是主库文件大小减书目文本的近似值，`otherBytes` = 前两者之外（三者之和恒等于 `dbBytes`）。
- `books` / `words` 读 0026 的 `book_totals`（4 行小表），`chapters` 读 0022 的 `store_metrics`：
  两条原先都是请求路径上的全表扫（生产 18.2 万本时书目字数聚合 p95 261 ms）。预计算不可用时回退
  实时 SQL，**数字不变、这一次慢**，`compositionEstimated=true` 让运维看见「这次走了慢路径」。

## 驾驶舱端点（已实现）

### GET /api/dashboard

**驾驶舱一次刷新只需要它**：一次返回「数字类」7 块，服务端并发取数（原先浏览器要发 10 个请求）。

```json
{ "generatedAt": "2026-11-07T05:34:15Z", "cached": false, "ageSeconds": 0.002, "ttlSeconds": 5, "warnings": [],
  "stats": {…同 /api/stats}, "health": {…同 /api/health/report}, "readingStats": {…同 /api/stats/reading},
  "importTrend": [...同 /api/stats/import-trend], "readingProgress": [...同 /api/reading/progress 的 items],
  "topAuthors": { "items": [], "total": 0, "page": 1, "size": 10 }, "importJobs": [...同 /api/import/jobs 的 items] }
```

- 参数：`days`（14）/ `readingDays`（90）/ `tz`（0）/ `topAuthors`（10，≤50）/
  `progress`（50，≤50）/ `jobs`（20，≤100）；越界一律 **400**（不静默夹紧）。
- 每块的字段形状与它自己的端点逐字一致，块内 `stale` / `compositionEstimated` 语义不变
  （聚合层不改写）；`warnings` 里是「哪一块没取到 + 原因」。
- **部分失败不整体失败**：`stats` 是首屏核心，取不到 → 整体 5xx（与 `/api/stats` 同口径）；
  其余块失败只进 `warnings` 并把该块置 `null`（数组块置空数组）。
- **短 TTL 缓存**：默认 5 秒（`NORVELS_DASHBOARD_TTL` 改时长，负值关闭）。缓存键 = 参数 + 账号
  （`readingStats` / `readingProgress` 按 `user_id` 隔离，不同账号不共享条目）。TTL 内 `cached=true`、
  `ageSeconds` 是这份数据的年龄；TTL 外同一时刻只有一个请求真算、其余等它算完再复用
  （single-flight：刷新连点不会把 7 组查询打成 7 倍）。**只缓存七块都齐且自洽的结果**：
  带 `warnings`（某块失败，或空块与 `stats` 矛盾，如书数 > 0 而健康卡说 0 本）的响应不进缓存，
  下一次请求重算——这类 `warnings` 的文案是「<块> 尚未就绪（可能是启动回填/瞬时空结果）」。
- 指标类刻意留在外面：`/api/metrics`（进程内快照，必须反映当下）与 `/api/recommend/stats`
  （依赖独立推荐库，缺库是 503）。于是驾驶舱一次刷新 = `dashboard` + `metrics` + `recommend/stats` = **3 个请求**。

### GET /api/reading/progress
最近在读（按更新时间倒序，默认 5 条）。
```json
{ "items": [{ "bookId": 1, "bookTitle": "…", "chapterId": 42, "chapterIdx": 42, "chapterTitle": "第42章 …", "percent": 0.36, "paraIndex": 12, "updatedAt": "…" }] }
```

### PUT /api/reading/progress
body `{ "bookId": 1, "chapterId": 42, "percent": 0.36, "paraIndex": 12 }`（`paraIndex` 缺省 0），upsert，204。
同事务内若该书 `readStatus = ""` 则置为 `reading`；`want` / `done` 不被覆盖。
进度按 `(user_id, book_id)` 归属当前账号；开放模式下所有请求都归默认账号 `reader`。

### GET /api/health/report
库健康状况（人话指标，供驾驶舱状态卡与直方图）。
```json
{ "totalBooks": 865, "thinBooks": 3, "missingAuthor": 12, "missingIntro": 40, "hugeChapters": 0, "stale": false,
  "chapterBuckets": [{ "label": "1", "count": 0 }, { "label": "2-9", "count": 12 }],
  "chapterLengthBuckets": [{ "label": "<1千", "count": 120 }], "warnings": [] }
```
单项统计失败返回 `null` 并写入 `warnings`，不让整个接口 500。`stale=true` 表示某项聚合走了
降级路径（预计算未回填 / 口径落后），数字仍正确。另有 `content`（正文对账快照，只读后台结果）、
`fingerprints`（0023 覆盖表）、`quality`（0020 质量分覆盖率，0026 起读数不再扫全表）；
缺口 > 0 时各自的 `hint` 指向回填命令。

### GET /api/health/issues?kind=missing_author&page=1&size=20
「库健康状况」卡的下钻明细：某一类问题具体是哪些书 / 哪些章节。

- `kind` 必填，取 `missing_author` | `missing_intro` | `thin_book`（书级）、`huge_chapter`（章节级）；
  非法或为空 → **400** `invalid_param`（不回落默认值，避免把两类问题混在一起看）。
- `page`/`size` 与其它列表接口同口径（默认 1 / 20，`size` ≤ 200）；越界 400。
- `total` 与 `/api/health/report` 里同名指标用**同一条 WHERE** 算出，两者恒等。
- 书级条目只填 `bookId`/`title`/`author`/`chapterCount`/`wordCount`；章节级条目额外带
  `chapterId`/`chapterIdx`/`chapterTitle`，此时 `wordCount` 是**本章**字数。排序统一「字数多的在前」。

```json
{ "kind": "huge_chapter", "total": 5, "page": 1, "size": 20, "items": [{ "bookId": 12, "title": "书目名", "chapterIdx": 6, "wordCount": 41234 }] }
```

### GET /api/stats/import-trend?days=14
近 N 天导入量（无数据的日期补 0，前端需要连续 x 轴）。
```json
[{ "date": "2026-10-24", "books": 12, "chapters": 1234 }]
```

### GET /api/authors?sort=words&size=10
在现有作者列表上加 `sort=words`，用于「字数 Top 10 作者」横条图。

### 实现要点

- 阅读进度存 `reading_progress(book_id PK, chapter_id, chapter_idx, percent, updated_at)`，
  `book_id` 外键 `ON DELETE CASCADE`，按 `updated_at` 倒序取最近在读；`PUT` 校验顺序：
  JSON 形状 → id 为正 → `percent ∈ [0,1]` → `paraIndex ≥ 0` → 章节存在且属于该书（400×4 / 404）。
- **驾驶舱请求路径上不允许全表扫**（5125 万章规模下曾是 21 秒）：慢聚合一律改成
  「写入时同事务维护 / 后台回填 + 请求读小表 + stale 标记」——`0022`（长度直方图、导入趋势、作者聚合）、
  `0023`（指纹覆盖表）、`0025`（缺简介/缺作者/超薄书/章节数分桶）、`0026`（书数 / 书目字数 /
  已算质量分书数，指纹覆盖率的分母也取它）。四个迁移都「只建结构 + 后台回填」，回填完成前降级实时 SQL、数字仍正确。
- 分桶边界在 `internal/store/health.go`：章节数 1 / 2-9 / 10-49 / 50-199 / 200-999 / 1000+；
  章节字数 <1千 / 1千-3千 / 3千-5千 / 5千-1万 / 1万+。
- 健康明细的四个 kind 与报告指标一一对应，判据（WHERE + 阈值）只在 `internal/store/health_issues.go`
  一份，报告计数与明细列表都引用它。
- `import-trend` 章节按**所属书的 `imported_at`** 归日，时区取服务器本地时区，`days` 默认 14、上限 366，
  0022 起读 `import_daily`；`authors?sort=words` 0022 起读 `author_agg` 的前 N 行。
- 新增端点的 `limit` / `days` 越界一律 400（不静默夹紧）；性能预算：
  `make perf-budget BASE=http://<部署>:8080`（口径见 [observability](../observability.md)）。

## 其它端点

- `GET /api/tags` → `{ "items": [{ "id": 1, "name": "武侠", "bookCount": 120 }] }`
- `GET /api/version` → `{ "version": "0.1.0", "commit": "abc123", "builtAt": "…" }`
- `GET /api/releases` → 内置发版说明（按版本倒序），见 [api-releases.md](api-releases.md)
- 前端 SPA 路由：非 `/api/*` 且非静态资源命中时返回 `index.html`（200）；`/assets/*` 带内容哈希，`Cache-Control: public, max-age=31536000, immutable`。
