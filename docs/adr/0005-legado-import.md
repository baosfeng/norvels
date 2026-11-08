# ADR-005：Legado 书源导入适配器

> 决策日期 2026-10-31。触发：用户要能直接导入 Legado（开源阅读）书源 JSON，而不是手写
> norvels 的 Profile。在线抓取的安全边界与执行器设计见 [ADR-004](0004-online-book-source.md)。

## 1. 问题

Legado 书源是「一份 JSON 描述一个站点」：唯一键 `bookSourceUrl`、类型（文本/音频/图片/文件）、
搜索/详情/目录/正文四段规则、登录态与请求头。它和 norvels 的 Profile 字段不同，且混着
三类 norvels **本期不能执行**的东西：

| 混入项 | 若直接存会发生什么 |
|---|---|
| 音频/图片/文件源 | 规则块语义完全不同，抓取必失败，用户看不出为什么 |
| `@js:` 规则 | 没有 JS 引擎，运行期静默返回空串 |
| `loginUrl` / `cookie` / `webJs` | 被存下来但运行期忽略，用户以为登录态已生效（`header` 已支持静态键值） |

直接存等于把「不支持」变成「看起来支持」——这是导入功能最大的坑。

## 2. 决策

1. **适配器独立成包** `internal/booksource`（依赖 `source` + `ruleexec`，不依赖抓取与存储）：
   Legado JSON → `source.Profile`（`kind=online`）。
2. **先预览后应用**：`POST /api/import/source-profiles/import`，`apply` 缺省 false 只返回逐条结果，
   true 才落库；与前端「粘贴 → 看结果 → 应用」的流程一致。
3. **类型过滤**：只接受 `bookSourceType=0`（文本）；1/2/3 明确拒绝并给原因，不静默吞掉。
4. **id 派生**：`src-` + `sha256(bookSourceUrl)` 前 8 位（`[a-z0-9-]`）。同一 url 永远同一 id，
   重复导入即覆盖，与 Legado 的唯一键语义一致；local 规则包 id 不受影响。
5. **@js 降级**：用 `ruleexec.CompileFields` 编译五个规则块，按 hard/soft 字段给出
   `unusable` / `partial`，`degradedFields` 逐字段回显；硬字段是
   `bookList` / `chapterList` / `content` / `searchUrl` / `exploreUrl`。
6. **header 生效、其余不支持字段可见**：`header` 经 `SanitizeHeaders` 清洗后随请求发送
   （CR/LF 值与 Host/Content-Length 等键拒绝，丢弃项进 `reason`）；`loginUrl` / `cookie` / `webJs`
   非空时写入 `online.unsupportedFields`，前端提示「这些不会生效」。

## 2.1 通用导入识别（范围微调）

入口按字段判定条目类型，逐条返回 `type`，预览表直接显示「书源 / 净化 / TXT 规则 / 未知」：

| type | 判定字段 | 本期行为 |
|---|---|---|
| `book_source` | 含 `bookSourceUrl`（或书源特征字段） | 适配成规则包，可落库 |
| `replace_rule` | 含 `pattern` + `replacement` | 只识别，`recognized`，不落库 |
| `txt_toc_rule` | 含 `rule` 或单独的 `pattern` | 只识别，`recognized`，不落库 |
| `unknown` | 以上都不匹配 | `unknown_type` + 原因，不静默丢 |

取回 JSON（URL / `legado://` 链接）由 `internal/ruleimport` 负责，本适配器只接收
raw 字节、不发网络请求；净化与 TXT 规则的落库是下一个切片。

## 3. 字段映射

| Legado | norvels | 说明 |
|---|---|---|
| `bookSourceUrl` | `online.bookSourceUrl`（并派生 `id`） | 唯一键；同时必须是 http(s) |
| `bookSourceName` / `bookSourceGroup` | `name` / `online.group` | 名字为空时用 url 兜底 |
| `bookSourceType` | `online.bookSourceType` | 只收 0，其余拒绝 |
| `enabled` / `customOrder` | `enabled` / `priority` | 缺省 enabled=true |
| `searchUrl` / `exploreUrl` | `online.searchUrl` / `online.exploreUrl` | 原样保留，含 `@js` 记 unusable |
| `bookUrlPattern` | `online.bookUrlPattern` | 原样保留，本期不校验不执行 |
| `ruleSearch/Explore/BookInfo/Toc/Content` | `online.rule*` | Legado 原生简写先翻译成标准 CSS 再落库；`@css:`/`@xpath:`/`@json:` 前缀与 `##` 正则段原样保留，转换不了的保留原文并记降级 |
| `header` | `online.header` | 静态请求头，清洗后随请求发送 |
| `loginUrl` / `cookie` / `webJs` | `unsupportedFields` | 保存但不执行 |

## 4. 应用语义

- 可入库：`ok` / `partial`；`unusable`（@js 硬字段）、`invalid`、`unsupported_type`
  **跳过并进 `skipped`**——分享站整包必然混着漫画/音频源，一条坏的不该拖垮整批；
- 只有一条可用书源都没有时才 400（消息指向 results）；
- 合并：同 id 覆盖、其余（含 local 规则包）保留；合并结果走与 PUT 相同的归一化与校验
  （≤20 份、id 唯一、正则合法），校验失败不落库；
- 编译失败的规则不阻断导入：该条记 `partial` 并把原因写进 `reason`（规则串原样保留）。

## 5. 不做

真实网络抓取、登录态与请求头执行、音频/图片/文件源、书源市场自动更新、`web/` 前端。

## 6. 相关

- [ADR-004](0004-online-book-source.md)：在线抓取执行器与 SSRF 边界、规则引擎选型
- [api-source-import](../api/api-source-import.md)：端点与示例
- [source-pluggable](../source-pluggable.md)：本地规则包（`kind=local`）与共用字段
