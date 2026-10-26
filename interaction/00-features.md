# 00 功能盘点（norvels · 本地中文小说库）

> 交互设计的**事实源**：先盘清系统有什么、入口在哪、证据在哪，再谈怎么交互。
> 证据三选一：源码 `路径:行号` · 接口 `METHOD /path` · URL。拿不到证据的行标 `[待核实]`。
> 逐域契约与原型在 `interaction/<域>/`，索引见 [README](README.md)。

## 1. 平台与分档

| 项 | 事实 | 证据 |
|---|---|---|
| 产品形态 | 单二进制 + 内嵌 React SPA，浏览器访问 | `README.md` · `internal/webui/` |
| 目标平台 | **web**（响应式，一套代码两档信息架构） | `web/src/lib/tier.ts:20` |
| 分档阈值 | H5 ≤768px，PC ≥1024px（768–1024 归 PC） | `web/src/lib/tier.ts:20` `H5_QUERY` |
| 分档方式 | 条件渲染：窄屏节点**不出现**在文档里，不是 CSS 隐藏 | `web/src/layouts/TierRoute.tsx:14` |
| 窄屏不出现的入口 | 导入 / 作者 / 驾驶舱 / 书籍管理 | `web/src/lib/tier.ts:16` `MANAGEMENT_ROUTES` |
| 主题 | 日间 / 夜间 / 羊皮纸（阅读器），跟随系统 | `web/src/lib/theme.ts` |
| 语言 | 简体中文 | `web/index.html` |

## 2. 页面与入口

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 书架（首页） | 侧栏「书架」/ 根路径重定向 | `web/src/routes.tsx:31` | H5 无管理入口 |
| H5 首页 | 侧栏「首页」（仅窄屏） | `web/src/routes.tsx:24` | 继续阅读 + 轻量目录 |
| 书籍详情 | 点书卡 / 搜索结果 | `web/src/routes.tsx:32` | `/book/:bookId` |
| 阅读器 | 详情页「继续阅读」/ 侧栏继续卡片 / 搜索章节命中 | `web/src/routes.tsx:78` | 独立外壳，无侧栏 |
| 搜索 | 侧栏「搜索」/ 顶栏搜索框 | `web/src/routes.tsx:33` | 书名 + 章节两栏 |
| 系列列表 | 侧栏「系列」 | `web/src/routes.tsx:60` | PC 专属 |
| 系列详情 | 系列列表点某系列 | `web/src/routes.tsx:64` | `/series/:name` |
| 阅读历史 | 侧栏「阅读历史」 | `web/src/routes.tsx:68` | PC 专属 |
| 导入管理 | 侧栏「导入管理」 | `web/src/routes.tsx:36` | PC 专属 |
| 书籍管理（列表） | 侧栏「书籍管理」 | `web/src/routes.tsx:48` | PC 专属 |
| 单书管理 | 列表点书名 / 详情页编辑入口 | `web/src/routes.tsx:52` | `/admin/books/:bookId` |
| 驾驶舱 | 侧栏「驾驶舱」 | `web/src/routes.tsx:56` | PC 专属 |
| 作者 | 侧栏「作者」 | `web/src/routes.tsx:40` | PC 专属 |
| 404 | 未匹配路径 | `web/src/routes.tsx:73` | 带出口按钮 |

## 3. 跨页公共能力

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 侧栏导航（含计数） | 全局左栏 | `web/src/layouts/Sidebar.tsx:20` | H5 变汉堡抽屉 |
| 继续阅读卡（侧栏） | 全局左栏 | `web/src/layouts/Sidebar.tsx:62` | 无进度时不出现 |
| 顶栏（搜索 + 视图切换） | 全局顶部 | `web/src/layouts/Topbar.tsx` | 首页不显示顶栏 |
| 视图切换（网格/列表） | 顶栏右侧 | `web/src/layouts/AppShell.tsx:18` | 仅 PC 书架出现 |
| 导航抽屉 + 遮罩 | H5 顶栏汉堡 | `web/src/layouts/AppShell.tsx:22` | 路由一变即收起 |
| 提示条（toast） | 操作结果触发 | `web/src/lib/toastStore.ts:25` | 4 秒自动消失 |
| 三态面板（加载/空/错误） | 各列表与详情 | `web/src/components/StatePanel.tsx:17` | 统一结构 |
| 封面占位 | 无封面时 | `web/src/components/Cover.tsx` | 取书名首字 |
| 主题持久化 | 阅读器设置 | `web/src/lib/theme.ts` | 存本机 |

## 4. 接口面（交互依赖的部分）

| 能力 | 接口 | 证据 | 备注 |
|---|---|---|---|
| 书籍列表与筛选 | `GET /api/books` | `docs/api/api-books.md` | q/authorId/tag/series/rating/collection/sort |
| 书籍详情 + 目录 | `GET /api/books/{id}` | `docs/api/api-books.md` | 含卷与章 |
| 章节正文 | `GET /api/books/{id}/chapters/{cid}` | `docs/api/api-books.md` | 含 prevId/nextId |
| 检索 | `GET /api/search` | `docs/api/README.md` | 书名/作者/章节标题，≥2 字 |
| 阅读进度 | `PUT /api/reading/progress` | `docs/api/api-p3.md` | 含章内段落下标 |
| 阅读时长 | `POST /api/reading/sessions` | `docs/api/api-p3.md` | 1–86400 秒 |
| 书签 | `/api/books/{id}/bookmarks` | `docs/api/api-p3.md` | 按位置幂等 |
| 导入任务 | `POST /api/import` | `docs/api/api-import.md` | 202 + jobId，并发 409 |
| 上传 | `POST /api/import/upload` | `docs/api/api-import.md` | 逐文件 accepted/rejected |
| 元信息批量 | `POST /api/books/batch` | `docs/api/api-p3.md` | ≤200 本 |
| 分类聚合 | `GET /api/categories` | `docs/api/api-category.md` | 两级分类 |
| 图表数据 | `GET /api/stats/reading` | `docs/api/api-dashboard.md` | days/tz |

## 5. 域划分（契约与原型索引）

| 域 | 范围 | 契约 | 原型 | 规模 |
|---|---|---|---|---|
| 阅读 | `/read/:bookId/:chapterId` | [reader/01-contract.md](reader/01-contract.md) | [reader/02-prototype.html](reader/02-prototype.html) | 阅读是产品核心 |
| 发现 | 外壳 + 书架 + 首页 + 搜索 + 系列 + 历史 | [discovery/01-contract.md](discovery/01-contract.md) | [discovery/02-prototype.html](discovery/02-prototype.html) | 最常用路径 |
| 书目 | 详情 + 作者 + 驾驶舱 | [library/01-contract.md](library/01-contract.md) | [library/02-prototype.html](library/02-prototype.html) | 决策页 |
| 运营 | 导入 + 书籍管理 + 单书管理 | [ops/01-contract.md](ops/01-contract.md) | [ops/02-prototype.html](ops/02-prototype.html) | PC 专属 |
