# 00 功能盘点 · 书目域（`/book/:bookId` · `/authors` · `/dashboard`）

> 本域范围的功能盘点与证据。全系统盘点见 [../00-features.md](../00-features.md)（由 lead 维护，本文件不重复其内容）。
> 证据三选一：源码 `路径:行号` · 接口 `METHOD /path` · URL。凡是本域独有的入口都逐条列出。

## 1 书籍详情 `/book/:bookId`

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 进入详情 | 书架书卡 / 搜索结果 / 系列详情 | `web/src/routes.tsx:38` | AppShell 内，非独立外壳 |
| 面包屑返回书架 | 页面左上「书架 /」 | `web/src/features/book/BookPage.tsx:116` | 有历史走 back，否则回 /shelf |
| 封面（含占位） | 左侧封面区 | `web/src/components/Cover.tsx:32` | 无封面取书名首字 |
| 继续阅读 | 左栏主按钮 | `BookPage.tsx:134` | 有进度才渲染 |
| 从第一章开始 | 左栏次按钮 | `BookPage.tsx:139` | 有章节才渲染 |
| 加入书单 | 左栏幽灵按钮 | `BookPage.tsx:144` | 开 `AddToCollectionPanel.tsx:12` |
| 管理这本书（仅 PC） | 左栏幽灵链接 | `BookPage.tsx:153` | H5 条件不渲染 |
| 阅读状态 | 信息栏下拉四态 | `BookPage.tsx:182` · `lib/readStatus.ts:15` | 未标记/想读/在读/已读 |
| 评分星标 | 信息栏 ☆×5 | `RatingStars.tsx:13` | 点当前颗清零（`:36`） |
| 系列入口 | 信息栏「系列」行 | `BookPage.tsx:168` | 现为纯文本，不可点 |
| 标签 | 副标题下 chips | `BookPage.tsx:247` | 现为静态 span，不可点 |
| 简介展开 | 简介下方按钮（仅 H5） | `BookPage.tsx:267` | PC 全文展开 |
| 追更角标 | 作者行「新章 +n」 | `BookPage.tsx:237` · `newChapters.ts:22` | 读到底清零 |
| 目录搜索 | 目录标题下输入框 | 契约新增位（`BookPage.tsx:282` 之后） | 现状无搜索 |
| 卷跳转 | 目录上方 chips | `BookPage.tsx:303` | 单卷时不渲染 |
| 卷折叠 | 卷头 | `BookPage.tsx:321` · `toggleVolume:101` | 默认展开有进度的卷 |
| 章节行 | 卷内网格 | `ChapterList.tsx:125` | 多列 PC / 单列 H5 |
| 长目录分块 | 卷内「再显示 60 章」 | 契约新增位（`ChapterList.tsx:22` 无界行上限） | 现状整卷铺开 |
| 删除本书 | **不在本页**，在 `/admin/books/:id` | `AdminBookPage.tsx:253` · `DeleteBookDialog.tsx:22` | 抄书名才可执行，不可恢复 |
| 三态面板 | 加载 / 错误 / 无此书 | `BookPage.tsx:39` `:42` `:45` | 统一走 StatePanel |

## 2 作者 `/authors`（仅 PC）

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 进入作者页 | 侧栏「作者」 | `web/src/layouts/Sidebar.tsx:55` · `routes.tsx:49` | H5 走 TierRoute 重定向 |
| 作者计数 | 页头 eyebrow | `AuthorsPage.tsx:20` | 读取中显示「读取中」 |
| 排序 | 工具栏下拉三选 | `AuthorsPage.tsx:29` | 书籍数 / 总字数 / 名字 |
| 作者表 | 三列表格 | `AuthorsPage.tsx:52` | 作者 / 作品 / 总字数 |
| 作者名 | 表格首列 | `AuthorsPage.tsx:64` | 现为纯文本，无作品页 |
| 分页 | 底部 ← 页码 / → | `AuthorsPage.tsx:75` | 一页 30 条，1 页时整条不渲染 |
| 加载 / 错误 / 空 | 表格位置 | `AuthorsPage.tsx:45` `:47` `:49` | 三态面板 |
| 分页数据 | 作者列表接口 | `GET /api/authors?sort=&page=&size=` | 实测 486 → 96 条 |

## 3 驾驶舱 `/dashboard`（仅 PC）

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 进入驾驶舱 | 侧栏「驾驶舱」 | `Sidebar.tsx:52` · `routes.tsx:56` | PC 专属 |
| 统计卡 | 主区四张卡 | `DashboardPage.tsx:89` | 书 / 章 / 作者 / 字数 |
| 最近在读 | 主区列表 | `DashboardPage.tsx:96` | 点击进阅读器 |
| 全部书架入口 | 「最近在读」右上 | `DashboardPage.tsx:99` | 跳 `/shelf` |
| 导入概况 | 主区四格 | `DashboardPage.tsx:131` | 上次导入时间与结果 |
| 前往导入管理 | 「导入概况」右上 | `DashboardPage.tsx:134` | 跳 `/import` |
| 阅读时长 + 热力图 | 图表区第 1 卡 | `ReadingHeatmap.tsx:22` | 91 天，5 档 |
| 热力格读数 | 格子悬停 | `ReadingHeatmap.tsx:132` | 现仅原生 title |
| 导入趋势 | 图表区 | `Charts.tsx:81` | 14 天竖条 |
| 字数 Top 10 | 图表区 | `Charts.tsx:117` | 横条排行 |
| 章节数分布 | 图表区 | `Charts.tsx:99` | 六档直方图 |
| 阅读进度分布 | 图表区 | `Charts.tsx:143` | 环形图 |
| 图例过滤 | 环形图图例 | `Charts.tsx:181` | 现不可点 |
| 库体积构成 | 图表区 | `Charts.tsx:195` | 三段堆叠 |
| 章节长度分布 | 图表区 | `Charts.tsx:236` | 峰值加深 |
| 图表重试 | 失败态按钮 | `Charts.tsx:64` | 每卡独立 |
| 健康卡 | 右栏四行 | `DashboardPage.tsx:172` | 章节完整 / 待补 / 超长 / 最近更新 |
| 去书籍管理 / 去导入 | 健康卡底部 | `DashboardPage.tsx:183`（契约新增位） | 现状无下一步入口 |
| 加载 / 空 / 错误 | 整页 | `DashboardPage.tsx:72` `:76` `:74` | 空态带「去导入」 |
| 统计接口 | 库总览 | `GET /api/stats` | 实测 books/chapters/authors/words/dbBytes |
| 健康接口 | 完整性 | `GET /api/health/report` | 实测 totalBooks/missingIntro/chapterBuckets |
| 阅读时长接口 | 热力图数据 | `GET /api/stats/reading?days=` | 实测 days[{date,seconds}] |
| 导入趋势接口 | 趋势图 | `GET /api/stats/import-trend?days=` | 实测 [{date,books,chapters}] |
