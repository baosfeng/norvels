# 交接文档 · H5 书架打磨（shelf 子 agent）

> 基线 commit：86e293f40cab464a73676cf2026fc6a0a223b5ed（开工时 git rev-parse HEAD）
> 本轮结论：验收标准 5 项全过。范围外的两个已知红点是别人的改动，见「遗留风险」。

## 任务契约
打磨 norvels 前端 H5（≤768px）书架页，风格沿用 warm-editorial（token 见 web/src/styles/tokens.css）。

## 写区（只改这些）
- web/src/features/shelf/**
- web/src/styles/ix-discovery.css

## 已确认事实
- 后端夹具 http://127.0.0.1:8099（12 本虚构书），前端 Vite http://localhost:5173（proxy 已配）。
- 走查用 `agent-browser --session shelfpolish open <url>` + `set viewport 390 844` + `screenshot /tmp/x.png`。
- 截图只落 /tmp（本轮最终 3 张：default / sheet / filtered），仓库无图片改动（`git status` 已核）。
- 后端阅读进度接口 `GET /api/reading/progress?limit=50` 提供 bookId/chapterIdx/percent，是书架卡进度的唯一数据源。

## 每轮改动清单

### 第 1 轮（本轮，全部落地）
新增文件：
- `useShelfProgress.ts` — H5 专用进度索引（bookId → ReadingProgress），PC 不取。
- `CardProgress.tsx` — 卡片进度条 + 「已读 N% · 第 M 章」（chapterIdx 0 基，展示 +1）。
- `ShelfCardSkeleton.tsx` — 与真实书卡同尺寸的骨架（首屏 3 张 / 追加 2 张）。
- `shelfFilterChips.ts` — 筛选状态 → chip 列表的纯函数（可单测）。
- `useShelfChipFilters.ts` — 上面那份 model 的 hook 包装（清单/清全部共用 setter）。
- `FilterChipsRow.tsx` — 回显行组件（每个 chip 带 ×）+「清除全部」。
- `ShelfH5Polish.test.tsx` — 14 条新用例（进度 / 回显 / 骨架 / 角标 / 页头 / 吸底按钮）。

改动文件：
- `ShelfCard.tsx` — 角标移进 `.book-title-row`（视觉外壳 `.book-status-chip`），可选 `progress` 渲染进度行。
- `ShelfFilterSheet.tsx` — 分区 + 横排 chip（排序三列、评分六列、作者/书单/标签单行横滑），头部「清除筛选 + ×」、底部「查看 N 本」吸底；选中不收起。
- `ShelfHead.tsx` — H5 去掉重复的 eyebrow，标题下一次给「N 本 · 按X」；排序/筛选合成一个按钮「筛选 n · 排序名」。
- `ShelfList.tsx` — 加载态与追加态改骨架；到底文案改「已经到底啦 · 共 N 本」；透传 `progress`。
- `ShelfPage.tsx` — 接 `useShelfProgress` / `useShelfChipFilters`；有筛选时也取候选表（chip 才能显示人名/书单名）；弹层 `resultCount={loading ? null : total}`。
- `ix-discovery.css` — 追加 H5 书架段（页头 / 书卡按下反馈 / 角标小 chip / 进度 / 骨架 / 回显 chips / 弹层）。
- 测试同步：`ShelfPage.test.tsx` / `ShelfPage.h5-filter.test.tsx` 里「选完即收起」「没有更多了」两处旧断言改为新口径。

## 两条实现判断（读者读不到，记在这里）
1. **弹层改成「选中不收起」**：原来选任何一项都关弹层，调整两项要开两次。现在选中即生效、弹层留着，底部「查看 N 本」收工；Esc / 遮罩 / × 仍是关。这是本轮的交互取舍，旧测试已按新口径改。
2. **文件命名避坑**：`ShelfFilterChips.tsx`（组件）与 `shelfFilterChips.ts`（model）在 macOS 大小写不敏感盘上会互相解析到对方，运行时崩成 "Element type is invalid"。组件已改名 `FilterChipsRow.tsx`——后续别再加只差大小写的同名文件。

## 证据（本轮实跑）
```
cd web && pnpm vitest run src/features/shelf   → Test Files 10 passed / Tests 86 passed
cd web && pnpm exec tsc --noEmit               → exit 0
cd web && pnpm exec eslint src/features/shelf --max-warnings=0 → exit 0
bash scripts/gate-size.sh                      → ✅ 门禁 5 通过
bash scripts/gate-complexity.sh                → ✅ 门禁 4 通过（豁免 37 项）
```

## 遗留风险 / 没做的事
- **全量 vitest 有 1 红**：`src/lib/theme.test.ts`「四档主题」期望 4 项，实际 6 项（day/night/parchment/**green**/**black**/system）——`src/lib/theme.ts` 与 `tokens.css` 有别的 agent 在加主题，不在我的写区，未动。
- **`ix-discovery.css` 是并行写点**：期间出现过别人（阅读历史 / 搜索域）的同文件追加，已确认两侧内容都在、我的段落完整（`grep` 标记齐全）。合并时请以「双方内容都在」为准，不要按行数覆盖。
- Cover 组件未改（不在写区）：无封面书仍是「大色块 + 单字」。我只在书架卡侧把封面收到 48×64（3:4）、统一 `--radius-sm` 圆角，观感已收敛；若要让占位更精致（如书名首字 + 细纹底），需改 `web/src/components/Cover.tsx`，交 leader 统一处理。
