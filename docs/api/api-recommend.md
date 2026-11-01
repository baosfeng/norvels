# 推荐与埋点（发现页）

> 设计见 [2026-11-01-recommend-design](../specs/2026-11-01-recommend-design.md)：推荐数据独立成库
> `norvels-rec.db`（`NORVELS_REC_DB` 可整路径覆盖），整库可重建；缺库时两个端点都返回 503 `unavailable`，
> 其余端点不受影响；推荐相关端点（events / recommend / recommend-stats）统一按此降级。

## POST /api/events

批量上报埋点，**单次 ≤100 条**；整批校验通过才落库（任一条非法整批 400，不做部分接受）。

- body：`{"events":[{kind, bookId?, authorId?, categoryL1?, categoryL2?, tags?, series?, value?, extra?, ts?}]}`
- `kind` 白名单：`impression|click|read|view|search|shelf|rating|dislike`；除 `search` 都必须带 `bookId`。
- 字段上限：分类名 / 标签名 / 系列名各 64 字节、单条事件 ≤20 个标签、`extra` 512 字节；
  `ts` 缺省取服务端当前时间，格式非法 → 400。
- 缺失的 `authorId` / `categoryL1` / `categoryL2` / `tags` / `series` 由后端按 `bookId` 反查补齐
  （`impression` 除外：曝光只进聚合，不需要维度），客户端只要保证带 `bookId`。
- → 200 `{accepted:n}`（n 含 impression）；未启用鉴权时全部归默认账号（与 `store.ScopeUserID` 同口径）。

写入口径（写放大护栏）：

- `impression` 只更新 `book_stats` / `book_daily` / `recent_items`，**不落 `events` 明细**；
- 其余 kind 在同一事务内写 `events` + 聚合 + 兴趣 + 共读；
- 兴趣权重按 14 天指数衰减累计：click 1 / read 1+min(秒数/300, 3) / shelf 3 / rating 5 / view 0.5；
- `dislike`（负反馈）：落明细 + 写入 `negative_items`（召回时整本剔除）+ 相关兴趣各减 2，
  **权重下限为 0**（不出现负分）；**带 `authorId` 时同时拉黑该作者**（`negative_authors`，
  该作者的所有书都不再推），不带就只针对这一本；
- 兴趣维度：`category_l1` / `category_l2` / `author` / `tag`（每个标签一条）/ `series`；
  每条兴趣增量同时记进 `time_interest`（按事件时刻分 4 槽：0-5 深夜 / 6-11 上午 / 12-17 下午 /
  18-23 晚上，本地时区），读取时当前时段的那份 ×1.5 并入全局兴趣（时段偏好）；
- 共读只对 `read` 计数（点击只代表有兴趣，读才代表真读过）。
- **保留窗口**：`events` 明细与 `recent_items` 保留 90 天，serve 启动时与每 24h 清理一次（best-effort，聚合表不清）。

## GET /api/recommend

- `limit` 1–50，默认 20；越界夹取（非整数 → 400）。
- `similar`：`<bookId>`，取「与这本书相似的书」；正整数以外 → 400，书不存在 → 404。
  与 `tab` 互斥且 **similar 优先**（给了种子书就不再校验 tab）。
- `tab`：`for_you`（默认）| `resume` | `random`；非法值 → 400 `invalid_param`。
  - `for_you`：全量召回（见下）；
  - `resume`：只拼「读到一半」的书（进度 0<percent<1），按进度倒序，热门/新鲜兜底保证非空；
  - `random`：随机池 + 热门，ε 探索比例从 10% 提到 **50%**（这个 tab 的用户就是要不一样）。
- → 200 `{items:[{bookId,title,authorName,authorId,categoryL1,categoryL2,hasCover,reason,score}], generatedAt}`
  （`authorId` 供前端「不喜欢这个作者」直接下发 dislike，缺作者时前端置灰）
- 只读、无副作用；`items` 恒为数组（冷启动可能为空，见下）。
- `reason` 取值：「因为你常看<分类>」/「因为你喜欢「<标签>」标签」/「同系列《<系列名>》」/
  「同一个作者」/「同一分类」/「读到一半，接着读」/「读过这本书的人也在看」/「最近很多人读」/
  「最近入库」/「随机发现」。
- **读完不推**（主库 `reading_progress.percent ≥ 0.99`）、`dislike` 过的不推；
  **读过的书只降权**（`ReadPenalty` −1.0），仍然可以出现——把「读了几页」当读完会让
  候选池越用越少，真实库里几乎每本书都被 read 事件覆盖过。
- 同作者 ≤2 本、同大分类 ≤4 本（`resume` tab 不设配额）；默认约 10% 槽位来自随机探索
  （`random` tab 为 50%）。
- 续读候选（进度 0<percent<1）本身不算读完，可以正常出现，理由为「读到一半，接着读」；
  进度到 100% 后它们会转入「读完不推」。

召回来源（每路都回主库补标题与封面）：兴趣分类（L1/L2 合并 top3 × 40）→ 兴趣作者（top2 × 20）→
兴趣标签（top2 × 20）→ 兴趣系列（top2 × 20）→ 共读邻居（≤60）→ 近 7 天热度（≤40）→
最近入库（20）→ 随机（20）→ 续读（最近 20 条进度里读到一半的）。

`similar` 召回顺序固定为 **同系列 → 同作者 → 同分类（L1/L2）→ 该书的共读邻居**，
排除自身、读完、dislike 过的书与拉黑作者；理由按第一条命中的关系给出。

## GET /api/recommend/stats

推荐看板（驾驶舱「推荐与埋点」区块）的聚合数据，只读、无副作用。

- `days` 1–90，默认 14；越界夹取（非整数 → 400）。
- → 200：

```json
{
  "summary": {"days": 14, "impressions": 120, "clicks": 33, "reads": 8, "readSeconds": 2400, "shelfAdds": 5},
  "daily": [{"date": "2026-11-01", "impressions": 12, "clicks": 3, "reads": 1, "seconds": 300}],
  "interests": [{"dim": "category_l1", "key": "武侠", "name": "武侠", "weight": 3.2},
                {"dim": "category_l2", "key": "江湖", "name": "江湖", "weight": 1.6},
                {"dim": "author", "key": "12", "name": "示例作者", "weight": 2.4}],
  "hotBooks": [{"bookId": 1, "title": "示例书", "authorName": "示例作者", "reads": 5, "clicks": 2, "seconds": 900}],
  "reasonMix": [{"reason": "因为你常看武侠", "clicks": 12}],
  "categoryClicks": [{"categoryL1": "武侠", "clicks": 20}]
}
```

口径（两处混用会导致「这台机器上所有人的平均兴趣」这种没有消费方的数字）：

- `summary` / `daily` 是**全局埋点口径**：按书聚合，不分账号。`daily` 从今天往前 days 天平铺、
  缺日补 0、升序；`summary.shelfAdds` 是**累计**收藏数（收藏没有日维度），不是窗口值。
- `interests` 是**当前账号口径**：权重已按 14 天衰减，最多 10 行，同权重按 dim、key 升序。
  每项 `{dim,key,name,weight}`，`dim` 取值
  **`category_l1` | `category_l2` | `author` | `tag` | `series`**：
  一级分类与二级分类**分开计数**（同一本书的「武侠」与「江湖」是两行，不再成对重复）；
  `key` 是稳定标识（作者 id / 分类名 / 标签名 / 系列名），`name` 是展示名——作者维度回主库
  换成作者名（看板上不该出现「作者 1 / 3 / 2」），其余维度 `name` 与 `key` 相同；
  作者已删除时省略 `name`（omitempty），前端按 `name → key` 回退。
- `hotBooks` 固定取近 7 天热度前 10（与推荐召回的窗口参数无关），标题/作者回主库补齐；
  主库已删的书直接跳过（事件不级联删，但榜单上「未知书名」没有信息量）。
- `reasonMix` / `categoryClicks` 是**全局口径**（同 daily），窗口 `days`、各取 top10：
  前者按 click 的 `extra`（推荐理由）分组、空 `extra` 跳过，后者按 click 的 `category_l1` 分组。
  只从 click 明细聚合——`impression` 不落明细（写放大护栏），拿不到曝光侧的理由分布。

## 冷启动

没有任何事件时只有「最近入库 + 随机」两路有货，结果仍然非空（库里至少有一本书时），
理由是「最近入库」或「随机发现」。

## 前端埋点

`/discover` 的曝光（可见 ≥50% 且停留 ≥500ms）、点击、详情页停留秒数都由前端 SDK 合并上报；
阅读会话 `POST /api/reading/sessions` 成功时，后端额外补一条 `read` 事件（best-effort，
失败只记日志，不影响 204）。

## 错误码

- `invalid_param`：kind 未知、缺 `bookId`、字段超长、批量超限、`ts` 非法、
  `days`/`limit` 非整数、`tab` 不在枚举内。
- `unavailable`：推荐库未启用（`Config.Rec` 为 nil，例如启动时打开失败）；`/api/events`、
  `/api/recommend`、`/api/recommend/stats` 三个端点统一返回。
