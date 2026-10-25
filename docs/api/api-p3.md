# P3 增量端点（阅读体验 / 发现 / 管理补全）

> 只列本轮增量，未列端点不变。字段名即 JSON 名（camelCase），时间为 UTC ISO-8601，
> 错误码沿用 `invalid_param` / `invalid_id` / `not_found` / `conflict` 家族；范围取舍见 [p3-scope](../specs/2026-10-25-p3-scope.md)。

## 书籍

- `PATCH /api/books/{id}` 增 `rating`(0–5，0 = 未评)、`publisher`、`pubDate`(YYYY-MM-DD，空串清空)、`isbn`、`language`；rating 越界或 pubDate 格式非法 → 400 `invalid_param`。
- `POST /api/books/batch` body `{ids:[≤200], patch:{同 PATCH 字段}}` → 200 `{updated:[id], failed:[{id,message}]}`；单本失败只进 failed、不整体回滚；ids 为空/超 200/非正 → 400。
- `GET /api/books` 增 `series`、`rating`(1–5)、`collection`(书单 id) 筛选，`sort` 增 `rating`（评分倒序，0 = 未评排最后）。
- `GET /api/books/random?authorId=&tag=&collection=` → 200 BookBrief；无符合条件的书 → 404 `not_found`。
- `POST /api/books/{id}/chapters` body `{title, content, afterChapterId?}`（省略 = 追加末尾）→ **201** ChapterBrief；正文去空白后为空或 >1 MiB → 400；afterChapterId 不属于该书 → 400；书不存在 → 404。
- `POST /api/books/{id}/ack-new-chapters` → **204**（`newChapters` 清零）；书不存在 → 404。

## 系列

- `GET /api/series?q=&page=&size=` → 200 `{items:[{name,bookCount,wordCount,lastUpdatedAt,coverBookId}],total}`；只统计 series 非空的书，按最近更新倒序。
- `GET /api/series/{urlencoded-name}` → 200 `{name,books:[BookBrief]}`；系列无书 → 404 `not_found`；名字为空/含路径分隔符 → 400。

## 书单（扁平收藏夹）

- `GET /api/collections` → 200 `{items:[{id,name,bookCount}]}`。
- `POST /api/collections` body `{name}` → **201** Collection；重名 → 409 `conflict`；空名或超 60 字 → 400。
- `PATCH /api/collections/{id}` body `{name}` → 200 Collection；重名 409；书单不存在 404。
- `DELETE /api/collections/{id}` → **204**（只删收藏夹关系，不动书）；不存在 404。
- `PUT /api/collections/{id}/books` body `{bookIds:[...]}` 全量替换 → **204**（`[]` = 清空）；缺 `bookIds` 或书不存在 → 400；书单不存在 → 404。

## 标签

- `GET /api/tags` → 200 `{items:[{id,name,bookCount}]}`（按书籍数倒序）。
- `POST /api/tags` body `{name}` → **201**（新建）或 **200**（同名返回已有，幂等）；空名或超 64 字 → 400。
- `PATCH /api/tags/{id}` body `{name}` → 200 TagSummary；重名 → 409 `conflict`；标签不存在 → 404。
- `DELETE /api/tags/{id}` → **204**（全库解绑，书不动）；不存在 → 404。

## 阅读时长与历史

- `PUT /api/reading/progress` 增 `paraIndex`(≥0，缺省 0)；负值 → 400。
- `POST /api/reading/sessions` body `{bookId,chapterId,seconds:1–86400,startedAt?}` → **204**；章节不属于该书 → 404；seconds 越界 → 400。
- `GET /api/reading/history?page=&size=` → 200 `{items:[{bookId,bookTitle,chapterId,chapterTitle,lastReadAt,totalSeconds,days}],total}`（按书聚合，最近阅读在前）。
- `GET /api/stats/reading?days=1–365&tz=分钟偏移` → 200 `{totalSeconds,todaySeconds,weekSeconds,streak,days:[{date,seconds}]}`；days 默认 90、tz 默认 0（东八区 480），越界 400。
