# impl-notes · 书目域（library）

> 实现方记录。设计冻结在 [01-contract.md](01-contract.md) / [02-prototype.html](02-prototype.html)，本轮**不改设计**，只落实现。
> 写域：`web/src/features/{book,authors,dashboard}/**`、`web/src/lib/charts.ts`、`web/src/styles/ix-library.css`，另经 lead 授权改 `web/src/routes.tsx`（只新增作者作品路由）。

## 1 交付摘要

| 项 | 结果 |
|---|---|
| 契约锚点 | **32 / 32 落地**，每个锚点一个节点，`data-od-id="C-nn"` 与契约逐字相同 |
| 新增控件 | **9 个**（C-06 / C-09 / C-11 / C-16 / C-22 / C-26 / C-27 / C-28 / C-30） |
| 触发 | click / hover / focus / 键盘 / 输入 逐条实现；本域契约无长按与滑动手势 |
| 演示件 | `grep -rn 'demo-only' web/src | wc -l` → **0** |
| 旧描述性锚点 | `book-detail` / `book-aside` / `authors` / `dashboard` / `dashboard:stats` 等全部保留 |

## 2 锚点 → 文件:行

| 锚点 | 控件 | 文件:行 |
|---|---|---|
| C-01 | 面包屑「书架」 | `features/book/BookPage.tsx:106` |
| C-02 | 继续阅读（无进度不渲染） | `features/book/BookAside.tsx:48` |
| C-03 | 从第一章开始 | `features/book/BookAside.tsx:53` |
| C-04 | 加入书单 | `features/book/BookAside.tsx:62` |
| C-05 | 管理这本书（仅 PC） | `features/book/BookAside.tsx:73` |
| C-06 | 系列名链接【新增】 | `features/book/BookAside.tsx:156` |
| C-07 | 阅读状态下拉 | `features/book/BookAside.tsx:98` |
| C-08 | 评分星标行 | `features/book/RatingStars.tsx:88` |
| C-09 | 标签 chip【新增】 | `features/book/BookHeader.tsx:60` |
| C-10 | 展开/收起简介（仅 H5） | `features/book/BookHeader.tsx:105` |
| C-11 | 目录搜索输入框【新增】 | `features/book/BookToc.tsx:74` |
| C-12 | 空态「清空搜索」 | `features/book/BookToc.tsx:176` |
| C-13 | 卷跳转 chip | `features/book/BookToc.tsx:277` |
| C-14 | 卷头 | `features/book/BookToc.tsx:305` |
| C-15 | 章节行 | `features/book/ChapterList.tsx:131` |
| C-16 | 卷内「再显示 60 章」【新增】 | `features/book/BookToc.tsx:365` |
| C-17 | 书单勾选框 | `features/book/AddToCollectionPanel.tsx:164` |
| C-18 | 面板「完成」 | `features/book/AddToCollectionPanel.tsx:190` |
| C-19 | 作者页排序下拉 | `features/authors/AuthorsPage.tsx:40` |
| C-20 | 分页「←」 | `features/authors/AuthorsPage.tsx:99` |
| C-21 | 分页「→」 | `features/authors/AuthorsPage.tsx:110` |
| C-22 | 表格作者名【新增】 | `features/authors/AuthorsPage.tsx:78` |
| C-23 | 「全部书架 →」 | `features/dashboard/DashboardPage.tsx:126` |
| C-24 | 「前往导入管理 →」 | `features/dashboard/DashboardPage.tsx:166` |
| C-25 | 最近在读行 | `features/dashboard/DashboardPage.tsx:143` |
| C-26 | 环形图图例【新增】 | `features/dashboard/Charts.tsx:256` |
| C-27 | 健康卡「去书籍管理」【新增】 | `features/dashboard/DashboardPage.tsx:255` |
| C-28 | 健康卡「去导入」【新增】 | `features/dashboard/DashboardPage.tsx:258` |
| C-29 | 「刷新概览」 | `features/dashboard/DashboardPage.tsx:91` |
| C-30 | 热力格【新增】 | `features/dashboard/ReadingHeatmap.tsx:214` |
| C-31 | 图表卡「重试」 | `features/dashboard/Charts.tsx:100` |
| C-32 | 署名作者链接 | `features/book/BookHeader.tsx:42` |

C-15 / C-25 / C-17 / C-22 / C-26 / C-30 / C-31 是「每行/每格一个」的控件：同一锚点在同文件里挂在每个实例上（与原型同粒度），脚本按锚点去重，不报重复。

## 3 新增控件清单（9 个）

| 锚点 | 控件 | 文案 | 前置 / 边界 | 落在哪 |
|---|---|---|---|---|
| C-06 | 系列名链接 | 系列名（如「长夜」），后面仍显示「· 第 2 部」 | 有系列名才渲染；跳到 `/series/:name`（路由已存在） | `BookAside.tsx` + `.fact-link` |
| C-09 | 标签 chip | 标签原文 | 有标签才渲染；点击 = 写入书架筛选意图 + `/shelf?tag=…` | `BookHeader.tsx` + `shelfTagLink.ts` |
| C-11 | 目录搜索 | 占位「搜索章节名或章号，例如「雨」」；计数「412 章」→「35 章匹配」；结果「找到 35 章」 | 无章节时整块不渲染；输入即筛，Esc 清空，Enter 不提交 | `BookToc.tsx` + `tocView.ts` |
| C-12 | 清空搜索 | 「清空搜索」 | 只在命中 0 条时出现；点完回全量目录、焦点回输入框 | `BookToc.tsx` |
| C-16 | 卷内再显示 60 章 | 「在本卷再显示 60 章（本卷还有 145 章）」 | 本卷还有未显示的章才渲染；一次补 60 行 | `BookToc.tsx` + `tocView.ts` |
| C-22 | 作者作品入口 | 作者名 | 每行一个；新建 `/authors/:authorId` 页面（`AuthorWorksPage.tsx`） | `AuthorsPage.tsx`、`AuthorWorksPage.tsx` |
| C-26 | 图例过滤 | 「已读完 38 本」等 | 三段各一个可聚焦按钮；`aria-pressed` 同步；隐藏段透明度 0.16，数字不变；隐藏时补一句「已隐藏的段不画在环上，总数不变。」 | `Charts.tsx` |
| C-27 | 健康卡按钮 | 「去书籍管理」 | 健康卡有数据时渲染；跳 `/admin` | `DashboardPage.tsx` |
| C-28 | 健康卡按钮 | 「去导入」 | 同上；跳 `/import` | `DashboardPage.tsx` |
| C-30 | 热力格 | 每格 `aria-label`「8 月 7 日 · 1 小时 10 分」/「10 月 2 日 · 没有阅读」；浮层同文 | 补位空格透明不可聚焦；91 天全 0 换空态「读几分钟就会在这里留下第一格」；hover 停 120ms 才浮出、移开立即消失；click 固定 5 秒；Esc 收起 | `ReadingHeatmap.tsx` |

## 4 「移除 / 关闭后」五问自查

| 控件 | 撤销窗口 | 焦点去向 | 滚动位置 | 计数联动 | 列表归属 |
|---|---|---|---|---|---|
| C-04 / C-18 书单面板 | 勾选即时写入，完成后给「已加入/已移出书单」2 秒提示条（不需撤销） | 关闭后回「加入书单」按钮（`collectionButtonRef`） | 打开时锁背景滚动，关闭还原；面板内滚动不动 | 每行本数 +1/-1，失败回滚 | 本书归属按勾选结果定格 |
| C-12 清空搜索 | 清空即恢复全量，可重新输入 | 焦点回搜索框 | 滚回目录顶部 | 计数回「412 章」 | 当前章选中态保留 |
| C-14 折叠卷 | 可再展开 | 焦点留卷头 | 页面不滚 | 目录计数仍按全书 | 该卷章节行移出 DOM，当前章高亮随之隐藏 |
| C-16 再显示 60 章 | 无需撤销（只是多显示） | 按钮仍在 → 回按钮；按钮消失 → 本卷新增末行 | 滚动不动 | 计数不变（全书真实数） | 本卷归属不变 |
| C-26 图例过滤 | 再点恢复 | 焦点留被点图例 | 卡片不滚 | 数字与总数不变 | 只影响本视图 |
| C-29 刷新概览 | 无需撤销 | 焦点留按钮 | 滚动不动 | 全部卡片就地更新 | 筛选与图例状态保留 |
| C-30 热力格浮层 | 浮层 5 秒自散 / Esc 收起 | 焦点留该格 | 滚动不动 | 今日/本周/累计/连续四个数字不变 | 格子档位不变 |
| C-31 卡片重试 | 无需撤销 | 成功 → 卡片标题；仍失败 → 按钮 | 滚动不动 | 本卡更新，其他卡不受影响 | 图例过滤状态保留 |
| C-11 目录搜索 | Esc / 清空按钮 | 输入框 | 清空回目录顶部 | 计数随命中变 | 目录归属不变 |

## 5 证据（可复跑）

```bash
# 锚点对账（用主仓库升级版脚本，按域扫描）
node scripts/check-anchors.mjs --root ../norvels-wt-library --domain library
# → library 32/32 OK；✅ 所选域全部锚点已落地；exit 0

# 定向测试（含新增控件用例）
pnpm -C web exec vitest run src/features/book src/features/authors src/features/dashboard src/lib/charts.test.ts
# → Test Files 17 passed (17)；Tests 137 passed (137)；exit 0

pnpm -C web typecheck        # → exit 0
pnpm -C web exec eslint src/features/book src/features/authors src/features/dashboard src/lib/charts.ts src/routes.tsx
# → 0 errors / 1 warning（ChapterList 的 TanStack Virtual 兼容性提示，改动前既有）；exit 0

grep -rn 'demo-only' web/src | wc -l    # → 0
```

新增用例（每个新增控件至少 1 条）：`BookToc.test.tsx`（C-11/C-12/C-16）、`BookAside.test.tsx`（C-06/C-07/C-08）、`BookHeader.test.tsx`（C-09/C-10/C-32）、`AuthorsPage.test.tsx`（C-19/C-20/C-21/C-22）、`AuthorWorksPage.test.tsx`（C-22 落点）、`Charts.ix.test.tsx`（C-26/C-31）、`ReadingHeatmap.ix.test.tsx`（C-30）、`DashboardPage.test.tsx`（C-23…C-29）、`AddToCollectionPanel.ix.test.tsx`（C-17/C-18 移除后行为）。

## 6 未覆盖项 / 偏差（逐条写原因）

| # | 项 | 情况 | 原因 / 建议 |
|---|---|---|---|
| 1 | C-07「保存中可聚焦但不可改」 | 实现用 `disabled`：置灰、不可展开、防连点，但**不可聚焦** | 原生 `<select>` 的 disabled 与「可聚焦」互斥（ARIA 上 disabled 也不进 Tab 序列）；契约同一行既写「置灰不可展开」又写「焦点环仍在」，实现取前者。若必须可聚焦，需作者改契约为 `aria-disabled` + 拦截 |
| 2 | C-29 提示条时长 | 实现 4 秒（全局 toast） | 契约写 2 秒；`lib/toastStore.ts` 是共享文件，本轮不动。建议由 lead 统一给 toast 加时长参数，或本域改本地提示条 |
| 3 | C-09 书架侧按标签筛选 | 详情页写入书架会话状态 + `/shelf?tag=…`；**书架读取 URL 参数属 discovery 域** | `features/shelf/ShelfPage.tsx` 不在本域写域。需要 discovery 域在初始化 tag 时读 `useSearchParams`，否则刷新页面后标签条件丢失 |
| 4 | C-11 命中超过 60 条的后续 | 只显示前 60 条并说明「先显示前 60 章」，没有「继续显示」 | 契约未定义搜索结果的翻页；不自行补。建议作者补一行 |
| 5 | C-16 按钮的网格定位 | 按钮在目录网格之下、全宽（CSS 带 `grid-column:1/-1` 兜底） | 目录网格由虚拟滚动容器拥有，插入网格内会与虚拟行高度冲突；视觉上仍跨满整行 |
| 6 | 焦点环规格 | 全部控件用全局 `:focus-visible`（2px `--accent` + 2px offset），未逐控件写「4px rgba(155,91,50,.24) 外环」 | 全站已有统一焦点环口径；逐控件覆写会造成同一页三种焦点环。属 A 类翻译损耗 |
| 7 | C-31 的覆盖范围 | 只挂在 6 张图共用的 `ChartFrame`（对应契约点名的 `Charts.tsx:64`）；阅读时长卡失败态的重试按钮仍是无锚点的按钮 | 契约 C-31 只点名图表卡；热力卡是否同样纳入需产品决定 |
| 8 | C-13 前置「本书 3 卷及以上」 | 产品与既有用例是「2 卷及以上才出卷跳转」；本轮保持产品现状 | 既有 `BookPage.test.tsx` 用 2 卷书断言卷跳转存在，改契约口径会打破既有用例；且 2 卷时跳转确实省一次滚动。建议作者确认是否真要在 2 卷时去掉 |
| 9 | 前端函数长度的 70 行口径 | 新增 TSX 组件按 JSX 统计超过 70 行（与既有 `DashboardPage` / `ChapterList` 同口径）；纯逻辑函数全部 <70 行 | 仓库 `scripts/gate-size.sh` 只统计 Go；前端没有对应脚本，严格套用会把所有组件判定超标。若要强制，需先定 JSX 计数口径 |

## 7 需要的共享 / 跨域改动

1. **discovery 域**：`ShelfPage` 初始化时读 `?tag=`（见上表 #3），否则 C-09 只在「点进来这一次」生效。
2. **共享文件**：本域未改 `globals.css` / `tokens.css` / `prototype.css` / `api/**`。唯一新增的跨域依赖是：`web/src/routes.tsx` 加了一条 `authors/:authorId` 路由（lead 已授权）；若 lead 希望统一收口路由，可把这条移回主分支处理。
3. **全局 toast 时长**（见 #2）：若要严格 2 秒，需要 `lib/toastStore.ts` 支持自定义时长。

## 8 关键实现说明（便于复核）

- 详情页拆成 `BookPage`（编排）+ `BookAside`（左栏）+ `BookHeader`（头部/标签/简介）+ `BookToc`（目录/搜索/分卷）；`BookPage` 从 403 行降到 179 行，目录逻辑的纯函数抽到 `tocView.ts`（搜索命中、60 条分块、按钮文案）。
- 评分星标 hover 只改视觉（`is-preview`），不改 `aria-pressed` 与真值；保存中整组禁用 0.4 透明度。
- 书架标签筛选的意图写在 `features/book/shelfTagLink.ts`：写会话状态 + 带 `?tag=`，不越过书架的取数口径。
- 热力格每格自带 `aria-label` 与浮层文案（闭包持有自己的 day），修掉原型首轮 P0-4 那类「91 格同一句」的坑。
- 图表重试：`重试中…` + 禁用 + 骨架块，最短 240ms；连续 3 次失败补「多次失败，检查服务是否在运行。」；成功把焦点送回卡片标题。
