# 净化规则（replaceRule）

> 把 Legado 的净化规则变成可导入、可试跑的数据：存 `settings` 的 `replace_rules` 键
> （JSON 数组、全量替换，与 `chapter_rules` / `tag_rules` / `source_profiles` 同形）。
> 设计见 [source-pluggable](../source-pluggable.md) 与 [ADR-006](../adr/0006-js-engine.md)；
> 本页只写接口契约。

## CleanRule 字段

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `id` | int | 自动补 | 缺省按数组下标从 1 起；只用于回显与 preview 的 `ruleId` |
| `name` | string | `规则N` | 展示名，≤60 字；留空补默认、超长截断（不因此拒绝保存） |
| `group` | string | 空 | 分组（如「格式」「内容」），只影响前端归类 |
| `order` | int | 0 | 执行顺序，升序；可为负。只在自定义规则之间比较 |
| `enabled` | bool | false | 停用规则保留在配置里但不参与导入与试跑 |
| `isRegex` | bool | false | `true` 按正则；`false` 按**字面量**（见下节） |
| `pattern` | string | 必填 | 查找表达式，≤200 字，空串 400 |
| `replacement` | string | 空 | 替换串，支持 `$1` 组引用 |
| `scopeTitle` / `scopeContent` | bool | false | 作用于章节标题 / 正文（透传，供前端标记） |

## 转义语义（`isRegex=false`）

`pattern` 被 `regexp.QuoteMeta` 转义后按正则执行：`a.b` 只匹配字面量 `a.b`，**不匹配** `axb`；
`*`、`(`、`[` 等全部按字面量处理。因此用户不需要懂正则也能写「删除这一行广告」。
`isRegex=true` 时按 RE2 编译，编译失败在 PUT 阶段就拒绝（`invalid_pattern`，消息带第几条）。

## 顺序

1. **内置过滤器先行**：`bom`(10) → `newline`(20) → `noise`(30) → `width`(40) 固定在最前；
2. 全局净化规则按 `order` **升序**追加在内置之后（停用的不进入执行链）；
3. 与规则包自身的 `filters.extra` 合并时，**全局规则整体前置**、规则包自带规则在后；
4. 合并后同一批注入项之间再按 `priority` 升序（规则包编译时排序）。

## 端点

### GET /api/settings/replace-rules
未配置时返回 `{"rules":[]}`（不是 `null`）：
```json
{ "rules": [{ "id": 1, "name": "去广告", "group": "内容", "order": 1, "enabled": true,
  "isRegex": true, "pattern": "广告\\d+", "replacement": "",
  "scopeTitle": false, "scopeContent": true }] }
```

### PUT /api/settings/replace-rules
请求体同 GET，**全量替换**；成功 **204** 无响应体（与 `PUT /api/settings/chapter-rules` 一致）。

| 情况 | HTTP | code |
|---|---|---|
| 超过 100 条 / 空 pattern / pattern > 200 字 | 400 | `invalid_param`（消息带「第 N 条」） |
| `isRegex=true` 且正则编译失败 | 400 | `invalid_pattern`（消息带「第 N 条」与编译原因） |
| 请求体不是 `{"rules":[…]}` | 400 | `invalid_param` |

### POST /api/settings/replace-rules/preview
对一段文本试跑**当前已保存**的规则（不落库、不改配置）：
```json
{ "text": "广告123 a.b axb" }
```
→
```json
{ "before": "广告123 a.b axb", "after": " X axb",
  "hits": [{ "ruleId": 1, "name": "去广告", "count": 1 },
           { "ruleId": 2, "name": "字面量", "count": 1 }] }
```

- `hits` 口径：每条**启用**规则在**当前文本**（前面规则已应用之后）上的匹配次数，
  按 `order` 顺序逐条执行；停用规则不出现在 `hits` 里；
- `after` 是全部规则依次替换后的文本；`before` 原样回显，便于前端做 diff；
- 文本上限 1 MiB，超出 400 `invalid_param`；规则里一条坏正则不会中断试跑（跳过并继续）。

## 与导入的关系

保存后立即生效：目录导入、上传导入、dry-run 预检三条入口都会在读 settings 时把规则
编译成 `parser.Filter` 注入，与规则包的 `filters.extra` 合并。**不批量回写已入库正文**
——清洗只发生在解析阶段。