# 00 功能盘点（运营域 ops）

> 域范围：`/import` · `/admin` · `/admin/books/:bookId`。PC 专属，H5 下入口不出现。
> 证据三选一：源码 `路径:行号` · 接口 `METHOD /path` · URL。

| 功能 | 入口 | 证据 | 备注 |
|---|---|---|---|
| 导入管理 | 侧栏「导入管理」 | `web/src/routes.tsx:41` | PC 专属，H5 不渲染 |
| 目录扫描导入 | 导入页「开始导入」 | `web/src/features/import/ImportPage.tsx:109` | 并发 409 |
| 只预检 | 导入页「只预检」 | `web/src/features/import/ImportPage.tsx:133` | 同步扫描，上限 60 秒 |
| 浏览器上传 | 导入页上传区 | `web/src/features/import/UploadPanel.tsx:137` | 逐文件 accepted/rejected |
| 章节识别规则 | 导入页章节规则面板 | `web/src/features/import/ChapterRulesPanel.tsx:84` | 最多 20 条 |
| 标签词表规则 | 导入页词表规则面板 | `web/src/features/import/TagRulesPanel.tsx:82` | 最多 50 条 |
| 任务进度与取消 | 导入页进度卡 | `web/src/features/import/ImportPage.tsx:97` | 终态停止刷新 |
| 失败明细与重试 | 导入页失败清单 | `web/src/features/import/FailedFilesPanel.tsx:25` | 新建重试任务 |
| 任务历史 | 导入页任务历史 | `web/src/features/import/ImportPage.tsx:349` | 只列已结束任务 |
| 书籍管理列表 | 侧栏「书籍管理」 | `web/src/routes.tsx:58` | PC 专属 |
| 列表筛选与搜索 | 列表筛选条 | `web/src/features/admin/AdminBooksPage.tsx:80` | 关键词 / 标签 / 连载状态 |
| 多选与批量操作 | 列表批量条 | `web/src/features/admin/AdminBooksPage.tsx:149` | 上限 200 本 |
| 批量失败重试 | 批量弹层失败清单 | `web/src/features/admin/AdminBatchDialog.tsx:157` | 单本失败不整体回滚 |
| 标签管理 | 列表「标签管理…」 | `web/src/features/admin/TagManageDialog.tsx:74` | 删除只解绑不删书 |
| 单书管理 | 列表点书名 | `web/src/routes.tsx:66` | `/admin/books/:bookId` |
| 元信息编辑与保存 | 单书页书籍信息卡 | `web/src/features/admin/AdminMetaForm.tsx:8` | 草稿 + 吸底操作条 |
| 脏数据离开确认 | 单书页返回列表 | `web/src/features/admin/AdminBookPage.tsx:116` | 三条出口 |
| 封面上传与移除 | 单书页封面卡 | `web/src/features/admin/AdminCover.tsx:49` | 上限 2 MiB |
| 删除本书 | 单书页危险区 | `web/src/features/book/DeleteBookDialog.tsx:8` | 抄写书名解锁 |
| 章节表与行内改标题 | 单书页章节卡 | `web/src/features/admin/ChapterTable.tsx:26` | 按卷分组 |
| 新建章节 | 章节工具条 | `web/src/features/admin/AdminChapters.tsx:181` | 标题 + 插入位置 + 正文 |
| 正文抽屉编辑 | 章节行「正文」 | `web/src/features/admin/ChapterBodyDrawer.tsx:23` | 未保存拦截 |
| 章节拆分 | 章节行「拆分」 | `web/src/features/admin/SplitChapterDialog.tsx:22` | 只在段落边界切 |
| 章节合并 | 批量条「合并」 | `web/src/features/admin/ChapterMergeDialog.tsx:15` | 只合同卷相邻 |
| 按序号重命名 | 章节工具条 | `web/src/features/admin/ChapterRenumberDialog.tsx:13` | 当前卷 / 全书 |
| 批量删除章节 | 批量条「删除所选」 | `web/src/features/admin/ChapterBatchDeleteDialog.tsx:12` | 二次确认 |
| 卷管理 | 章节工具条「卷管理…」 | `web/src/features/admin/VolumeManageDialog.tsx:23` | 新建/改名/排序/删除 |
| 移动到卷 | 批量条「移动到卷…」 | `web/src/features/admin/VolumeMoveDialog.tsx:22` | 含未分卷 |
| 章节分批加载 | 每卷「载入更多」 | `web/src/features/admin/ChapterTable.tsx:188` | 每页 20 章 |
| H5 分档闸门 | 窄屏访问管理路由 | `web/src/layouts/TierRoute.tsx:14` | 不渲染，跳回首页 |
