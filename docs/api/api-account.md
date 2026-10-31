# 账号与我的书架（0016）

> 只注册 + 输账号登录，**没有密码**。书库（books/authors/chapters/tags）共享；
> 个人记录（阅读进度 / 阅读会话 / 书签 / 书单 / 我的书架）按账号隔离。
> 无密码的前提是「内网私有部署」；暴露公网请先换成真正的身份系统（见 [ADR-0001](../adr/0001-public-release.md)）。

## 鉴权与账号的分工

| 层 | 回答的问题 | 凭证 |
|---|---|---|
| 共享口令 `NORVELS_AUTH_TOKEN` | 能不能进这个库 | `Authorization: Bearer <token>` 或 `norvels_auth` Cookie（**语义未变**） |
| 账号会话 | 进来之后你是谁 | `norvels_session` Cookie（HttpOnly + SameSite=Lax + 30 天） |

- **未配置口令（开放模式）**：没有登录这回事，所有请求按默认账号 `reader`(id=1) 读写个人数据，单用户用法照旧。
- **配置了口令**：Bearer / 旧 Cookie 仍可访问全部 API（管理侧、部署脚本、健康检查不受影响）；
  另可用会话 Cookie 进门。两者并存，互不替代。
- 会话无效（过期 / 已撤销）时**回落默认账号**而不是报错：口令仍有效的地方必须继续能用。
- 会话 token 是 `crypto/rand` 32 字节 hex；库里只存它的 SHA-256，明文只在 Cookie 里。
  登出即删行（立即撤销），过期行在读会话时被过滤并在建会话时清理。

## 端点

### POST /api/auth/register

body `{"name":"账号名"}`：去首尾空白后 1–32 字（按字符计），**注册即登录**。

- `201` + `{id,name,createdAt,lastSeenAt}`，同时下发 `norvels_session`；
- `400` `invalid_param`：空名 / 纯空白 / 超过 32 字 / body 非 JSON 对象；
- `409` `conflict`：「这个账号名已经有人用了」——唯一性按 **TRIM + 大小写折叠**比较
  （`Alice` / `alice` / ` Alice ` 是同一个账号；存储的 `name` 仍是用户输入原样）。

### POST /api/auth/login

body `{"name":"账号名"}`：存在则刷新 `lastSeenAt` 并下发新 Cookie。

- `200` + 账号对象；
- `404` `account_not_found`：「账号不存在，请先注册」——这里没有密码可输错，唯一的失败原因就是没建号；
- `400` `invalid_param`：同上。

### POST /api/auth/logout

撤销当前会话并清 Cookie，`204`（幂等：没登录 / Cookie 已失效同样 204）。

### GET /api/auth/me

`200` + 当前账号对象；开放模式返回默认账号 `reader`。

### GET /api/auth/users

登录页「选已有账号 or 新建账号」用的账号列表，`200`：

```json
{"items":[{"id":2,"name":"读者甲","createdAt":"2026-10-31T05:26:24.041Z","lastSeenAt":null}],"total":2}
```

- 按「最近活跃」倒序：`lastSeenAt` 倒序，从未登录（`null`）的排最后，时间相同按 `id` 倒序；
- 最多 **50 条**；`total` 是账号总数，可能大于 `items` 长度；
- `lastSeenAt` 为 `null` = 建号后从未登录（注册不刷新活跃时间，登录一次才有值）；
- 时间是 RFC3339 UTC 字符串，与账号对象同口径。

**这个端点不鉴权**：登录页本身就在鉴权之外，要求凭证才能拿列表等于要求先登录才能登录。
代价是门外的人能看到「这台库上有哪些账号名」——本产品面向内网私有部署、账号无密码、
账号名不是秘密，该取舍已接受；暴露公网请先换成真正的身份系统（[ADR-0001](../adr/0001-public-release.md)）。

### 账号对象

```json
{ "id": 2, "name": "读者甲", "createdAt": "2026-10-29T15:51:42.697Z", "lastSeenAt": "" }
```

`lastSeenAt` 为空串 = 建号后从未登录。时间为 UTC ISO-8601。

## 我的书架

一本书只属于一个书架（用户主动加入的书）；全部书仍可浏览。与「书单」不同：
书单可建多个、可命名，一本可属多个书单。加入 / 移出都幂等。

### GET /api/shelf?page=&size=

`200` `{"items":[BookBrief + addedAt],"total":N}`，按加入时间倒序（同秒按 book_id 倒序）。

### GET /api/shelf/ids

`200` `{"bookIds":[1,2,3]}`：当前账号书架的全部 book_id（**不分页**）。
前端用它给书库列表逐行打「已加入」标记，翻页时不必再问后端。

### PUT /api/shelf/{bookId}

加入书架，`204`。重复加入不报错、不刷新 `addedAt`。书不存在 `404` `not_found`；id 非正 / 非数字 `400` `invalid_id`。

### DELETE /api/shelf/{bookId}

移出书架，`204`。本来不在架上也算成功。

## 个人数据归属

下列端点读写的是**当前账号**的数据，同一本书不同账号各有一份：

| 端点 | 隔离方式 |
|---|---|
| `GET|PUT /api/reading/progress` | 主键 `(user_id, book_id)` |
| `POST /api/reading/sessions`、`GET /api/reading/history`、`GET /api/stats/reading` | 会话行带 `user_id` |
| `GET|POST /api/books/{id}/bookmarks`、`DELETE /api/bookmarks/{id}` | 唯一键 `(user_id, book_id, chapter_id, para_index)` |
| `GET|POST|PATCH|DELETE /api/collections…` | 书单名在同一账号内唯一 |

别人的书签 / 书单按「不存在」处理（`404`），不返回 `403`：id 是自增的，区分二者只会泄露「这个 id 存在」。
