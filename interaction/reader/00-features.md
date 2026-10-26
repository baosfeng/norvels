# 阅读域功能盘点（`/read/:bookId/:chapterId`）

> 域级盘点：只覆盖阅读器，供 `ixcheck` 在本目录内取证；系统级盘点见 [../00-features.md](../00-features.md)（**只读，由 lead 维护**）。
> 证据口径：源码 `路径:行号` / 接口 `METHOD /path`。平台 = web，H5 ≤768px、PC ≥1024px（`web/src/lib/tier.ts:20`）。

## 1 入口与外壳

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 阅读器路由 | `/read/:bookId/:chapterId` | `web/src/routes.tsx:78` | 独立外壳，无侧栏 |
| 从详情页进入 | 「继续阅读」/ 章节行 | `web/src/features/book/BookPage.tsx:111` | 带上次读到的段落 |
| 从书架继续卡进入 | 侧栏继续阅读卡 | `web/src/layouts/Sidebar.tsx:62` | 无进度时不出现 |
| 从搜索章节进入 | 搜索结果章节命中 | `web/src/features/search/SearchPage.tsx` | 直达该章 |
| 分档判定 | `useTier()` → `H5_QUERY` | `web/src/lib/tier.ts:20` | ≤768 走 H5 信息架构 |

## 2 阅读与翻章

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 正文分段 | `\n` 逐段渲染，段落下标即位置锚 | `web/src/features/reader/ReaderPage.tsx:120` | 空行占位不挪下标 |
| 顶栏上一章 / 下一章 | 顶栏与章末按钮 | `web/src/features/reader/ReaderPage.tsx:578` | 首末章边界 |
| H5 底栏翻章 | 底栏格 | `web/src/features/reader/ReaderBottomBar.tsx:91` | 每格 ≥48px |
| 键盘 ← / → | 全局键盘 | `web/src/features/reader/ReaderPage.tsx:349` | 输入框内不接管 |
| 键盘空格 / PageDown / PageUp | 全局键盘 | `web/src/features/reader/ReaderPage.tsx:352` | 0.85 视口高 |
| H5 左右滑动翻章 | 正文区手势 | `web/src/lib/swipe.ts:26` | 阈值 60px、横向 >1.2 倍 |

## 3 控制层显隐

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 单击唤出 / 收起 | H5 正文单击 | `web/src/lib/useReaderControls.ts:74` | 位移 ≤12px、按住 ≤700ms |
| 3s 无操作自动收起 | H5 | `web/src/lib/useReaderControls.ts:10` | `AUTO_HIDE_MS` |
| 面板打开时暂停收起 | H5 | `web/src/lib/useReaderControls.ts:59` | `paused` 参数 |
| PC 恒显示 | PC | `web/src/lib/useReaderControls.ts:117` | `isH5 ? visible : true` |

## 4 目录 / 书签 / 搜索

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 目录抽屉 | 顶栏「目录」/ 底栏 | `web/src/features/reader/TocDrawer.tsx:59` | 打开即定位当前章 |
| 目录搜索 | 抽屉内输入框 | `web/src/features/reader/TocDrawer.tsx:84`（输入框）、`web/src/lib/tocRows.ts:53`（筛选） | 章节名或章号 |
| 长目录分块 | 章节列表 | `web/src/features/book/ChapterList.tsx:97` | 1000+ 章分段渲染 |
| 书签抽屉 | 顶栏「书签」/ 底栏 | `web/src/features/reader/BookmarksDrawer.tsx:39` | 三态齐备（加载 / 空 / 错误） |
| 加书签（同段幂等） | 抽屉内按钮 | `web/src/features/reader/BookmarksDrawer.tsx:56`、`ReaderPage.tsx:278`（同段判定） | 提示「这个位置已经有书签了。」 |
| 章内搜索 | 顶栏「搜索」/ 底栏 | `web/src/features/reader/ReaderSearchPanel.tsx:47` | 第 n / N 处，上限 200 |

## 5 阅读设置

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 设置面板 | 顶栏「设置」/ 底栏 | `web/src/features/reader/ReaderSettingsPanel.tsx:82` | PC 浮层 / H5 底部面板 |
| 主题三档 + 跟随系统 | 面板「主题」 | `web/src/lib/theme.ts` | 只重绑语义色 |
| 字号 / 行距 / 段距 / 字体 / 宽度 / 对齐 | 面板各行 | `web/src/features/reader/readerSettings.ts:83` | 即时生效并记住 |
| 首行缩进 / 繁转简 | 面板开关 | `web/src/features/reader/readerSettings.ts:87` | 繁转简长度不变，下标不挪 |
| 自动滚动 + 速度 + 到底下一章 | 面板开关与档位 | `web/src/lib/autoScroll.ts` | 面板打开即暂停 |
| 快捷键帮助 | 顶栏「?」或 `?` 键 | `web/src/features/reader/ReaderHelpPanel.tsx:32` | 键盘与触摸两段 |

## 6 状态与记账

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 正文三态 | 正文区 | `web/src/components/StatePanel.tsx:17` | 加载 / 空 / 错误 |
| 章内位置恢复 | 进入章节 | `web/src/features/reader/ReaderPage.tsx:399` | 按段落下标回滚 |
| 阅读进度上报 | 滚动停 1s / 切章 / 页面隐藏 | `web/src/features/reader/ReaderPage.tsx:190` | 段落维度 |
| 阅读时长 | 页面可见时累计 | `web/src/lib/readingTimer.ts:16` | ≥60s 才记一次 |
| 提示条 | 各操作 | `web/src/lib/toastStore.ts:25` | 4s 自动消失 |
| 全屏 | H5 顶栏 | `web/src/features/reader/ReaderPage.tsx:93` | 不支持时降级为收起控制层 |

## 6.5 契约里声明为「新增」的能力（产品当前无实现）

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 进度恢复提示条「已回到上次读到的地方」+「知道了」 | 进入章节时 | 新增 · 暂无实现 | `grep -rn '已回到上次读到的地方' web/src` → 0；`grep -rn '知道了' web/src/features/reader` → 0。评审 P0-4 整改项 |
| 目录按卷分组折叠 | 目录抽屉卷头 | 新增 · 暂无实现 | `grep -rn '卷折叠\|vol-body' web/src` → 0；现有目录是平铺列表（`TocDrawer.tsx:118` 直接把扁平数组交给 ChapterList） |
| 目录「再显示 60 章」分块续显 | 目录列表底部 | 新增 · 暂无实现 | `grep -rn '再显示\|已显示' web/src` → 0；现有靠列表自身的分段加载 |

## 7 接口面（阅读器依赖）

| 能力 | 接口 | 证据 | 备注 |
|---|---|---|---|
| 书籍详情 + 目录 | `GET /api/books/{id}` | `docs/api/api-books.md` | 含卷与章 |
| 章节正文 | `GET /api/books/{id}/chapters/{cid}` | `docs/api/api-books.md` | 含 prevId/nextId |
| 阅读进度 | `PUT /api/reading/progress` | `docs/api/api-p3.md` | 含章内段落下标 |
| 阅读时长 | `POST /api/reading/sessions` | `docs/api/api-p3.md` | 1–86400 秒 |
| 书签 | `/api/books/{id}/bookmarks` | `docs/api/api-p3.md` | 按位置幂等 |
