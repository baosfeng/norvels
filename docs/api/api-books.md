# 书籍与作者

## 书籍

### GET /api/books
查询参数：`q`（书名/作者模糊）、`authorId`、`tag`、`readStatus` = `want|reading|done`（留空 = 全部，非法值 400）、`sort` = `updated|title|chapters|words|imported`（默认 updated）、`page`、`size`。

```json
{
  "items": [{
    "id": 1, "title": "山居杂记", "authorId": 12, "authorName": "示例作者",
    "chapterCount": 1300, "wordCount": 2800000, "volumeCount": 7,
    "intro": "……", "status": "parsed", "serialStatus": "ongoing", "readStatus": "reading",
    "importedAt": "2026-10-24T21:30:00+08:00"
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
  "status": "parsed", "serialStatus": "ongoing", "readStatus": "reading",
  "bookmarkCount": 3,
  "lastChapterIdx": 1299, "lastChapterTitle": "第1299章 风起",
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

> **书籍身份的增量字段**（0008 起，只加不改）：`language`、`publisher`、`isbn`、`pubDate`、`series`、`seriesIndex`、`hasCover`、`format`（`txt`/`epub`），以及详情独有 `tags`（字符串数组，空数组表示无标签）。这些字段只有 EPUB 等带元数据的来源才会填，txt 路径留空/零值，前端按零值降级。

> **0010 起详情再增三项**：`serialStatus`（`""` 未知 / `ongoing` 连载中 / `completed` 已完结）与 `lastChapterIdx`、`lastChapterTitle`（`idx` 最大的那一章，无章节时为 `0` 与 `""`）。`serialStatus` 与 `status` 是两个语义（见下方 PATCH 一节）。

> **0011 起增阅读状态与书签数**（列表与详情都有 `readStatus`，详情独有 `bookmarkCount`）：迁移 `0011`，存量书 `readStatus` 为 `""`，不回填。三套状态互不覆写：`status`（解析）/ `serialStatus`（连载）/ `readStatus`（读者阅读）。

### PATCH /api/books/{id}
body 为任意子集：`title`、`author`、`authors`（字符串数组，**全量替换**，最多 10 个；空数组 = 清空作者）、`intro`、`tags`（字符串数组，**全量替换**）、`series`、`seriesIndex`、`serialStatus`、`readStatus`。

- 成功 200，返回更新后的 BookDetail（含 tags/hasCover/authors/volumes）；
- 清空语义：`intro`/`series` 传 `""` 即清空，`seriesIndex` 传 `0` 即清空；`authors` 传空数组/只有空白名 → **清空作者**（成功 200，详情 `authors` 返回 `[]`、`authorName` 为空串，`books.author_id` 置 0，旧作者的作品数/字数随之归零）；
- `author` 与 `authors` 同时给出时 `authors` 优先；只给 `author` 等价于 `authors=[author]`；
- 空 body、空对象、只含未知字段、title 为空白 → 400 `invalid_param`；id 非数字 → 400 `invalid_id`；书不存在 → 404；
- 改作者会同步重建作者聚合（新旧作者都刷新）；改 `tags` 会清理不再被引用的孤儿标签；
- `serialStatus` 只接受 `""`（未知）/ `ongoing` / `completed`，其它值 → 400 `invalid_param`；
  它只改连载状态，不动解析状态 `status`，覆盖重导也不会被重置；
- `readStatus` 只接受 `""`（未标记）/ `want` / `reading` / `done`，其它值 → 400 `invalid_param`；
  与 `serialStatus` 可同批出现，两者互不覆写。

### DELETE /api/books/{id}
单事务级联删除：`chapters` / `chapter_blocks` / `volumes` / `book_covers` / `books_authors` / `reading_progress` / `bookmarks` / `book_tags`，并重算作者聚合（作者行保留，作品数与字数归零）。

- 成功 **204**，无响应体；
- 书不存在 → 404 `not_found`（再删同一个 id 也是 404）；
- 删掉的书立即从书架/搜索/作者页消失，「最近在读」不再返回它；同一 `source_path` 可以重新导入。

### GET /api/books/{id}/cover
- 有封面：200，`Content-Type` 为 `image/jpeg|png|webp|gif`，`Cache-Control: public, max-age=86400`；
- 无封面：404 `not_found`。
- 覆盖导入换封面后浏览器可能缓存旧图 1 天，前端给 img src 加 `?v=<updatedAt>` 规避。

### GET /api/books/{id}/chapters?from=1&limit=50
轻量章节列表（不带正文），用于目录虚拟滚动：
```json
{ "items": [{ "id": 10, "idx": 1, "title": "第1章 穿越", "volumeId": 1, "wordCount": 3210 }], "total": 1300 }
```

### GET /api/books/{id}/bookmarks
全书书签，按 `createdAt` 倒序（同秒按 `id` 倒序；章节被删时 `chapterTitle` 为空串）。
```json
{ "bookmarks": [{ "id": 1, "chapterId": 9, "chapterIdx": 4, "chapterTitle": "第4章 风起",
  "paraIndex": 12, "label": "灯塔的光扫过海面", "createdAt": "2026-10-24T21:30:00.000Z" }] }
```
书不存在 → 404 `not_found`；没有书签 → 200 + 空数组。

### POST /api/books/{id}/bookmarks
body `{ "chapterId": 9, "paraIndex": 12, "label": "…" }`（`chapterId`/`paraIndex` 必填，`label` 可空，默认「章名 · 第 n 段」）。

- 新建 **201**；同一 `(chapterId, paraIndex)` 已存在 → **200** 且只更新 `label`（幂等，总数仍为 1）；
- `chapterId` 不属于该书 / `paraIndex < 0` / `label` 超 200 字 / 字段缺失或类型不符 → 400 `invalid_param`；
  书不存在 → 404 `not_found`；
- `paraIndex` 是正文按 `\n` 切段后的段落下标（与前端阅读器一致），比字符偏移更耐正文重排。

### DELETE /api/bookmarks/{id}
**204** 无响应体；不存在（含删两次）→ 404 `not_found`。删书时书签随 `DELETE /api/books/{id}` 级联清空。

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
