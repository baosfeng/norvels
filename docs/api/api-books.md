# 书籍与作者

## 书籍

### GET /api/books
查询参数：`q`（书名/作者模糊）、`authorId`、`tag`、`readStatus`=`want|reading|done`、`serialStatus`=`ongoing|completed|none`（`none` 筛「未知」）、`sort`=`updated|title|chapters|words|imported`（默认 updated）、`page`、`size`。留空 = 不过滤，非法值 400。

```json
{ "items": [{ "id": 1, "title": "山居杂记", "authorId": 12, "authorName": "示例作者",
  "chapterCount": 1300, "wordCount": 2800000, "volumeCount": 7, "intro": "……",
  "status": "parsed", "serialStatus": "ongoing", "readStatus": "reading",
  "importedAt": "2026-10-24T21:30:00+08:00" }],
  "total": 865, "page": 1, "size": 20 }
```

### GET /api/books/{id}
```json
{ "id": 1, "title": "山居杂记", "authorId": 12, "authorName": "示例作者",
  "chapterCount": 1300, "wordCount": 2800000, "intro": "…", "encoding": "gb18030",
  "sourceName": "☆文笔好10%△《山居杂记（七改）》.txt", "fileSize": 5639497,
  "status": "parsed", "serialStatus": "ongoing", "readStatus": "reading", "bookmarkCount": 3, "lastChapterIdx": 1299, "lastChapterTitle": "第1299章 风起",
  "volumes": [ { "id": 1, "idx": 0, "title": "第一卷",
    "chapters": [ { "id": 10, "idx": 1, "title": "第1章 穿越" } ] } ] }
```
> 未归卷的章节放在伪卷 `{ "id": 0, "idx": -1, "title": "未分卷" }` 里。增量字段（只加不改）：0008 起 `language`/`publisher`/`isbn`/`pubDate`/`series`/`seriesIndex`/`hasCover`/`format` + 详情独有 `tags`；0010 起 `serialStatus`/`lastChapterIdx`/`lastChapterTitle`；0011 起 `readStatus`（列表也有）与 `bookmarkCount`（详情独有）。三套状态互不覆写：`status`（解析）/ `serialStatus`（连载）/ `readStatus`（读者）。

### GET /api/books/{id}/chapters/{chapterId}
```json
{ "bookId": 1, "bookTitle": "山居杂记", "id": 1051, "idx": 42, "title": "第42章 风起",
  "content": "正文……", "wordCount": 3210, "prevId": 1050, "nextId": 1052 }
```
> `prevId`/`nextId` 为 0 表示到头；章节按 `(book_id, idx, id)` 排序。

### PATCH /api/books/{id}
body 为任意子集：`title`、`author`、`authors`（**全量替换**，最多 10 个；空数组 = 清空作者）、`intro`、`tags`（**全量替换**）、`series`、`seriesIndex`、`serialStatus`、`readStatus`。

- 成功 200，返回更新后的 BookDetail；`authors` 与 `author` 同时给出时 `authors` 优先；清空语义：`intro`/`series` 传 `""`，`seriesIndex` 传 `0`，`authors`/`tags` 传空数组；
- 空 body / 空对象 / 只含未知字段 / title 空白 → 400 `invalid_param`；id 非数字 → 400 `invalid_id`；书不存在 → 404；
- `serialStatus` 只接受 `""|ongoing|completed`、`readStatus` 只接受 `""|want|reading|done`，其它值 400（两者与解析状态 `status` 互不覆写）；改作者会重算新旧作者的聚合列，改 `tags` 会清理孤儿标签。

### DELETE /api/books/{id}
单事务级联删除 `chapters`/`chapter_blocks`/`volumes`/`book_covers`/`books_authors`/`reading_progress`/`bookmarks`/`book_tags`，并重算作者聚合（作者行保留）。成功 **204**；书不存在或已删 → 404 `not_found`；立即从书架/搜索/作者页消失，同一 `source_path` 可重新导入。

### 封面 /api/books/{id}/cover（GET · PUT · DELETE）
- **GET**：有封面 200，`Content-Type` 为 `image/jpeg|png|webp|gif`，`Cache-Control: public, max-age=86400`；无封面 404。前端给 src 加 `?v=<updatedAt>` 规避一天缓存。
- **PUT**：multipart 单文件，字段名 `cover`，JPEG/PNG/WebP/GIF 且 ≤2 MiB；成功 **204**（不回图，前端 reload 详情拿新的 `updatedAt`）。非图片/超 2 MiB/缺字段 → 400 `invalid_param`；请求体超 3 MiB → 413 `payload_too_large`；书不存在 → 404。
- **DELETE**：**204**，幂等（本来没有也 204）；`has_cover` 置 0 并刷新 `updated_at`，列表回退成书名首字占位。书不存在 → 404。

### GET /api/books/{id}/chapters?from=1&limit=50
轻量章节列表（不带正文），用于目录虚拟滚动：
```json
{ "items": [{ "id": 10, "idx": 1, "title": "第1章 穿越", "volumeId": 1, "wordCount": 3210 }], "total": 1300 }
```

### PATCH /api/books/{id}/chapters/{chapterId}
body `{ "title": "…" }`：**204** 无响应体。标题去空白后不得为空、≤200 字（否则 400 `invalid_param`）；章节/书不存在 → 404。改标题会刷新书的 `updated_at`。

### POST /api/books/{id}/chapters/{chapterId}/split
body `{ "cutAfter": [0, 11] }`：`cutAfter` 是 0 起的**段落下标**，含义「在这一段之后切开」，必须严格递增、落在 `0 ~ 段落数-2`。
> 成功 200 `{ "chapters": 3 }`（拆完后的章数，最多 100 章）；只在段落边界切，正文一个字节不改：拆出来的章仍是同一压缩块里的不同区间，各章字数之和等于原章。新章标题为 `原标题（i/N）`，序号紧跟原章、其后各章整体后移；原章的章节 id 保留（阅读进度/书签仍指向它）；拆分点乱序/越界/落在空行上（会切出空章）→ 400 `invalid_param`（消息带原因），章节或书不存在 → 404，正文块损坏 → 503 `content_corrupted`。

### DELETE /api/books/{id}/chapters/{chapterId}
**204**；后面的章节序号自动前移（`idx` 连续），书级 `chapter_count`/`word_count` 与统计同步减少，正文块不再被任何章节引用时整块回收。重复删除 → 404。

### PUT /api/books/{id}/chapters/{chapterId}/content
body `{ "content": "正文……" }`：整章替换正文，**204** 无响应体（前端保存后重拉上面的 GET 拿新正文与新 `wordCount`）。
> 空正文（去空白后为空）/ 正文超 **1 MiB**（1,048,576 字节，UTF-8 编码后）→ 400 `invalid_param`，`message` 写明上限；章节或书不存在、章节不属于本书 → 404 `not_found`。章 id、`idx`、卷归属不变（阅读进度与书签按 chapter id 指过来）；`books.word_count` 与统计（`chapter_word_sum`/`text_bytes`/`text_bytes_raw`）同事务重算，旧块无人引用时整块回收。无版本号：后写覆盖先写（本地单用户），但一次写入要么全生效要么全回滚，不接受「正文换了、计数没动」。

### POST /api/books/{id}/chapters/batch-delete
body `{ "chapterIds": [10, 11] }`：**204** 无响应体。空数组 / 重复 id / 非正整数 → 400 `invalid_param`；任一章不属于本书（含已被删）→ 404 `not_found`，整批不动。
> 单事务：删完 `idx` 重排成 0 起连续、书级与统计计数同步、无人引用的正文块整块回收；不做「部分成功」（不留「前几章删了、后面没删」的半成品目录）。

### POST /api/books/{id}/chapters/merge
body `{ "chapterIds": [10, 11] }`：把**同卷且连续**的若干章并成一章，成功 200 `{ "chapters": 3 }`（合并后**全书章数**——不是这次并了几章，那个数恒为 1）。

> 少于 2 章 / 空数组 / 重复 id → 400 `invalid_param`；跨卷或不连续 → **409** `not_mergeable`（`message` 原样带原因：「跨卷（第 3 章在卷 1，第 4 章在卷 2）」/「不连续（第 1 章之后是第 3 章）」）；任一章不属于本书或不存在 → 404 `not_found`。保留**首章 id**（阅读进度与书签按它指过来）、首章标题与所在卷：正文按目录顺序拼接（不跟请求里的 id 顺序走），相邻两章之间插入**恰好一个换行**（不留分隔读者会把两句连读），字数取各章之和，章节数净减 `n-1`。

### POST /api/books/{id}/chapters/renumber
body `{ "scope": "book" }` 或 `{ "scope": "volume", "volumeId": 7 }`：200 `{ "renamed": 3 }`（范围内被重编号的章数）。

> 标题重写成「第 N 章 + 原标题去掉已有章号后的部分」，N 恒为**全书序号** `idx+1`（单卷重编号也不从 1 重来）；只改 `title`，不动正文与 `idx`。`scope` 非法 / 缺 `volumeId` 或非正 → 400 `invalid_param`（`scope=book` 时忽略 `volumeId`）；卷不属于本书或已删 → 404 `not_found`；标题本来就对的章不写库，`renamed` 是范围章数（不是实际更新行数）。

### 卷管理 /api/books/{id}/volumes
| 端点 | 成功 | 口径 |
|---|---|---|
| `POST .../volumes` body `{"title":"卷三"}` | 201 `{"volume":{"id":9,"idx":3,"title":"卷三","chapters":[]}}` | `idx` = 当前卷数；卷名去首尾空白后同书唯一：空名 400 `invalid_param`、重名 409 `volume_name_taken`（「这本书已经有同名卷了」） |
| `PATCH .../volumes/{vid}` body `{"title":"卷一"}` | 200 同上 | 重名/空名同上；卷不属于本书 → 404 `not_found` |
| `DELETE .../volumes/{vid}` | 204 | 卷内章节落未分卷（`volumeId=0`，`idx`/正文/`wordCount` 不变）；剩余卷 `idx` 重排 0..n-1、`volume_count` 重算 |
| `PUT .../volumes/order` body `{"ids":[3,1,2]}` | 200 `{}` | `ids` 必须恰好是本书全部卷 id 的一个排列：缺 id / 多 id / 重复 / 非本书卷 → 400 `invalid_param` |
| `POST .../chapters/move` body `{"chapterIds":[11,12],"volumeId":3}` | 200 `{"moved":2}` | `volumeId=0` = 移到未分卷；只改 `chapters.volume_id`，不改全书序号/正文/字数；卷或章节不属于本书 → 404 `not_found` |

### GET /api/books/{id}/bookmarks
全书书签，按 `createdAt` 倒序（同秒按 `id` 倒序；章节被删时 `chapterTitle` 为空串）。
```json
{ "bookmarks": [{ "id": 1, "chapterId": 9, "chapterIdx": 4, "chapterTitle": "第4章 风起", "paraIndex": 12,
  "label": "灯塔的光扫过海面", "createdAt": "2026-10-24T21:30:00.000Z" }] }
```
书不存在 → 404；没有书签 → 200 + 空数组。

### POST /api/books/{id}/bookmarks
body `{ "chapterId": 9, "paraIndex": 12, "label": "…" }`（`chapterId`/`paraIndex` 必填，`label` 可空）。

- 新建 **201**；同一 `(chapterId, paraIndex)` 已存在 → **200** 且只更新 `label`（幂等）；`chapterId` 不属于该书 / `paraIndex < 0` / `label` 超 200 字 → 400，书不存在 → 404；
- `paraIndex` 是正文按 `\n` 切段后的段落下标（与阅读器、拆章接口同一口径）。

### DELETE /api/bookmarks/{id}
**204**；不存在（含删两次）→ 404。删书时书签级联清空。

## 作者

### GET /api/authors
参数：`q`、`page`、`size`、`sort`=`books|name`。
```json
{ "items": [{ "id": 12, "name": "示例作者", "bookCount": 3, "wordCount": 5600000 }], "total": 412, "page": 1, "size": 20 }
```

### GET /api/authors/{id}
```json
{ "id": 12, "name": "示例作者", "bookCount": 3, "books": [ /* BookSummary，同 /api/books items */ ] }
```
