# 来源规则包与能力清单

> 「书源」是**声明式规则包**（Source Profile），不是可执行插件：一份 JSON 描述
> 「怎么认这个来源、用哪几条正则、强制哪个编码」，执行仍由
> [parser-architecture](../parser-architecture.md) 的注册表完成。设计见
> [source-pluggable](../source-pluggable.md)。

> 端点：`GET`/`PUT /api/import/source-profiles`、`POST /api/import/source-profiles/validate`、
> `POST /api/import/source-profiles/import`（导入 Legado 规则：预览/应用，细节见
> [api-source-import](api-source-import.md)）。

## 来源规则包

存 `settings` 表 `source_profiles` 键（JSON 数组，全量替换）。示例全虚构：

```json
{ "profiles": [{
  "id": "sample-dir",
  "name": "示例目录",
  "schemaVersion": 1,
  "enabled": true,
  "priority": 10,
  "kind": "local",
  "match": { "ext": ["txt"], "pathGlob": "/books/示例/**" },
  "format": ["txt"],
  "decode": { "forceEncoding": "" },
  "filters": { "disable": [], "extra": [{ "name": "广告行", "priority": 30, "pattern": "^广告：.*$", "replace": "" }] },
  "meta": [{ "name": "头部作者", "weight": 60, "pattern": "^作者：(.+)$", "author": 1 }],
  "split": { "forceSplitter": "", "rules": [{ "name": "分节符", "pattern": "^=== 第\\d+ 节 ===$" }] },
  "tags": [{ "tag": "奇幻", "keywords": ["魔法"] }],
  "limits": { "maxChapterChars": 30000, "minChapters": 0 }
}] }
```

字段语义与到解析管线的映射见 [source-pluggable](../source-pluggable.md) §3/§4。
导入时规则包**真正生效**：整批编译一次（编码名 / 切章策略名必须在 parser 注册表里，
坏包整份跳过并在报告的 `profileWarnings` 里给原因），每个文件按 `match` 选一份
（`ext` / `pathGlob` / `nameRegex` / `zipNameRegex` 全「与」），多份命中取
`priority` 最高；没有命中就走内置策略。同一文件只应用一份，不做多包叠加。

- `filters.extra` 的正则默认按行锚定（编译时自动补 `(?m)`），按 `priority` 升序在
  **内置过滤器之后**执行——本切片不改解析核心的既有排序，`priority` 只在注入项之间有意义；
- `tags` 与全局词表规则合并：规则包自带的标签在前、全局 `tag_rules` 追加在后，同一标签只记一次；
- dry-run 请求体可选 `profileId` 指定一份规则包，报告 `sample[].profileId` 回显实际生效的
  规则包（空串 = 走内置）；参数说明见 [api-import](api-import.md)。

`kind: "online"` 的规则包额外带 `online` 段（`bookSourceUrl` / `group` / `weight` /
`ruleSearch` / `ruleBookInfo` / `ruleToc` / `ruleContent`），**本期只校验结构、
不发起任何网络请求**。

### GET /api/import/source-profiles

返回 `{ "profiles": [...] }`，未配置时是空数组（不是 null）。返回归一化后的形态：
扩展名小写去点、`schemaVersion`/`kind` 补默认、规则名留空补 `规则N`。

### PUT /api/import/source-profiles

body 同 GET（`profiles` 为**全量替换**，传空数组即清空）。

- 成功 **204**，无响应体（与 `PUT /api/settings/chapter-rules` 一致）；
- 校验失败整批拒绝、**不改库**：不存在「半套规则生效」；
- 正则编译失败 → 400 `invalid_pattern`，消息带「第 N 份规则包的第 M 条规则」；
- 其它问题（份数 / id / 条数 / 长度 / 结构）→ 400 `invalid_param`。

| 上限 | 值 |
|---|---|
| 规则包份数 | 20 |
| 单份规则总数（`filters.extra` + `split.rules` + `meta`） | 30 |
| 单份标签词表条数 | 30 |
| 单条正则长度 | 200 字 |
| 规则包 / 规则名长度 | 60 字 |
| `id` | `[a-z0-9-]{1,64}`，全局唯一 |

`schemaVersion` 只接受 `1`（缺省视为 1）；更高版本整包拒绝，不静默降级。

### POST /api/import/source-profiles/validate

请求体同 PUT，**只校验不落库**，逐份返回：

```json
{ "results": [{ "index": 0, "id": "sample-dir", "ok": true, "reason": "" },
              { "index": 1, "id": "sample-2", "ok": false, "reason": "第 1 条切章规则的正则非法：…" }] }
```

超过 20 份这类「整批拒绝」没有「第几份」可定位，直接 400（与 PUT 同口径）。

## GET /api/settings/capabilities

服务端能力清单：前端所有硬编码上限与策略名单改读它；**只读下发、不接受写入**
（上限是护栏，可配置等于把 DoS 开关交给 JSON）。

```json
{
  "splitters": ["di-unit", "special", "numeric", "en-chapter", "unmarked"],
  "codecs": ["utf-8-bom", "utf-16le", "utf-16be", "utf-8", "gb18030", "big5", "big5-hkscs", "shift-jis"],
  "filters": [{ "name": "bom", "priority": 10 }],
  "metas": [{ "name": "filename", "weight": 80 }],
  "extensions": ["txt", "epub", "zip"],
  "limits": {
    "chapterRules": { "maxCount": 20, "maxPatternRunes": 200, "maxNameRunes": 60 },
    "tagRules": { "maxCount": 50, "maxTagRunes": 20, "maxKeywords": 100, "maxKeywordRunes": 20 },
    "sourceProfiles": { "maxCount": 20, "maxRulesPerProfile": 30, "maxPatternRunes": 200, "maxNameRunes": 60 },
    "upload": { "maxFileBytes": 67108864, "maxTotalBytes": 268435456, "maxFiles": 20 },
    "import": { "maxWorkers": 32, "defaultMaxChapterChars": 30000, "defaultFallbackChunkChars": 8000 },
    "dryRun": { "maxFiles": 200, "samples": 20, "errors": 50 },
    "pagination": { "maxSize": 200 }
  },
  "recommend": {
    "enabled": true,
    "tabs": [{ "id": "for_you", "label": "为你推荐" }, { "id": "resume", "label": "继续阅读" }, { "id": "random", "label": "随机发现" }],
    "defaultLimit": 20,
    "maxLimit": 50
  },
  "events": {
    "maxBatch": 100, "maxFieldLength": 64, "maxExtraLength": 512, "maxTags": 20,
    "kinds": ["impression", "click", "read", "view", "search", "shelf", "rating", "dislike"]
  },
  "ai": { "reviewAvailable": false, "curationAvailable": false }
}
```

- `recommend` / `events` / `ai` 是**运行时能力**（不是上限）：推荐库打不开时
  `recommend.enabled=false`（前端整块不渲染），`events.kinds` 是合法埋点类型的唯一清单，
  `ai.*` 只报「有没有 key」与模型名。第二个客户端接进来时从这里拿契约，不读源码。
- `filters` 按 `priority` 升序（执行顺序），`metas` 按 `weight` 降序（投票顺位）；
- `upload` 是本进程生效值（`Config.MaxUpload*` 注入，0 值回落默认）；
- `dryRun` 是预检的规模上限（与 importer 的导出常量同源）：最多解析 200 个候选，
  样例与错误清单各留 20 / 50 条；
- `limits` 与后端校验共用同一组常量，改上限只改一处。
