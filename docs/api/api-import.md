# 导入与设置

## 导入

### POST /api/import
```json
{ "dir": "/books", "recursive": true, "force": false, "workers": 4, "dryRun": false, "profileId": "" }
```
→ 202 `{ "jobId": 7 }`；已有运行中的任务时返回 409。

**只预检**：body 加 `"dryRun": true` → **不建任务、不占锁**，同步返回报告（最长 60 秒，超时 503 `timeout`）。`profileId` 可指定一份已配置的来源规则包（空 = 按 `match` 自动选；未配置的 id → 400 `invalid_param`），报告的 `sample[].profileId` 回显实际生效的规则包（空串 = 走内置）：
```json
{
  "truncated": false,
  "files": { "total": 12, "willImport": 9, "duplicate": 2, "unsupported": 1, "failed": 0 },
  "books": { "new": 8, "duplicate": 2, "split": 1 },
  "sample": [{ "source": "/books/a.txt", "title": "示例书名", "chapters": 12, "words": 34567, "duplicate": false, "reason": "" }],
  "errors": [{ "source": "/books/x.pdf", "reason": "unsupported_format" }]
}
```
- 判定口径与真实导入一致：SHA-256 / 源路径命中即 `duplicate`（追更时再加书名+作者指纹），扩展名不支持即 `unsupported`，解析出 0 章记 `failed`；`sample` 最多 20 条（按扫描顺序）、`errors` 最多 50 条；目录候选超过 200 个只解析前 200 并置 `truncated: true`（三个上限由 capabilities 的 `limits.dryRun` 下发）。

`dir` 可以是目录，也可以是单个 `.txt` / `.epub` / `.zip` 文件（如 `/books/某本书.epub`）：

- 目录：扫描其下 txt/epub/zip，`recursive` 决定是否进子目录（缺省 true）；单个文件：只导这一本，`recursive` 被忽略，任务记录里 `sourceDir` 存该文件路径。
- 路径既不是目录也不是支持的文件 → 400 `invalid_param`，不会建任务。
- `update`（布尔，默认 false）：追更，只追加新章节；对不上已有章节序列 → `not_appendable`，绝不覆盖。
- `.zip` 按包内条目逐条导入，条目名为 `包名.zip:包内路径`；四道闸：条目 ≤1000、解压总量 ≤512 MiB、单条目 ≤64 MiB、拒绝绝对路径/../符号链接；嵌套 zip 按不支持条目记 `import_errors`。
- 包内被跳过的条目（不支持的扩展名、嵌套 zip）会**逐条列入任务失败明细**，所以任务状态可能是 `completed_with_errors`——部分成功必须对用户可见，不静默吞掉。

### POST /api/import/upload
`multipart/form-data`：`files`（可重复，最多 20 个）+ `force` / `update`（文本 true/false）。服务端流式读取，文件落到数据卷 `uploads/` 后建一个导入任务，任务结束整目录删除。

只接受 `.txt` / `.epub` / `.zip`（大小写不敏感）。不支持的扩展名**逐个**进 `rejected`，同批合法文件照常导入；全部不支持时返回 400。
```json
202 {
  "jobId": 7,
  "accepted": [{ "name": "合订包.zip:包内书.epub", "format": "epub" }],
  "rejected": [{ "name": "合订包.zip:说明.pdf", "code": "unsupported_format",
                 "message": "暂不支持 .pdf" }]
}
```
zip 的每个条目单独出现在 `accepted`/`rejected` 里：`name` 是 `包名.zip:包内路径`，`format` 取包内真实格式；整包打不开或超限时 `name` 只是包名。

| 情况 | HTTP | code |
|---|---|---|
| 全部文件都不支持 | 400 | `unsupported_format` |
| zip 打不开 / 含越界路径 / 符号链接 | 202，该包进 `rejected` | `invalid_zip` |
| zip 解包超过条目数或体积上限 | 202，该包进 `rejected` | `zip_too_large` |
| zip 里没有可导入条目 | 202，该包进 `rejected` | `empty_zip` |
| 追更对不上已有章节序列 | 409 | `not_appendable` |
| 单文件超上限（默认 64 MiB）或单批总量超上限（默认 256 MiB） | 413 | `payload_too_large` |
| 文件数超过 20 | 400 | `too_many_files` |
| 加密 EPUB（正文条目被加密） | 422 | `encrypted_epub` |
| 已有导入任务在跑 | 409 | `conflict` |

上限可用 `Config` 的 `MaxUploadFileBytes` / `MaxUploadTotalBytes` / `MaxUploadFiles` 调整（0 值回落默认）。只做字体混淆（仅加密 `.ttf/.otf/.woff`）的 EPUB 会放行，正文仍可读；加密预检只覆盖**顶层**上传的 `.epub`（zip 整包上传时不预先解包，包内的加密 EPUB 在导入阶段被拒绝并记为失败条目）。

### GET /api/import/jobs
```json
{ "items": [{ "id": 7, "sourceDir": "/books", "status": "running", "total": 865, "imported": 120, "skipped": 3, "failed": 1, "startedAt": "…", "finishedAt": null, "message": "" }] }
```
### GET /api/import/jobs/{id}
同上单条 + `errors: [{ "filePath": "…", "reason": "…" }]`（最多 200 条）+ `failedCount`（= `import_job_files` 里 `status=failed` 的条数，0012 起）。
### GET /api/import/jobs/{id}/files?status=failed
文件级终态清单（0012 起），按行序：
```json
{ "files": [{ "id": 1, "source": "/books/坏.txt", "displayPath": "坏.txt", "status": "failed", "reason": "解析失败", "bookId": 0, "updatedAt": "…" }], "truncated": false }
```
- `status` 取值 `imported|failed|skipped|unsupported|canceled`，其它值 → 400 `invalid_param`；不带返回全部；最多 1000 条，超出 `truncated: true`；任务不存在 → 404 `not_found`。
- `status=skipped` 的 `reason` 分两种结局：`疑似重复：…（《书名》质量更优/持平/无质量分）` 与 `升级候选：噪声率 3.2%→0.4%、乱码率 …（旧版本《书名》质量更差，需人工确认替换）`；
  升级候选**不入库、不替换**（设计 §7），结构化字段（旧书 id/书名、命中率、新旧质量分、逐项差异）由 `importer.Result.UpgradeCandidates` 交给调用方，服务日志逐条打印。

### POST /api/import/jobs/{id}/retry
body `{ "sources": ["…"] }`（省略 = 重试该任务全部 `failed`）→ **202** `{ "jobId": <新任务 id> }`。

- 重试生成**新任务**（原任务记录不变），新任务的 `import_job_files` 只含本次重试的文件；
- 任务不存在 → 404；任务仍在 `running` → 409 `conflict`；没有可重试项 / 源不在该任务的失败清单里 → 400 `invalid_param`；
- `source` 是可重放源：目录文件用绝对路径，zip 条目用 `<包绝对路径>:<包内路径>`；源已不存在时该文件记为 `failed`，不中断整批。

`status` 取值：`running` / `completed` / `completed_with_errors` / `failed` / **`canceled`**（用户取消或服务退出，已入库的书保留）。

### POST /api/import/jobs/{id}/cancel
- 任务在跑 → **202** `{ "status": "canceling" }`：取消信号已发出，终态 `canceled` 由导入收尾写入（本接口不写库）；任务不存在 → 404 `not_found`；
- 任务已结束（`completed` / `completed_with_errors` / `failed` / `canceled`），或进程重启后遗留的 `running` 记录在本进程没有取消入口 → 409 `conflict`。此时回 202 会让前端一直等一个不会到来的终态。

## 设置

### GET /api/settings/chapter-rules
全局的**自定义章节识别规则**（导入时优先于内置策略，按顺序尝试，命中即作为章节边界）。未配置时返回空数组而非 null：

```json
{ "rules": [{ "name": "分节符", "pattern": "^=== 第\\d+ 节 ===$" }] }
```

### PUT /api/settings/chapter-rules
body 同 GET（`rules` 为**全量替换**，传空数组即清空）。

- 成功 **204**，无响应体（与 `PUT /api/reading/progress` 一致，前端按「无内容」处理）；
- 正则编译失败 → 400 `invalid_pattern`（消息里带第几条）；
- 条数 > 20 或单条正则 > 200 字、正则为空 → 400 `invalid_param`；`name` 只用于展示：留空补「规则N」，超过 60 字截断（不因此拒绝保存）。

### GET /api/settings/tag-rules
词表打标规则（标签 ← 关键词列表），导入时按它给书打 `source=auto` 的标签：
```json
{ "rules": [{ "tag": "奇幻", "keywords": ["魔法", "精灵"] }] }
```
未配置时返回 `{ "rules": [] }`。

### PUT /api/settings/tag-rules
body 同 GET（`rules` 为**全量替换**，传空数组即清空）→ 成功 **204**，无响应体。

- 条数 > 50、标签名为空或 > 20 字、单条关键词 0 或 > 100 个、单个关键词 > 20 字、同名标签出现两次 → 400 `invalid_param`；
- 匹配口径（P1-B §2.4）：书名 + 作者 + 前 3 章标题 + 正文前 5000 字，两边去空白后小写子串比较；单书最多 10 个标签，同一标签命中多个关键词只打一次；规则顺序不影响结果（取并集）；打标只增不删：同书已有同名标签（任意来源）不重复插入且不改其来源，用户 PATCH `tags` 仍是全量替换。

### GET /api/settings/dedupe
内容去重阈值（章节命中率 ≥ 该值即视为同一本书）：`{ "threshold": 0.95 }`；未配置时返回缺省 0.95，库里的值非法时附 `warning` 并按缺省返回。

### PUT /api/settings/dedupe
body 同 GET，取值 **(0, 1]**（0.95 = 95%，不认百分数写法）；越界或非数字 → 400 `invalid_param`；成功 **204**，无响应体。命中率到阈值后判出的「升级候选」走 AI 审核与版本替换，见 [api-reviews](api-reviews.md)。
