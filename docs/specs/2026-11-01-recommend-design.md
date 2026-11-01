# 推荐系统设计（2026-11-01）

> 目标：用埋点事件（分类 / 作者 / 停留 / 点击 / 阅读）做一个小型、可解释、带随机性的推荐流。
> 推荐数据独立成库 `norvels-rec.db`，推荐查询与主库 / 正文库互不影响（用户建议）。

## 1. 库布局

| 库 | 文件 | 内容 | 打开方 |
|---|---|---|---|
| 主库 | `norvels.db` | books / chapters / 用户 / 阅读会话 | `internal/store` |
| 正文库 | `norvels-content.db` | chapter_blocks / 指纹 | `internal/store` |
| **推荐库** | `norvels-rec.db` | 事件流 + 兴趣 + 共读 + 热度 | **`internal/recstore`（新）** |

- 路径推导与正文库同口径：主库同目录兄弟文件，`NORVELS_REC_DB` 整路径覆盖。
- 独立 `internal/recstore` 包，只依赖标准库 + modernc sqlite（不依赖 store / model），
  由 `cmd/norvels serve` 组装后注入 `api.Config.Rec`；为 nil 时推荐端点返回 503。
- 推荐库 schema 自带 `PRAGMA user_version` 版本化，**不进主库迁移体系**：辅助数据可整库重建。

## 2. 表结构（rec 库）

```sql
events(id, user_id, kind, book_id, author_id, category_l1, category_l2, value, extra, ts)
  -- 只落「有信号」的事件：click / read / view / search / shelf / rating；impression 只进聚合
book_stats(book_id PK, impressions, clicks, reads, read_seconds, shelf_adds, last_at)
book_daily(book_id, day, impressions, clicks, reads, seconds, PK(book_id, day)) WITHOUT ROWID
user_interest(user_id, dim, key, weight, updated_at, PK(user_id, dim, key)) WITHOUT ROWID
  -- dim: category / author；key: 分类名 / 作者 id
cooccur(book_id, other_id, users, PK(book_id, other_id)) WITHOUT ROWID
recent_items(user_id, book_id, ts, PK(user_id, book_id)) WITHOUT ROWID  -- 近期已曝光，去重
```

写入口径：
- **impression**：只更新 `book_stats.impressions` / `book_daily` / `recent_items`，不落 events 明细（曝光量最大，避免事件表膨胀）。
- **其余事件**：同一事务内落 events + 更新 book_stats / book_daily / user_interest / cooccur。
- **read**：由后端在 `AddReadingSession` 成功后 best-effort 同步写一条（前端阅读器零改动）。
- 兴趣权重用**指数衰减累计**：`w = w·exp(-Δt/14d) + delta`，delta 按事件类型（click 1 / read 1+min(s/300,3) / shelf 3 / rating 5 / view 0.5）。
- 写放大护栏：单次请求一个事务批量插入，impression 走聚合不落明细；门禁 9 用写放大测试验证。
- **保留窗口**：events 明细与 recent_items 保留 90 天，serve 启动时与每 24h 跑一次 `Prune`（best-effort），事件表不无限增长。

## 3. 推荐算法（小型版「多路召回 + 粗排 + 重排」）

参考抖音的四段式，小型库只保留三段的骨架：

1. **多路召回**（api 层，读主库候选 + rec 库信号）
   - 兴趣分类：用户 top 分类下的书（每类取 40）
   - 兴趣作者：用户 top 作者的书
   - 共读：读过的书的共读邻居（item-item co-read）
   - 热门：近 7 天 reads / clicks 排行（rec 库 book_daily）
   - 新鲜：最近入库
   - 随机探索：均匀随机抽样（保证每次不一样）
2. **打分**（`internal/recommend`，纯函数）
   ```
   raw = 3.0·interest(L1) + 1.5·interest(L2) + 2.5·interest(author)
       + 2.0·log1p(cooccur) + 1.5·log1p(hot_reads) + 0.5·log1p(hot_clicks)
       + 1.0·freshness - 1.0·已读 - 0.5·近期已曝光
   score = raw · (0.8 + 0.4·rand) + 0.6·rand   // 随机扰动 + ε 探索
   ```
   10% 候选强制来自随机池（ε-greedy），保证「每次都有新东西」。
3. **重排**：贪心多样性——同作者 ≤2 本、同大分类 ≤4 本；已读书不推；输出 `reason`
   （「因为你常看武侠」「读过《X》的人也在看」「最近很多人读」「最近入库」「随机发现」）。

冷启动（无任何事件）：热门 + 新鲜 + 随机 各取 1/3。

## 4. API 契约（v1）

```
POST /api/events            # 批量上报，≤100 条/次
  {events:[{kind, bookId?, authorId?, categoryL1?, categoryL2?, value?, extra?, ts?}]}
  → 200 {accepted: n}；非法 kind / 超长字段 → 400；未登录按默认账号

GET  /api/recommend?limit=20
  → 200 {items:[{bookId, title, authorName, categoryL1, categoryL2, hasCover, reason, score}], generatedAt}
```

- 鉴权沿用现有中间件；未启用鉴权时全部归默认账号（`store.ScopeUserID` 同一口径）。
- 推荐端点只读，无副作用；`limit` 夹在 1–50。

## 5. 前端

- 新路由 `/discover`（PC / H5 两档）：「发现」页 = 为你推荐卡片流 + 「换一批」。
- 埋点 SDK `web/src/lib/telemetry.ts`：内存队列 1s 合并 → `POST /api/events`；`pagehide` 用 `sendBeacon`。
- 曝光：IntersectionObserver（可见 ≥50% 且停留 ≥500ms 记 impression）。
- 点击：推荐卡跳转前记 click；详情页只在离开时记一条 view（value = 停留秒数）——**没有 start 事件**，白名单见 §4。

## 6. 验收标准（冻结）

- 门禁 9 项全绿（`make gate`），新包覆盖率不低于基线。
- Gherkin：`tests/features/recommend.feature` 覆盖「上报事件 → 兴趣分类书被推荐」「冷启动非空」「随机性（两次请求结果不完全相同）」「已读书不推」。
- 实测：`curl POST /api/events` → `curl GET /api/recommend` 返回非空且带 reason；两次结果顺序/内容不同。
- 资源护栏：1000 条 impression 批量上报，rec 库落盘增长 ≤ 事件本体 × 1.6（写放大测试为证）。
- 前端：`/discover` 在 390 与 1440 断点无横向溢出，截图留证。

## 7. 非目标

- 不做深度学习 / 向量召回（库量级与算力不匹配）；不做实时流式训练。
- 不做多目标排序（完读率 / 时长 / 点赞加权融合）——留作后续。
- 不把推荐事件回写主库（两库解耦是设计前提）。
