# AI 审核（版本替换）

> 升级候选（导入判出「新版本显著更优」）的审核入口。设计见 [设计定稿](../specs/2026-11-01-content-db-and-dedupe-design.md) §6–§7。
> **任何自动替换都不做**：新版本入库即隐藏，替换 / 保留 / 回滚全部要人工点。

## 数据从哪来

导入判出升级候选时（命中率 ≥ `dedupe.threshold` 且质量分至少两项显著更优）：新版本**照常入库**，但 `books.superseded_by`
立刻指向旧书（书架、搜索、详情、阅读都只认 `superseded_by = 0` 的书），同时落一条 `pending` 审核记录。因此：

- 用户**看不到重复的书**，审核页是唯一的版本对照入口；
- 两版正文都在库里，替换 / 回滚只改两个整数字段，不需要重新导入。

## 状态机

| 状态 | 含义 | 可去的下一步 |
|---|---|---|
| `pending` | 等 AI 裁决（或 AI 未启用） | `ready` / `needs_manual` / 人工决策 |
| `ready` | AI 给出结论（两次一致） | 人工决策 |
| `needs_manual` | AI 不可用（无 key / 开关关 / 超时 / 两次不一致 / 无差异段落） | 人工决策 |
| `confirmed_replace` | 已替换：旧书隐藏、新书成为当前版本 | `rolled_back` |
| `confirmed_keep` | 已保留旧版本（新版本继续隐藏） | — |
| `rolled_back` | 已回滚到旧版本 | — |

非法迁移一律 **409** `review_state`（消息里带当前状态与动作）。

## GET /api/import/reviews

查询参数：`status` = `pending`（**默认**）| `ready` | `needs_manual` | `all`；`limit` 默认 50、上限 200
（越界 → 400 `invalid_param`）。未知 status 按字面筛（返回空列表，不报错）。

```json
{
  "aiEnabled": false,
  "items": [{
    "id": 3, "jobId": 2, "status": "ready", "hitRatio": 0.958,
    "oldBook": { "id": 41, "title": "示例旧版", "chapterCount": 24, "wordCount": 25680, "supersededBy": 0,
                 "quality": { "garbageRatio": 0.0002, "noiseRatio": 0.0399, "structureScore": 0.4281,
                              "duplicateRatio": 0.9463, "chapters": 24, "chars": 25686, "computed": true } },
    "newBook": { "id": 57, "title": "示例新版", "chapterCount": 24, "wordCount": 25680, "supersededBy": 41,
                 "quality": { "garbageRatio": 0, "noiseRatio": 0, "structureScore": 0.4280,
                              "duplicateRatio": 0.9465, "chapters": 24, "chars": 25680, "computed": true } },
    "quality": {
      "old": { "garbageRatio": 0.0002, "noiseRatio": 0.0399, "structureScore": 0.4281,
               "duplicateRatio": 0.9463, "chapters": 24, "chars": 25686, "computed": true },
      "new": { "garbageRatio": 0, "noiseRatio": 0, "structureScore": 0.4280,
               "duplicateRatio": 0.9465, "chapters": 24, "chars": 25680, "computed": true },
      "better": [{ "key": "noise", "old": 0.0399, "new": 0 }, { "key": "garbage", "old": 0.0002, "new": 0 }],
      "worse": [],
      "note": ""
    },
    "ai": { "winner": "new", "confidence": 0.82, "reasons": ["错字更少", "广告已清除"] },
    "ruleReasons": ["噪声率 4.0%→0.0%", "乱码率 0.016%→0.000%"],
    "createdAt": "2026-11-01T05:25:53.786Z", "decidedAt": null, "decidedBy": ""
  }]
}
```

| 字段 | 说明 |
|---|---|
| `aiEnabled` | AI 开关当前值（settings 的 `ai.enabled`）；false 时前端提示「仅规则结论，需人工判断」 |
| `hitRatio` | 章级命中率（判定「同一本书」的依据） |
| `oldBook` / `newBook` | 两版书摘要；`supersededBy` = 0 表示这一版是**当前在用的版本** |
| `quality` | 规则侧对比；`better` / `worse` 是维度键（`noise` / `garbage` / `structure`）与新旧取值；`note` 非空 = 走了保守路径（哪一侧没有质量分） |
| `quality.*.computed` | false = 这份质量分**没算过**（不是 0 分），前端应显示「未计算」而不是「完美」 |
| `ai.winner` | `new` / `old` / `equal`；`needs_manual` 时为空 |
| `ai.reasons` | AI 依据（1–3 条）；`needs_manual` 时是「为什么没有 AI 结论」的原因，可直接展示 |
| `ruleReasons` | 导入时算出的规则对比文案（与导入任务的文件备注同源） |
| `decidedAt` / `decidedBy` | 最后一次决策的时间与账号（`user:<id>`，开放模式为空串） |

书已被删除时对应 `oldBook` / `newBook` 只保留 `id`（其余为零值），列表不报错。

## POST /api/import/reviews/{id}/decide

```json
{ "action": "replace" }
```

- `replace`：旧书 `superseded_by = 新书 id`、新书 `superseded_by = 0` → 新书成为当前版本；
- `keep`：**只改状态**（新版本入库时就已隐藏，书侧不动）。

→ 200，返回**决策后的那条记录**（形状同列表 `items[]`）。错误：404 `not_found`、400 `invalid_param`
（action 不是 replace / keep）、409 `review_state`（已决策 / 已回滚的记录不能再决策）。

## POST /api/import/reviews/{id}/rollback

无请求体。旧书 `superseded_by = 0`、新书 `superseded_by = 旧书 id`，状态置 `rolled_back`。

→ 200，返回回滚后的记录。**只有 `confirmed_replace` 能回滚**，其余状态 409 `review_state`。

## 前端要点

1. 待办用 `status=pending`（默认）与 `status=needs_manual`；历史用 `status=all`。
2. 决策后直接用响应体更新那一行，不必重拉列表；书架（`GET /api/books`）已经自动只剩当前版本。
3. `aiEnabled=false` 或 `ai.reasons[0]` 说明原因时，按钮语义不变：AI 只是建议。
