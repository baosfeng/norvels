# 阅读域实现说明（impl-notes）

> 分支 `impl/reader`；写域：`web/src/features/reader/**`、`web/src/lib/{useReaderControls,autoScroll,tocRows,bookmark,readerSearch}.ts`、`web/src/styles/ix-reader.css`。
> 设计冻结，未改契约 / 原型 / 评审 / 台账；共享文件（globals.css / tokens.css / prototype.css / routes.tsx / api/**）未动。

## 1 交付

- 62 个契约锚点全部落地（`data-od-id="C-xx"` 源码字面量，一锚点一节点；列表行按原型粒度逐行挂载）。
- 10 个新增控件全部实现：C-15、C-16、C-27、C-29、C-31、C-58、C-59、C-60、C-61、C-62。
- 四类触发：click（onClick）、hover（全局 `.btn/.icon-btn/.toc-vol` 150ms 过渡）、focus（`:focus-visible` + `--focus-ring`）、键盘（Enter/Space 原生按钮、Esc、方向键、Home/End、`?`）。
- 触屏：C-09 单击（位移 ≤12px、按住 ≤700ms，长按作废）、C-10 左右滑动 ≥60px 且横向 > 纵向 ×1.2 为既有实现，未改阈值。
- 禁用原因一律常驻 `.inline-note`（C-05/11/12/14/15/20/23/26/29/31/32/38/39/42/43/44/47/51/52/53）。

## 2 锚点 → 文件:行（行号为本次提交快照）

| 锚点 | 文件:行 | 锚点 | 文件:行 |
|---|---|---|---|
| C-01 | ReaderTopBar.tsx:57 | C-02 | ReaderTopBar.tsx:72 |
| C-03 | ReaderTopBar.tsx:81 | C-04 | ReaderTopBar.tsx:90 |
| C-05 | ReaderTopBar.tsx:100 | C-06 | ReaderTopBar.tsx:106 |
| C-07 | ReaderTopBar.tsx:114 | C-08 | ReaderTopBar.tsx:119 |
| C-09 | ReaderPage.tsx:619 | C-10 | ReaderPage.tsx:629 |
| C-11 | ReaderPager.tsx:20 | C-12 | ReaderPager.tsx:27 |
| C-13 | ReaderPager.tsx:24 | C-14 | ReaderStateArea.tsx:51 |
| C-15 | ReaderStateArea.tsx:73 | C-16 | ReaderRestoreHint.tsx:34 |
| C-17 | ReaderBottomBar.tsx:98 | C-18 | ReaderBottomBar.tsx:103 |
| C-19 | ReaderBottomBar.tsx:107 | C-20 | ReaderBottomBar.tsx:116 |
| C-21 | ReaderBottomBar.tsx:123 | C-22 | ReaderBottomBar.tsx:127 |
| C-23 | ReaderBottomBar.tsx:131 | C-24 | ReaderPage.tsx:714 |
| C-25 | TocDrawer.tsx:250 | C-26 | TocDrawer.tsx:259 |
| C-27 | TocList.tsx:55 | C-28 | TocList.tsx:22 |
| C-29 | TocDrawer.tsx:208 | C-30 | TocDrawer.tsx:234 |
| C-31 | TocDrawer.tsx:303 | C-32 | BookmarksDrawer.tsx:62 |
| C-33 | BookmarksDrawer.tsx:99 | C-34 | BookmarksDrawer.tsx:108 |
| C-35 | BookmarksDrawer.tsx:79 | C-36 | BookmarksDrawer.tsx:53 |
| C-37 | ReaderSearchPanel.tsx:57 | C-38 | ReaderSearchPanel.tsx:76 |
| C-39 | ReaderSearchPanel.tsx:79 | C-40 | ReaderSearchPanel.tsx:82 |
| C-41 | ReaderSettingsPanel.tsx:361 | C-42 | ReaderSettingsPanel.tsx:176 |
| C-43 | ReaderSettingsPanel.tsx:203 | C-44 | ReaderSettingsPanel.tsx:213 |
| C-45 | ReaderSettingsPanel.tsx:237 | C-46 | ReaderSettingsPanel.tsx:249 |
| C-47 | ReaderSettingsPanel.tsx:261 | C-48 | ReaderSettingsPanel.tsx:275 |
| C-49 | ReaderSettingsPanel.tsx:287 | C-50 | ReaderSettingsPanel.tsx:297 |
| C-51 | ReaderSettingsPanel.tsx:307 | C-52 | ReaderSettingsPanel.tsx:319 |
| C-53 | ReaderSettingsPanel.tsx:349 | C-54 | ReaderSettingsPanel.tsx:170 |
| C-55 | ReaderAutoScrollBar.tsx:27 | C-56 | ReaderAutoScrollBar.tsx:30 |
| C-57 | ReaderHelpPanel.tsx:40 | C-58 | BookmarkConfirmDialog.tsx:34 |
| C-59 | BookmarkConfirmDialog.tsx:40 | C-60 | BookmarkConfirmDialog.tsx:43 |
| C-61 | TocDrawer.tsx:283 | C-62 | TocDrawer.tsx:286 |

## 3 新增控件与用例

| 锚点 | 控件 | 实现 | vitest |
|---|---|---|---|
| C-15 | 正文空态「先读下一章」 | `ReaderStateArea.tsx`（空态 actions；末章 disabled + 常驻原因） | `ReaderNewControls.test.tsx` C-15 组 |
| C-16 | 进度恢复提示条「已回到上次读到的地方 / 知道了」 | `ReaderRestoreHint.tsx` + `ReaderPage`（storedParaIndex>0 时出现；滚动超一屏自动收；关闭后焦点回 C-09） | C-16 组 |
| C-27 | 目录卷折叠 | `TocList.tsx` 卷头 + `TocDrawer` collapsed Set；空卷 disabled + 原因；单卷也显示卷头 | C-27 组 |
| C-29 | 再显示 60 章 | `TocDrawer` revealed/TOC_CHUNK；加载中文案、全量后「已显示全部 N 章」+ 原因 | C-29 组 |
| C-31 | 目录空态清空搜索 | 无匹配态按钮；清空关键词、回列表顶、焦点回搜索框；空目录不出现 | C-31 组（含空目录分支） |
| C-58 | 删除书签二次确认框 | `BookmarkConfirmDialog.tsx`（标题/正文写明对象名与不可恢复；遮罩/Esc=取消） | C-58 组 |
| C-59 | 取消 | 确认框内独立按钮；关闭后焦点回该行删除按钮 | C-58/59/60 组 |
| C-60 | 删除书签 | 「正在删除…」禁用；成功后行移除、焦点下一行（末行上一行）、计数 -1、浮条「书签已删除」；失败保留行 | C-58/59/60 组 |
| C-61 | 目录失败态重试 | 错误面板 + 「正在重试…」禁用；成功后回搜索框、列表回顶 | C-61/C-62 组 |
| C-62 | 目录失败态用平铺列表 | 隐藏卷头、平铺渲染、保留搜索、焦点首行、浮条提示 | C-61/C-62 组 |

其余关键行为用例：C-14 连续失败 3 次兜底、C-02 不支持全屏降级（`ReaderPage.test.tsx`）、C-29 真追加、C-31 焦点、C-58 默认焦点在取消。

## 4 移除/关闭后五问（逐条打勾）

- [x] 撤销窗口：书签删除不可恢复 → 二次确认替代撤销（C-58/59/60）；无其它破坏性操作；契约明确不做撤销条。
- [x] 焦点去向：抽屉/面板关闭由 `useFocusTrap` 还焦点给触发按钮；确认取消→该行删除按钮；确认删除→下一行（末行→上一行）；C-31→搜索框；C-16→正文 C-09；C-62→首行。
- [x] 滚动位置：目录/设置关闭保持；搜索关闭回打开前 `scrollY`；书签跳转按段；C-29 停在按钮处；C-31/C-62 回列表顶。
- [x] 计数联动：书签抽屉「n 条」随 reload；目录章/卷数、匹配数同源计算；进度计数按章更新。
- [x] 列表归属：删除行立即移出；搜索态只显示命中；平铺模式不出现卷头；当前章高亮随切章移动。

## 5 证据（本 worktree 实测）

```
node ../norvels/scripts/check-anchors.mjs --root . --domain reader
  reader 62/62 OK   ✅ 退出码 0
pnpm -C web exec vitest run src/features/reader src/lib
  Test Files 31 passed / Tests 308 passed  退出码 0
pnpm -C web typecheck
  tsc --noEmit  退出码 0
pnpm -C web exec eslint src/features/reader src/lib/{useReaderControls,autoScroll,tocRows,bookmark,readerSearch}.ts
  0 errors / 4 warnings（均为既有的 react-refresh「只导出组件」提示，非本次新增）
git status --short  → 空
```

全仓 `pnpm -C web lint` 仍退出 1，但 12 个 error 全在写域外（`features/admin/AdminBookPage.test.tsx`、`features/book/AddToCollectionPanel.tsx`、`features/book/ChapterList.tsx`、`features/book/collectionMembers.ts`、`features/shelf/ShelfPage.test.tsx`、`features/shelf/ShelfPage.tsx`、`layouts/AppShell.tsx`），本域文件 0 error。

## 6 未覆盖 / 偏差 / 需产品决定

| 项 | 说明 |
|---|---|
| C-42 字号边界 | 契约写 15–26px，源码与既有测试是 14–28px；按「与源码对齐」保留 14–28，禁用说明按实际边界渲染。需产品决定是否改契约。 |
| C-44 行距边界 | 契约写 1.3–2.2，源码为 1.4–2.4；同上保留源码值。 |
| C-58 原型挂点 | 原型把 C-58 挂在 toast 节点；实现按契约语义挂在确认框容器（实际控件）。 |
| C-13 名称 | 契约 P2 建议改名「返回书籍详情」，设计冻结本轮未改，仍为「返回目录」跳书籍详情。 |
| 抽屉宽度 / H5 设置面板高度 | 契约 420px / 74%，现有共享样式为 390px / 70dvh（`prototype.css`，写域外）。需共享改动，本轮未动。 |
| 文件/函数长度 | `ReaderPage.tsx` 改动前即 708 行（本次约 730），远超 400/70 的 TS 自检口径；本仓库门禁只统计 Go（`docs/quality-gates.md`）。拆分留待后续，不在本轮扩面。 |
| parity | 按 lead 口径由收口方统一跑；H5 不做数值 parity，本文件不声称 parity 通过。 |

## 7 需要的共享改动

- 无阻塞项。若要与契约数值完全对齐：`prototype.css` 抽屉宽 390→420px、H5 设置面板 70dvh→74%；由共享文件所有者统一改。
