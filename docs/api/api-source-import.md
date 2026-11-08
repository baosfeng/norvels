# 导入 Legado 规则（通用）

> 把 Legado（开源阅读）的规则 JSON 导入 norvels：**按字段自动识别条目类型**
> （书源 / 替换净化 / TXT 章节规则 / 未知）。书源适配成来源规则包（`kind=online`），
> 净化与 TXT 规则本期只识别、不落库。只做导入 + 结构校验 + 降级标记，不抓取；
> 设计见 [ADR-005](../adr/0005-legado-import.md)，规则包契约见 [api-source](api-source.md)。

## POST /api/import/source-profiles/import

body：

```json
{ "raw": "<粘贴的 Legado 书源 JSON 文本>", "apply": false }
```

- `raw` 必填：接受三种打包形态——单个书源对象、JSON 数组、`{"bookSources":[…]}`（兼容 `{"sources":[…]}`）；
- `apply` 缺省 false：**只预览不落库**；true 才把可入库的规则包合并落库。

预览响应（`apply=false`）：

```json
{
  "results": [{
    "index": 0,
    "type": "book_source",
    "id": "src-1a2b3c4d",
    "name": "示例书源",
    "status": "ok",
    "degradedFields": [],
    "unsupportedFields": [],
    "reason": ""
  }]
}
```

应用响应（`apply=true`）额外带 `applied` / `overwritten` / `skipped`：

```json
{
  "applied": 1,
  "overwritten": 0,
  "skipped": [{ "index": 1, "id": "src-…", "name": "示例音频源", "type": "book_source",
                "status": "unsupported_type", "reason": "bookSourceType=1：只支持文本书源…" }],
  "results": [ "…" ]
}
```

`skipped` 是 apply 时被跳过的**书源**（音频/图片/文件源、缺 url、含 `@js` 硬字段）；
净化 / TXT 规则与未知条目不是书源，只在 `results` 里出现。

## 条目类型（type）

| type | 判定字段 | 本期行为 |
|---|---|---|
| `book_source` | 含 `bookSourceUrl`（或书源特征字段，缺 url 报 invalid） | 适配成规则包；`ok`/`partial` 可落库，其余跳过 |
| `replace_rule` | 含 `pattern` + `replacement` 且无书源字段 | 只识别，`status=recognized`，不落库 |
| `txt_toc_rule` | 含 `rule` 或单独的 `pattern` | 只识别，`status=recognized`，不落库 |
| `unknown` | 以上都不匹配 | `status=unknown_type`，带原因，不静默丢 |

## status 取值

| status | 含义 | 可入库 |
|---|---|---|
| `ok` | 全部规则可执行 | 是 |
| `partial` | 部分字段含 `@js`、规则无法编译或简写无法转换，已降级 | 是（带标记） |
| `unusable` | 硬字段（`bookList`/`chapterList`/`content`/`searchUrl`/`exploreUrl`）含 `@js` | 否（apply 跳过，进 `skipped`） |
| `invalid` | 缺 `bookSourceUrl` / 非 http(s) / 对象不合法 | 否 |
| `unsupported_type` | 音频/图片/文件源（`bookSourceType` ≠ 0） | 否 |
| `recognized` | 净化 / TXT 规则，已识别但本期不落库 | — |
| `unknown_type` | 无法判定条目类型 | — |

导入时把 Legado 简写（`tag.`/`class.`/`id.` 前缀、`@` 链、`.N`/`!N` 索引）翻译成标准 CSS 再落库；`@css:`/`@xpath:`/`@json:` 前缀、`@text`/`@ownText`/`@html`/属性后缀与 `##正则##替换` 原样保留。转换不了的规则保持原文、字段进 `degradedFields`，状态降为 `partial`（整源仍可入库）。

`degradedFields` 是「降级」的字段（形如 `ruleSearch.bookList`、`searchUrl`）：因 `@js` 不可执行，或 Legado 简写无法安全转换（此时规则保留原文）；
`unsupportedFields` 是「保存但不会生效」的字段（`loginUrl`/`cookie`/`webJs`）；`header` 已生效（随请求发送，CR/LF 值与 Host/Content-Length 等键在校验时拒绝）。

## 错误与合并语义

- `raw` 为空、JSON 解析失败、没有可导入条目 → 400 `invalid_param`；
- `apply=true` **跳过不可用书源**（`unsupported_type` / `invalid` / `unusable`），只落
  `ok` / `partial`：分享站整包导入必然混着漫画/音频源，一条坏的不该拖垮整批；
- 只有**一条可用书源都没有**时才 400，消息为「N 条输入里没有可导入的书源（ok/partial），
  原因见 results」；
- `apply=true` 只落书源条目（`book_source`）；净化 / TXT（`recognized`）与未知条目
  （`unknown_type`）不阻断、不落库，结果里原样回显；
- 合并：同 id 覆盖、其余（含 local 规则包）保留；合并结果走与 PUT 相同的校验
  （≤20 份、id 唯一、正则合法），失败不落库；
- `id` 由 `bookSourceUrl` 派生（`src-` + sha256 前 8 位），同一 url 重复导入即覆盖，幂等。

## 示例（全虚构）

```json
{
  "bookSourceUrl": "https://example.invalid/books",
  "bookSourceName": "示例书源",
  "bookSourceType": 0,
  "searchUrl": "https://example.invalid/search?q={{key}}",
  "ruleSearch": { "bookList": ".item", "name": ".title" },
  "ruleToc": { "chapterList": ".chapter" },
  "ruleContent": { "content": ".content" },
  "header": { "User-Agent": "示例" }
}
```

→ `results[0] = { "id": "src-…", "status": "ok", "unsupportedFields": [] }`（`header` 会随请求发送）。
## 支持的导入入口形态

`internal/ruleimport` 把用户给的「任何分享站入口」统一取回成一段 JSON（上限 1 MiB），
调用方只负责按 `Hint` 分派类型：

| 输入形态 | 例子（全虚构） | `Via` | `Hint` |
|---|---|---|---|
| JSON 文本 | `[{"bookSourceUrl":"https://example.invalid"}]` | `text` | 空 |
| http(s) 直链 | `https://example.invalid/sources/x.json` | `url` | 空 |
| Legado 导入链接 | `legado://import/bookSource?src=https://example.invalid/sources/x.json` | `legado` | `bookSource` |
| 旧版阅读链接 | `yuedu://booksource/importonline?src=https://example.invalid/sources/x.json` | `yuedu` | `bookSource` |

- 链接里的 `src` 必须是 http(s)，其它协议（`file:`/`javascript:`/`ftp:`）一律 `unsupported`；
- 取回经 `internal/fetch`，SSRF 校验（私网/回环/云元数据/重定向逐跳）沿用同一套；
- 响应是 HTML 时，从 `<pre>` / `<code>` / `<script type="application/ld+json">` 里提取 JSON，
  提不到返回 `not_json`；超过 1 MiB 返回 `too_large`。