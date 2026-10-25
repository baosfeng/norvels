# H5 书架交接（w3-web / task-5）

## 基线
- 开工 `b63bb3f`（`feat/import-formats`），交付 `f95023d`；未切分支、未推送、未动 web/ 之外的文件。
- 本轮**不碰** ReaderPage（点击唤出/全屏/header 显隐等原型 A/B 拍板后再派）。

## 任务契约
1. H5 书架无限滚动：首屏 `page=1&size=30`，滚到底经 IntersectionObserver 追加下一页，到底「没有更多了」，在途响应丢弃。
2. H5 排序入口：5 种排序（updated/imported/title/chapters/words）从 `isH5 ? null` 放出，H5 用底部弹层；PC 保持下拉 + 页码分页。
3. 视口与触摸底子：100vh → 100dvh（带 100vh 回退）、`env(safe-area-inset-*)`、`touch-action: manipulation`、tap-highlight 透明、控件 `user-select: none`（正文仍可选中）。
4. 注释口径：H5 不是「不渲染工具栏」，管理入口（导入/作者/驾驶舱）才按档位不渲染。

## 改动清单
- `lib/useBookFeed.ts`（新增）：累计取数 hook。状态快照带 `key`（查询序列化）+ `pending`（在途页码），
  响应落库前校验 `prev.key === key && prev.pending === page`；换键时渲染期重置为第 1 页、effect 清理 abort。
  `mode: "paged"` 换页替换（PC），`"infinite"` 追加（H5）；`hasMore = total > items.length`。
- `features/shelf/ShelfPage.tsx`：H5 排序按钮 + `sheet-backdrop/sheet` 弹层（Esc/遮罩/关闭收起，选中回顶部）；
  底部 `shelf-more`：追加中微标、追加失败重试、「没有更多了 · 共 N 本」、sentinel；
  无 IntersectionObserver 时降级为「加载更多」按钮。PC 分页改走同一 hook（`feed.setPage`）。
- `styles/globals.css`：`.reader-shell` 补 100dvh 回退与安全区内边距；新增触屏底子与弹层/底部区样式。
- `styles/prototype.css`：`.layout-a` 补 100dvh 回退与 `padding-bottom: env(safe-area-inset-bottom)`。
- `lib/tier.ts`：分档注释按新口径改写。
- 测试 `features/shelf/ShelfPage.test.tsx`（7 例）：首屏 30 本、滚动追加、到底文案、换排序回 page=1 且替换、在途响应丢弃、PC 分页回归、100vh 仅作 100dvh 回退（源码直读）。

## 验证口径
- `cd web && pnpm run typecheck` → exit 0。
- `cd web && pnpm test -- --run` → 12 文件 / 138 例全绿（上轮 131）。
- `cd web && pnpm exec vitest run --coverage` → 全量 88.42% / 函数 91.86%（阈值 70 通过）。
- `cd web && pnpm run lint` → 0 error（仅 `ChapterList.tsx` 既有 react-hooks 警告）。
- 源码 `100vh` 仅剩 `globals.css:13` 与 `prototype.css:287`，各自紧跟 100dvh 回退。

## 遗留
- 无限滚动在「追加结果仍填不满视口」时会连续自动取下一页（IO 重新 observe 即触发），直到填满或到底；属于预期补屏行为。
- 底部弹层未做 body 滚动锁定；H5 页面较短时可透出背景滚动（如需可下轮补 `overflow: hidden`）。
- 未跑 `make gate`（归 leader 在写操作停止后单点跑）。
