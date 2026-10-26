# discovery 域实现说明（impl-notes）

> 分支 `impl/discovery`，基线 `f586c70`。设计冻结：未改 `01-contract.md` / `02-prototype.html` / `03-review.md` / `04-ledger.md`。
> 写域：`web/src/layouts/**`、`features/{start,shelf,search,series,history}/**`、`styles/ix-discovery.css`。

## 1 交付概览

- **锚点 71/71 落地**（`node scripts/check-anchors.mjs --root . --domain discovery` → `71/71 OK`，退出码 0）。
- **新增控件 2 个**：C-54 搜索·上一页、C-55 搜索·下一页（服务端已支持 `page`/`size`，前端首次接入；每页 8 条，书名/作者与章节两栏一起翻页，页码写进地址栏）。
- **触发**：71 个锚点的 click / hover / focus / 键盘 逐行落实（hover 与 focus 走 CSS，键盘走 keydown / 原生控件语义）；4 个输入触发（C-12 顶栏搜索、C-43 首页搜索、C-49 搜索页、C-56 系列搜索）落实逐键回显、Enter 提交、Esc 清空、输入法组合期间不提交、不足 2 字不发请求。本域契约**没有**长按 / 双击 / 右键 / 滑动拖拽行，故未实现（无设计）。
- **四态**：首页最近入库、书架、搜索、系列、系列详情、阅读历史的加载 / 空 / 错误 / 成功均按契约；错误态保留关键词与筛选。
- **禁用原因常驻**：C-33「批量设作者（0）」旁给 `.inline-note`「勾选至少一本书后可用。」；其余禁用（分页首末页、按钮运行中）由文案与位置直接可见。

## 2 锚点 → 文件:行（71）

| 锚点 | 落地位置 |
|---|---|
| C-01 | `web/src/layouts/Sidebar.tsx:35` |
| C-02 | `web/src/layouts/Sidebar.tsx:46` |
| C-03 | `web/src/layouts/Sidebar.tsx:50` |
| C-04 | `web/src/layouts/Sidebar.tsx:42` |
| C-05 | `web/src/layouts/Sidebar.tsx:55` |
| C-06 | `web/src/layouts/Sidebar.tsx:58` |
| C-07 | `web/src/layouts/Sidebar.tsx:61` |
| C-08 | `web/src/layouts/Sidebar.tsx:64` |
| C-09 | `web/src/layouts/Sidebar.tsx:67` |
| C-10 | `web/src/layouts/Sidebar.tsx:71` |
| C-11 | `web/src/layouts/Sidebar.tsx:85` |
| C-12 | `web/src/layouts/Topbar.tsx:69` |
| C-13 | `web/src/layouts/Topbar.tsx:92` |
| C-14 | `web/src/layouts/Topbar.tsx:62` |
| C-15 | `web/src/features/start/HomePage.tsx:68` |
| C-16 | `web/src/layouts/AppShell.tsx:71` |
| C-17 | `web/src/layouts/ToastHost.tsx:25` |
| C-18 | `web/src/layouts/Redirects.tsx:29` |
| C-19 | `web/src/features/shelf/ShelfPage.tsx:408` |
| C-20 | `web/src/features/shelf/ShelfPage.tsx:387` |
| C-21 | `web/src/features/shelf/ShelfPage.tsx:627` |
| C-22 | `web/src/features/shelf/ShelfPage.tsx:425` |
| C-23 | `web/src/features/shelf/ShelfPage.tsx:442` |
| C-24 | `web/src/features/shelf/ShelfPage.tsx:461` |
| C-25 | `web/src/features/shelf/CollectionFilter.tsx:29` |
| C-26 | `web/src/features/shelf/CategoryFilter.tsx:39` |
| C-27 | `web/src/features/shelf/CategoryFilter.tsx:57` |
| C-28 | `web/src/features/shelf/ShelfPage.tsx:479` |
| C-29 | `web/src/features/shelf/ShelfPage.tsx:496` |
| C-30 | `web/src/features/shelf/ShelfPage.tsx:784` |
| C-31 | `web/src/features/shelf/ShelfPage.tsx:742` |
| C-32 | `web/src/features/shelf/ShelfPage.tsx:359` |
| C-33 | `web/src/features/shelf/ShelfPage.tsx:370` |
| C-34 | `web/src/features/shelf/BatchAuthorDialog.tsx:62` |
| C-35 | `web/src/features/shelf/RandomBookButton.tsx:40` |
| C-36 | `web/src/features/shelf/ResumeCard.tsx:22` |
| C-37 | `web/src/features/shelf/ShelfPage.tsx:604` |
| C-38 | `web/src/features/shelf/ShelfPage.tsx:615` |
| C-39 | `web/src/features/shelf/ShelfPage.tsx:586` |
| C-40 | `web/src/features/shelf/ShelfPage.tsx:523` |
| C-41 | `web/src/features/shelf/ShelfPage.tsx:519` |
| C-42 | `web/src/features/start/HomePage.tsx:88` |
| C-43 | `web/src/features/start/HomePage.tsx:116` |
| C-44 | `web/src/features/start/HomePage.tsx:137` |
| C-45 | `web/src/features/start/HomePage.tsx:103` |
| C-46 | `web/src/features/start/HomePage.tsx:144` |
| C-47 | `web/src/features/start/HomePage.tsx:162` |
| C-48 | `web/src/features/start/HomePage.tsx:173` |
| C-49 | `web/src/features/search/SearchPage.tsx:91` |
| C-50 | `web/src/features/search/SearchPage.tsx:117` |
| C-51 | `web/src/features/search/SearchPage.tsx:177` |
| C-52 | `web/src/features/search/SearchPage.tsx:203` |
| C-53 | `web/src/features/search/SearchPage.tsx:159` |
| C-54 | `web/src/features/search/SearchPage.tsx:221` |
| C-55 | `web/src/features/search/SearchPage.tsx:229` |
| C-56 | `web/src/features/series/SeriesPage.tsx:89` |
| C-57 | `web/src/features/series/SeriesPage.tsx:149` |
| C-58 | `web/src/features/series/SeriesPage.tsx:123` |
| C-59 | `web/src/features/series/SeriesPage.tsx:119` |
| C-60 | `web/src/features/series/SeriesPage.tsx:168` |
| C-61 | `web/src/features/series/SeriesPage.tsx:173` |
| C-62 | `web/src/features/series/SeriesDetailPage.tsx:23` |
| C-63 | `web/src/features/series/SeriesDetailPage.tsx:78` |
| C-64 | `web/src/features/series/SeriesDetailPage.tsx:45` |
| C-65 | `web/src/features/series/SeriesDetailPage.tsx:58` |
| C-66 | `web/src/features/history/HistoryPage.tsx:73` |
| C-67 | `web/src/features/history/HistoryPage.tsx:46` |
| C-68 | `web/src/features/history/HistoryPage.tsx:42` |
| C-69 | `web/src/features/history/HistoryPage.tsx:91` |
| C-70 | `web/src/features/history/HistoryPage.tsx:96` |
| C-71 | `web/src/features/start/HomePage.tsx:204` |

## 3 新增控件（C-54 / C-55）

| 锚点 | 控件 | 实现 | 用例 |
|---|---|---|---|
| C-54 | 搜索页 · 上一页 | `web/src/features/search/SearchPage.tsx` 分页条；第 1 页禁用；`changePage` 回顶部、焦点留在按钮 | `SearchPagination.test.tsx`「命中超过一页时出现分页；第 1 页上一页禁用」 |
| C-55 | 搜索页 · 下一页 | 同一分页条；末页禁用；两栏按同一 page 一起换 | 「点下一页按第 2 页重查，且下一页在末页禁用」；「一页装得下时整个分页条不出现」 |

## 4 「移除/关闭后」五问

| 问 | 落实 |
|---|---|
| 撤销窗口 | 筛选 / 排序 / 翻页均不弹确认，改回原值即还原（契约明写无撤销）；C-18 离开 404 无撤销；C-31 角标切换用 toast 回执 |
| 焦点去向 | 抽屉关闭回汉堡（C-14/C-15/C-16）；批量对话框关闭回「批量设作者」（focusTrap）；C-29 清除筛选回排序下拉；C-53/C-59/C-65/C-68 重试后焦点回搜索框 / 面包屑；C-71 重试成功焦点给最近入库首行；C-18 回书架焦点给页标题；翻页焦点留在按钮 |
| 滚动位置 | 排序 / 筛选 / 翻页 / 重试 / 404 回书架均回列表顶部；抽屉关闭与批量对话框关闭保持原位；书架进详情返回仍按会话缓存恢复滚动位置 |
| 计数联动 | 筛选后总数与页数重算（取数钩子按查询键重置）；C-31 状态筛选下书卡移出列表；批次成功后书数徽标随库存刷新 |
| 列表归属 | 不满足筛选的书移出并说明；筛选清空后恢复全量；C-29 现在同时清空大/小分类（原实现漏了分类，本分支修） |

## 5 取证（本分支实跑）

```
node ../norvels/scripts/check-anchors.mjs --root . --domain discovery
→ discovery 71/71 OK · 退出码 0

pnpm -C web typecheck
→ tsc --noEmit -p tsconfig.app.json · 退出码 0

pnpm -C web exec vitest run src/layouts src/features/shelf src/features/search src/features/series src/features/history src/features/start
→ Test Files 10 passed (10) / Tests 64 passed (64) · 退出码 0

pnpm -C web exec eslint src/layouts src/features/start src/features/search src/features/series src/features/history src/features/shelf
→ 0 problems · 退出码 0（顺手清了本域既有 2 条：ShelfPage.tsx 渲染期 Date.now、ShelfPage.test.tsx 正则空格）
```

`pnpm -C web lint` 全仓仍有其它域的既有错误（reader / book / admin 等），不在本域写域内，未动。

## 6 测试增量

- 新增 `features/search/SearchPagination.test.tsx`（5 条）：C-54/C-55 分页出现与禁用、翻页重查、一页装得下不出现、Esc 清空、左右方向键切分栏（C-50）。
- `SearchPage.test.tsx` 两处查询由 `getByRole("button")` 改为 `getByRole("tab")`：契约 C-50 原型本身用 `role="tablist"/role="tab"`，这是按设计补齐可访问角色后的必然调整，不是掩盖缺陷。
- 既有 59 条本域用例全绿，未删改任何断言。

## 7 需要共享改动（已做最小改动，请 lead 复核）

| 文件 | 改动 | 原因 |
|---|---|---|
| `web/src/api/hooks.ts` | `useSearch(q, type, page=1, size=8)` 透传分页 | C-54/C-55 为新增控件；服务端已支持 `page/size`，钩子原先写死不带 |
| `web/src/lib/toast.tsx` | 改为 re-export `layouts/ToastHost.tsx` | C-17 属本域「外壳」，提示条实现须落在本域扫描目录才能对账；去掉契约已删的关闭按钮（只读浮层） |
| `web/src/components/StatePanel.tsx` | **未改**（曾加 `odId` prop，因对账脚本只认字面量 `data-od-id` 已回退） | 面板锚点用本域 `<div data-od-id="C-xx">` 包裹承载 |

`globals.css` / `tokens.css` / `prototype.css` / `routes.tsx` / `api/client.ts` 未改。

## 8 未覆盖项 / 偏差

1. **未提交的工作区改动（主线合并后新增，需 lead 决定是否收）**：`SearchPage.tsx`（`let next: number` 消 lint）、`ShelfPage.tsx`（挂载时间取一次，消渲染期 `Date.now` lint + 分页/清筛选细节）、`ShelfPage.test.tsx`（正则空格 lint）、`HomePage.tsx`（重试用 ref 消 setState-in-effect lint）、`AppShell.tsx`（`useTier` 无条件调用消 rules-of-hooks lint）、`ix-discovery.css`（提示条 200ms 淡入上移 + 错误描边、首页段头 44px、分栏焦点环）。**这些改动只在本 worktree，未 commit、未进主线。**
2. **文件行数债务**：`ShelfPage.tsx` 现 760 行 > 400。属既有超限文件，本轮只加锚点与行为，未重构；建议后续把 `BookCard` / `ShelfToolbar` / `SortSheet` 拆出。
3. **parity 未跑**：按 lead 口径本轮不跑 `odops parity`（H5 页不做数值 parity），由 lead 收口。
4. **长按 / 滑动 / 双击 / 右键**：本域契约 0 行，未实现（不自行补设计）。
5. **C-11 继续阅读卡**：契约要求「进度读取失败时整卡不出现并保留上一次成功的数据」。现实现是 `progress.data?.items[0] ?? null`，取数钩子失败会清空 data，因此失败时整卡不出现但**不保留**上一次数据（契约「保留上一次成功的数据」未完全落实）。
6. **C-03 搜索页保留上一次关键词与分栏选择**：关键词随地址栏保留；分栏选择是页面内 state，离开搜索页再回来会回到「全部」。
7. **C-35 随机一本**：契约结果列写「跳到详情页」（r2 已裁决），实现即此；原型里的「滚动到该书卡并高亮」已被评审裁决删除，未实现。

## 9 文案口径

新增 / 改动文案均为用户语言，未出现 SQLite / API / localStorage / 后端 / 端点 / 数据库等实现词（沿用既有 `findForbiddenTerms` 审计用例的判据）。
