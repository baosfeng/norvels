# 推荐与埋点（发现页）

> 设计见 [2026-11-01-recommend-design](../specs/2026-11-01-recommend-design.md)：推荐数据独立成库
> `norvels-rec.db`（`NORVELS_REC_DB` 可整路径覆盖），整库可重建；缺库时两个端点都返回 503 `unavailable`，
> 其余端点不受影响。

## POST /api/events

批量上报埋点，**单次 ≤100 条**；整批校验通过才落库（任一条非法整批 400，不做部分接受）。

- body：`{"events":[{kind, bookId?, authorId?, categoryL1?, categoryL2?, value?, extra?, ts?}]}`
- `kind` 白名单：`impression|click|read|view|search|shelf|rating`；除 `search` 都必须带 `bookId`。
- 字段上限：分类名 64 字节、`extra` 512 字节；`ts` 缺省取服务端当前时间，格式非法 → 400。
- 缺失的 `authorId` / `categoryL1` / `categoryL2` 由后端按 `bookId` 反查补齐（`impression` 除外：
  曝光只进聚合，不需要维度），客户端只要保证带 `bookId`。
- → 200 `{accepted:n}`（n 含 impression）；未启用鉴权时全部归默认账号（与 `store.ScopeUserID` 同口径）。

写入口径（写放大护栏）：

- `impression` 只更新 `book_stats` / `book_daily` / `recent_items`，**不落 `events` 明细**；
- 其余 kind 在同一事务内写 `events` + 聚合 + 兴趣 + 共读；
- 兴趣权重按 14 天指数衰减累计：click 1 / read 1+min(秒数/300, 3) / shelf 3 / rating 5 / view 0.5；
- 共读只对 `read` 计数（点击只代表有兴趣，读才代表真读过）。
- **保留窗口**：`events` 明细与 `recent_items` 保留 90 天，serve 启动时与每 24h 清理一次（best-effort，聚合表不清）。

## GET /api/recommend

- `limit` 1–50，默认 20；越界夹取（非整数 → 400）。
- → 200 `{items:[{bookId,title,authorName,categoryL1,categoryL2,hasCover,reason,score}], generatedAt}`
- 只读、无副作用；`items` 恒为数组（冷启动可能为空，见下）。
- `reason` 取值：「因为你常看<分类>」/「读过这本书的人也在看」/「最近很多人读」/「最近入库」/「随机发现」。
- 已读不推；同作者 ≤2 本、同大分类 ≤4 本；约 10% 槽位来自随机探索（保证每次不一样）。

召回来源（每路都回主库补标题与封面）：兴趣分类（top3 × 40）→ 兴趣作者（top2 × 20）→
共读邻居（≤60）→ 近 7 天热度（≤40）→ 最近入库（20）→ 随机（20）。

## 冷启动

没有任何事件时只有「最近入库 + 随机」两路有货，结果仍然非空（库里至少有一本书时），
理由是「最近入库」或「随机发现」。

## 前端埋点

`/discover` 的曝光（可见 ≥50% 且停留 ≥500ms）、点击、详情页停留秒数都由前端 SDK 合并上报；
阅读会话 `POST /api/reading/sessions` 成功时，后端额外补一条 `read` 事件（best-effort，
失败只记日志，不影响 204）。

## 错误码

- `invalid_param`：kind 未知、缺 `bookId`、字段超长、批量超限、`ts` 非法。
- `unavailable`：推荐库未启用（`Config.Rec` 为 nil，例如启动时打开失败）。
