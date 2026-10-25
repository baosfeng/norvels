# norvels REST API 契约 v1

Base：`/api`。全部返回 JSON（UTF-8），错误统一：

```json
{ "error": { "code": "not_found", "message": "book not found" } }
```

状态码：200 正常 / 202 已接受（异步任务）/ 204 无内容 / 400 参数错误 / 404 未找到 / 409 冲突（任务进行中或已结束）/ 500 内部错误。

分页参数统一：`page`（从 1 开始，默认 1）、`size`（默认 20，最大 200）。分页响应统一：

```json
{ "items": [], "total": 123, "page": 1, "size": 20 }
```

## 文档索引

| 文件 | 内容 |
|---|---|
| [api-books.md](api-books.md) | 书籍（列表/详情/章节/封面/书签）与作者 |
| [api-import.md](api-import.md) | 导入任务（dry-run 预检/上传/重试/取消）与设置（章节规则/标签规则） |
| [api-dashboard.md](api-dashboard.md) | 元信息（health/stats）、驾驶舱端点、其它端点 |

原 `docs/api.md` 章节号对照：§1/§7/§9 → [api-dashboard.md](api-dashboard.md)；§2/§3 → [api-books.md](api-books.md)；§4/§8/§10 → 本文件；§5/§6 → [api-import.md](api-import.md)。

## 搜索

### GET /api/search
参数：`q`（必填，≥1 字符）、`type`=`chapter|book|all`（默认 all）、`page`、`size`。

```json
{
  "q": "临川市",
  "books": { "items": [ /* BookSummary */ ], "total": 2 },
  "chapters": {
    "items": [{
      "bookId": 1, "bookTitle": "风流名将", "chapterId": 88, "chapterTitle": "第88章 夜宴",
      "snippet": "", "score": 12.3
    }],
    "total": 57, "page": 1, "size": 20
  }
}
```
> `snippet` 字段保留但**恒为空字符串**：正文检索已下线，后端不再计算命中片段（见 [ADR-0002](../adr/0002-drop-fulltext-index.md)）。

**检索约束（实现现状，前端必须处理）**

- 检索范围是**书名 / 作者名 / 章节标题**，三类都走 `LIKE`，**不检索正文**。
  - `q` trim 后 **少于 2 个字符** → `400` `{"error":{"code":"query_too_short","message":"关键词至少 2 个字"}}`；
  - 不需要分词：中文两字词（`秋蝉`）直接按子串匹配。
- 响应可能带可选字段 `notice`（string），说明降级原因；前端有则提示、无则忽略。

## 错误码一览

| code | HTTP | 含义 |
|---|---|---|
| `invalid_param` | 400 | 参数缺失/格式错（page/size/id/dir/percent 等） |
| `unauthorized` | 401 | 启用鉴权后未提供或提供了错误的令牌 |
| `query_too_short` | 400 | 搜索关键词少于 2 字符 |
| `unsupported_format` | 400 | 上传/导入的文件格式不在 TXT/EPUB/ZIP 白名单内 |
| `invalid_zip` `zip_too_large` `empty_zip` | 202 | zip 包被打回（逐条进 `rejected`） |
| `not_appendable` | 409 | 追更时新章节不是已有序列的追加（拒绝覆盖，确要整本重导用 force） |
| `payload_too_large` | 413 | 上传单文件或单批总量超过上限 |
| `too_many_files` | 400 | 一次上传的文件数超过上限 |
| `encrypted_epub` | 422 | EPUB 正文条目被加密（DRM），拒绝入库 |
| `not_found` | 404 | 书籍/章节/作者/任务不存在，或未知 `/api/*` 路径 |
| `method_not_allowed` | 405 | 路径存在但方法不对（带 `Allow` 头） |
| `invalid_pattern` | 400 | 自定义章节规则的正则编译失败 |
| `conflict` | 409 | 已有导入任务在运行，或要取消/重试的任务已结束、无取消入口 |
| `timeout` | 503 | dry-run 预检超过 60 秒（缩小目录范围后重试） |
| `unavailable` | 503 | 数据库不可用（`/api/health`） |
| `internal` | 500 | 未预期错误（已脱敏） |

## 前端约定

- 阅读进度：**服务端为准**（见 [api-dashboard.md §9](api-dashboard.md#get-apireadingprogress) 的 `GET /api/reading/progress`），`localStorage` 降级为离线缓存。
- 章节正文为纯文本，前端按段渲染（`\n` 分段），不做富文本。
- 搜索无高亮：`snippet` 恒为空，前端不要按标签渲染。

## 鉴权（可选）

默认开放模式（单人本地/内网使用）。设置环境变量 `NORVELS_AUTH_TOKEN` 后：

- 所有 `/api/*` 与页面要求 `Authorization: Bearer <token>` 或 Cookie
- 免鉴权路径：`/api/health`（容器健康检查）、`GET|POST /login`、`POST /logout`
- 未通过时：`/api/*` 返回 `401 {"error":{"code":"unauthorized",…}}` 并带 `WWW-Authenticate`；
  页面请求 303 跳到 `/login?next=<原路径>`
- `GET /login` 是服务端渲染的内联 HTML 表单；`POST /login` 用 `token` 字段提交，
  成功后下发 `norvels_auth` Cookie（HttpOnly + SameSite=Lax + Path=/，30 天）并跳回 `next`
- 令牌用 `crypto/subtle.ConstantTimeCompare` 恒定时间比较；`next` 只接受站内相对路径（防开放重定向）
- 未设置时启动日志打印 `WARN: 未启用鉴权，请勿暴露到公网`

**公开部署（公网 VPS）必须先设 `NORVELS_AUTH_TOKEN`**，详见 [ADR-0001](../adr/0001-public-release.md)。
