# 书籍管理 P0：删除 · 连载状态 · 最新章节 · 导入取消 · 多卷归并 · 自定义切章（2026-10-25）

> 来源：与正规小说站的差距复核（main@f2c611a）。本批只做用户点名 + 明显影响可用性的 P0，P1 另批。
> 边界：不做合并、不做 URL 书源、不做繁简转换、不做导出、不做划线批注、不做追更通知。

## 1. 用户可见行为

- 详情页可**删除**一本书（二次确认），书架/搜索/作者页立即消失，阅读进度一并清除。
- 详情页可标记**连载中 / 已完结 / 未知**，并显示「最后更新 · 最新章节」。
- 导入进行中可**取消**，任务立刻停止并标记 `canceled`，已入库的书保留。
- 目录导入时，同名多卷文件（`书名（第1卷）.txt` / `书名_卷2.txt` / `书名 03.txt`）**合并成一本书**，章节按卷序拼接。
- 可添加**自定义章节规则**（正则），导入时优先于内置规则切章；支持试切预览。

## 2. 契约

### 2.1 迁移 0010（只加列/加表）

```sql
ALTER TABLE books ADD COLUMN serial_status TEXT NOT NULL DEFAULT '';
CREATE TABLE settings(key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);
```

`serial_status` 取值：`''`（未知）/ `ongoing` / `completed`。存量书回填为空，不猜。

### 2.2 API

| 端点 | 契约 |
|---|---|
| `DELETE /api/books/{id}` | 204；不存在 404。级联删 chapters / chapter_blocks / volumes / book_covers / books_authors / reading_progress，刷新作者统计 |
| `PATCH /api/books/{id}` | 新增 `serialStatus`（`''`/`ongoing`/`completed`，其它 400 `invalid_param`） |
| `GET /api/books/{id}` | 新增 `serialStatus`、`lastChapterTitle`、`lastChapterIdx`（取 idx 最大的章节） |
| `POST /api/import/jobs/{id}/cancel` | 202 `{"status":"canceling"}`；任务不存在 404；已结束 409 `conflict` |
| `GET /api/import/jobs/{id}` | 任务状态新增 `canceled` |
| `GET /api/settings/chapter-rules` | `{"rules":[{"name":"…","pattern":"…"}]}` |
| `PUT /api/settings/chapter-rules` | 全量替换；正则非法 400 `invalid_pattern`；条数 >20 或单条 >200 字 400 |

### 2.3 parser / importer

- `importer.Cancel(jobID int64) bool`：取消运行中的任务，返回是否命中；`Result` 增 `Canceled bool`。
- `parser.Rule{Name string; Pattern *regexp.Regexp}`；`parser.ParseBytesWithRules(data, rules)`；规则按顺序尝试，命中即作为章节边界，**优先于内置策略**；一条规则一章都没切出时退回内置并对该规则记 warning。
- 多卷归并：目录扫描后按归一化书名分组；组内 ≥2 个文件**且每个都带卷号/序号后缀**时合并为一本；书名取去除后缀后的共同部分；卷号顺序决定章节拼接顺序；不满足条件不动（保持一文件一本书）。
- 卷号后缀识别：`（第N卷）` `（卷N）` `_卷N` `-卷N` ` 卷N` ` 第N部` ` N`（纯数字，需与同组其它文件同名）；N ≤ 999。

### 2.4 前端

- 详情页：删除按钮（二次确认需输入书名）、连载状态下拉（未知/连载中/已完结）、元信息行显示「最后更新 · 最新章节」。
- 导入页：运行中任务显示「取消」；新增「章节识别规则」区块（列表 + 名称/正则 + 保存 + 试切预览，预览用粘贴文本）。
- 删除成功后回书架并 toast「已删除《书名》」。

## 3. 工程化决定

| 能力 | 决定 | 程度 | 依据 |
|---|---|---|---|
| 删除 | 做 | 单事务级联 + 二次确认 | 错导入无路可退，source_path UNIQUE 还会顶住重导 |
| 连载状态 | 做 | 独立列，不覆写解析状态 status | status 是解析状态，语义不同 |
| 取消 | 做 | context 取消 + 状态置 canceled | 长导入只能杀进程会留 running 任务 |
| 多卷归并 | 做 | 严格门控（同名 + 全部带后缀 + 卷号连续无需） | 误归并比漏归并更难救 |
| 自定义正则 | 做 | 全局规则，最多 20 条，非法即拒 | 冷门书救急，不做书籍级覆盖 |
| 合并 / URL / 导出 | 省 | — | 用删除+重导替代；本地库定位 |

## 4. 验收

- `make gate` 9 项全绿，覆盖率不低于基线。
- 删除：`DELETE /api/books/{id}` → 204，列表总数 -1，再删同 id → 404，进度接口不再返回该书。
- 状态：PATCH `serialStatus=ongoing` → GET 详情返回 ongoing；传 `abc` → 400。
- 最新章节：GET 详情 `lastChapterIdx` 等于最大章序，`lastChapterTitle` 非空。
- 取消：导入 500 个文件途中取消 → 任务 `canceled`，已入库数量 < 500 且书可读。
- 多卷归并：3 个分卷文件 → 1 本书、章节数 = 三份之和、卷数 3；无后缀的同名文件不归并。
- 自定义规则：用 `^=== 第\\d+ 节 ===$` 切开一本内置策略切不开的书；单条规则零命中时退回内置并记 warning。
