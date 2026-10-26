# 发现域（discovery）功能盘点

> 范围：外壳（侧栏 · 顶栏 · 抽屉 · 提示条）· `/home` · `/shelf` · `/search` · `/series` · `/series/:name` · `/history`。
> 本文件是**域级摘录**，逐条列本域入口与证据；系统级索引见 [../00-features.md](../00-features.md)（只读）。
> 证据一律相对路径 `web/src/…:行号`，本轮逐条核对。平台 = web；H5 ≤768 / PC ≥1024（`web/src/lib/tier.ts:20`）。

## 1. 外壳（全局常驻）

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 站名回首页/书架 | 侧栏顶部 | `web/src/layouts/Sidebar.tsx:26` | H5 去 `/home`，PC 去 `/shelf` |
| 书架导航 + 书数 | 侧栏主导航 | `web/src/layouts/Sidebar.tsx:34` | 计数 0 时不显示 |
| 搜索导航 | 侧栏主导航 | `web/src/layouts/Sidebar.tsx:40` | 两档都有 |
| 首页导航（仅 H5） | 侧栏主导航 | `web/src/layouts/Sidebar.tsx:30` | PC 不出现 |
| 系列 / 阅读历史 / 书籍管理 / 驾驶舱 / 作者 / 导入管理 | 侧栏主导航 | `web/src/layouts/Sidebar.tsx:46`–`:72` | PC 专属，H5 不出现（`TierRoute.tsx:18`） |
| 继续阅读卡 | 侧栏中部 | `web/src/layouts/Sidebar.tsx:69` | 无进度时整块不出现 |
| 库存三项（本书/章节/字数） | 侧栏底部 | `web/src/layouts/Sidebar.tsx:88` | 读取中显示「—」 |
| 顶栏搜索框 | 顶栏中段 | `web/src/layouts/Topbar.tsx:34` | 首页不出现顶栏（`AppShell.tsx:52`） |
| 视图切换（网格/列表） | 顶栏右侧 | `web/src/layouts/Topbar.tsx:51` | 仅 PC 且路由是书架（`AppShell.tsx:18`） |
| 导航抽屉 + 遮罩 | H5 顶栏汉堡 | `web/src/layouts/AppShell.tsx:22`、`:43` | 路由一变即收起，Esc 关闭（`AppShell.tsx:36`） |
| 首页汉堡 | H5 首页 | `web/src/features/start/HomePage.tsx:32` | 首页无顶栏，靠它开抽屉 |
| 提示条 toast | 操作结果 | `web/src/lib/toastStore.ts:25` | 4 秒自动消失（`toastStore.ts:17` `TOAST_MS`） |
| 404 与出口 | 未匹配路径 | `web/src/layouts/Redirects.tsx:14` | 带「回书架」按钮 |
| 根路径落点 | `/` 或 `/landing` | `web/src/layouts/Redirects.tsx:6` | H5 → `/home`，PC → `/shelf` |

## 2. 页面

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 书架排序（6 种） | 书架工具栏 | `web/src/features/shelf/ShelfPage.tsx:32` | 最近更新/导入时间/书名/章节数/字数/评分 |
| 阅读状态 / 评分 / 作者 / 书单 / 大小分类筛选 | 书架工具栏 | `web/src/features/shelf/ShelfPage.tsx:372`–`:425` | PC 专属 |
| 标签 chips | 书架工具栏 | `web/src/features/shelf/ShelfPage.tsx:427` | 单选 |
| 清除筛选 | 书架工具栏 | `web/src/features/shelf/ShelfPage.tsx:444` | 无筛选时不出现 |
| 书卡 → 详情 | 书架网格/列表 | `web/src/features/shelf/ShelfPage.tsx:736` | 记住位置，回来高亮 1.5 秒（`:210`） |
| 阅读状态角标循环 | 书卡底部 | `web/src/features/shelf/ShelfPage.tsx:677` | 想读→在读→已读→未标记（`lib/readStatus.ts:12`） |
| 选择 + 批量设作者 | 书架页头 | `web/src/features/shelf/ShelfPage.tsx:320`、`:331` | 对话框 `BatchAuthorDialog.tsx:53` |
| 随机一本 | 书架页头 | `web/src/features/shelf/RandomBookButton.tsx:37` | PC 专属 |
| 继续阅读卡 | 书架页头下方 | `web/src/features/shelf/ResumeCard.tsx:18` | PC 专属 |
| 分页（每页 24） | 书架底部 | `web/src/features/shelf/ShelfPage.tsx:542` | PC 专属 |
| 无限滚动（每页 30） | 书架底部 | `web/src/features/shelf/ShelfPage.tsx:509` | H5 专属 |
| H5 排序底部弹层 | 书架页头 | `web/src/features/shelf/ShelfPage.tsx:566` | 含阅读状态段 |
| 书架三态 | 书架主区 | `web/src/features/shelf/ShelfPage.tsx:458`–`:490` | 加载/错误/空（空态 H5 不给「去导入」） |
| H5 首页继续阅读 + 最近在读 + 最近入库 | `/home` | `web/src/features/start/HomePage.tsx:48`、`:99`、`:127` | 仅 H5 |
| 首页搜索条 | `/home` | `web/src/features/start/HomePage.tsx:72` | 仅 H5 |
| 搜索结果两栏 + 分栏 | `/search` | `web/src/features/search/SearchPage.tsx:44`、`:67` | 分栏仅 PC |
| 书名命中 / 章节命中 | 搜索两栏 | `web/src/features/search/SearchPage.tsx:105`、`:131` | 章节命中直达该章 |
| 检索范围与片段口径 | 搜索页说明 | `web/src/lib/snippet.tsx:7` | 只检索书名/作者/章节标题；正文不参与，片段恒为空串 |
| 关键词 ≥2 字 | 搜索页 | `web/src/api/hooks.ts:80` | 不足 2 字不发请求 |
| 系列列表 / 系列详情 | `/series`、`/series/:name` | `web/src/features/series/SeriesPage.tsx:44`、`SeriesDetailPage.tsx:23` | PC 专属 |
| 阅读历史表格 | `/history` | `web/src/features/history/HistoryPage.tsx:52` | PC 专属，每页 20 本（`:7`） |

## 3. 本域不覆盖（明确划出去）

| 能力 | 归谁 | 原因 |
|---|---|---|
| 书籍详情页全部交互（含评分星标） | [library](../library/01-contract.md) | 评分只在详情页给，书架不给 |
| 阅读器全部交互 | [reader](../reader/01-contract.md) | 独立外壳 |
| 导入 / 书籍管理 / 单书管理 | [ops](../ops/01-contract.md) | PC 专属管理面 |
| 作者页 / 驾驶舱 | [library](../library/01-contract.md) | 决策页 |
