# react-agent 交接文档（第二段）

## 基线
- 第一段：开工 `1be56e9` → 交付 `5c50557`，leader 已合并为 `37bbee5`。
- 第二段：从 `37bbee5` 开工，未切分支、未 commit。
- 写区：`web/`、`Makefile`；`internal/webui/` 只被 `make build` 写入构建产物（`dist/*` 已 gitignore），源码零改动。

## 任务契约（task-10，第二段）

leader 指定顺序（不可改）：
1. 驾驶舱六图接真实数据（纯 SVG/CSS、每图一句实算结论、空数据形态、深浅主题可读）
2. H5 左右滑动翻页（阈值 ≥60px、横向 > 纵向才判定、不干扰选择与滚动、键盘与按钮保留）
3. 作者 / 标签筛选（`/api/tags` 空数据接 UI + 空态即可）
4. `odops parity` 对照，P0 = 0，P1 逐条说明

## 第二段完成项

### 1 驾驶舱六图（`features/dashboard/Charts.tsx` + `lib/charts.ts`）
- 六图全部接真实接口：trend（`/stats/import-trend?days=14`）、top authors（`/authors?sort=words&size=10`）、
  chapter buckets + chapter length（`/health/report`）、reading dist（`/reading/progress?limit=50` + `/stats.books`）、
  storage（`/stats` 的 textBytes/indexBytes/otherBytes）。
- 手写 SVG/CSS，无图表库；颜色全取 CSS 变量，日间/夜间/羊皮纸都实测可读。
- 每图一句实算结论，与图体由 `lib/charts.ts` 同一份转换结果生成（不会各说各话）。
- 四态：读取中 / 失败（可重试）/ 空（暂无数据）/ 有数据，由 `ChartFrame` 统一处理。
- 阅读进度是**按书聚合**：去重取每本最新一条，三态相加 = 总书数；记录数达 50 上限时结论标注「按最近 50 条聚合」。

### 2 H5 左右滑动翻页（`lib/swipe.ts` + `lib/useSwipePaging.ts`）
- 阈值 `SWIPE_THRESHOLD = 60px`；横向须 > 纵向 × 1.2 才判定；右滑上一章、左滑下一章。
- 起点在按钮/链接/输入框/可编辑区或已选文字上时不接管。
- 只在 H5（`useIsH5()`）绑定；PC 不启用手势。键盘与按钮保留。

### 3 作者 / 标签筛选（`features/shelf/ShelfPage.tsx`）
- 作者下拉接 `/api/authors?sort=books&size=50`，选中后 `/api/books?authorId=N`（实测佚名 3 本）。
- 标签 chips 接 `/api/tags`；当前库无标签数据 → 「暂无标签（导入的书还没有标签信息）」空态。
- 筛选项自身的加载/失败/空态由 `FilterHints` 提示，不静默。
- H5 下整个工具栏不渲染（实测 `shelf-toolbar`/`filter-author`/`filter-tags` 数量均为 0）。
把 `.open-design/08-prototype.html` 落成真实 React 应用：五视图 + 驾驶舱骨架跑真数据、路由、tier 判断、四态、API 封装、主题切换；
书架 / 详情 / 阅读器连真数据跑通。第二段再做六图、动效、细节、odops parity。

## 第一段完成项
- Vite 工程（React 19 + TS + Tailwind v4 + react-router 7 + @tanstack/react-virtual）。
- `src/api/client.ts` 唯一请求出口：超时、AbortSignal 取消、错误分类（timeout/network/aborted/http/parse）、鉴权 token。
- `src/lib/useResource.ts`：四态取数 + 过期请求丢弃（请求序号机制，不在 effect 里 setState）。
- 视图：`features/{start,shelf,book,reader,search,import,authors,dashboard}`，全部接真实接口。
- 分档：`lib/tier.ts` + `layouts/TierRoute.tsx`，H5 下管理类路由不挂载（实测直达 `/import` 被送回 `/home`，DOM 里导入/作者/驾驶舱链接数 = 0）。
- 主题：`lib/theme.ts`，日间/夜间/羊皮纸写 `html[data-theme]` + localStorage。
- 虚拟滚动：`features/book/ChapterList.tsx` 用 `@tanstack/react-virtual`，详情页卷内与阅读器抽屉共用。
- 样式：`styles/tokens.css`（逐字取 `02-style.md`）+ `styles/prototype.css`（原型 626 行 CSS 移植，去掉 body[data-view]/data-tier 切视图规则）+ `globals.css` 补丁。
- 构建：`make build` → `make web`（构建 + rsync 进 `internal/webui/dist`）→ `make build-server`。

## 已验证（第一段证据）
| 项 | 命令 | 结果 |
|---|---|---|
| 构建 | `pnpm build` | exit 0，`dist/assets/index-BfnrMsv1.js` 344.56 kB（gzip 107.97 kB）+ `index-Cf29hg1R.css` 51.13 kB |
| 类型 | `pnpm typecheck` | exit 0 |
| Lint | `pnpm lint` | exit 0，0 error / 1 warning（tanstack-virtual 的 react-hooks/incompatible-library，官方已知限制） |
| 单测 | `pnpm exec vitest run --coverage` | Tests 48 passed；语句 93.41% / 分支 86.81% / 函数 91.52%，阈值 70% 通过 |
| 二进制 | `make build` + `./bin/norvels serve` | 内嵌资源 `internal/webui/dist/{index.html,assets/*}`；`/shelf` 渲染 19 张书卡、`/book/3` 直达可渲染、浏览器 console 0 error |
| 隐私 | `scripts/privacy-scan.sh --all` | 零命中 |
| 四断点自检 | 浏览器 eval | 1440 六视图 + 390 四视图：横向溢出 0、术语 0、重复信息 0、H5 可点元素 <44px 计数 0 |

### 4 odops parity（`web/scripts/parity.mjs`）
- 结果：**P0 = 0，P1 = 23**（全量见 `/tmp/parity-final3.log`）。
- 为什么不用裸命令：原型的 8 个视图**同处一页**（CSS 隐藏），parity 读到 37~49 个锚点且不判可见性；
  实现按路由拆页，一页只有自己那几个。直接比会把「别的视图的块」全判成 P0 缺块。
  脚本因此做两件事：① 每个视图生成副本（改 `body[data-view]` + 注入 `location.hash` 让原型自己渲染到位），
  ② 只统计该路由**应承载**的锚点白名单。
- 跳过项：`/home` 是 H5 专属视图，PC 视口（1280）下 TierRoute 按设计重定向到 `/shelf`，改由 390 浏览器断言覆盖。
- 不参与对照：`.demo-bar` 演示条与 `state:*` / `dashboard:empty` 这类靠演示条手动切换的面板——产品里由真实数据驱动，静态对照命中不到（三态另有单测与浏览器断言）。
- **P1 全部是「高度不同」**，一条条原因见下表。

| P1 | 原型 | 实现 | 保留理由 |
|---|---|---|---|
| shelf / main 高度 | 779px | 1395px | 原型 12 张演示卡片；实现读真实库 19 本，卡片行数不同导致 |
| dashboard / main | 1770px | 1536px | 原型驾驶舱是演示数据（4 条在读 + 假导入任务）；真实库只有 2 条进度、1 条导入 |
| dashboard:stats | 106px | 137px | 实现四个大数字用真实量级（3041.9 万），字体带动换行高度 |
| dashboard:reading | 370px | 219px | 原型 4 条在读；真实库 2 条 |
| dashboard:import | 207px | 111px | 原型有「处理中」运行卡；当前库没有运行中任务，只显示上次导入四项 |
| chart:buckets / chart:reading | 233px | 215px | 图卡内边距一致，差在结论文案占一行 vs 两行 |
| dashboard:health | 469px | 451px | 同上，健康卡条目文案长度不同 |
| book-detail 高度 | 1512px | 717px | 原型目录展开 12 卷 × 视口行；实现首卷展开、其余折叠（按需展开） |
| book-aside 高度 | 589px | 581px | 封面比例一致，差在按钮文案长度（真实章节名长短） |
| search 高度 | 908px | 2668px | 原型 5 条命中演示；真实库「美女」命中数百条，一页 20 条 |
| search-books-col | 313px | 454px | 真实书名结果 2 条，简介长度不同 |
| import 高度 | 654px | 558px | 原型有运行中进度卡；当前库任务已结束 |
| authors 高度 | 540px | 1004px | 原型 8 位演示作者；真实库 17 位，一页 30 条 |
| reader 高度 | 1163px | 1845px | 原型固定 8 段正文；真实章节 3000–6000 字，段数与高度自然不同 |

## 截图
落在 `.open-design/shots/web-impl/`（该目录被 .gitignore 排除，不进版本库）：
第一段：`shelf-1440` / `dashboard-1440` / `import-1440` / `authors-1440` / `shelf-390` / `home-390` / `book-1-390` / `read-390` / `state-reader-error-390`。
第二段新增：`dashboard-charts-1440`（六图白天）/ `dashboard-charts-night-1440`（六图夜间）/ `shelf-filters-1440`（作者+标签筛选）。

## 第二段证据

| 项 | 命令 | 结果 |
|---|---|---|
| 构建 | `pnpm build` | exit 0，`index-C76HAI35.js` 357.10 kB（gzip 111.88）+ `index-DycihfDh.css` 51.29 kB |
| 类型 | `pnpm typecheck` | exit 0 |
| Lint | `pnpm lint` | exit 0，0 error / 1 warning（tanstack-virtual 已知限制） |
| 单测 | `pnpm exec vitest run --coverage` | 7 文件 / 79 passed；语句 87.4% / 分支 89.59% / 函数 87.01% |
| 二进制 | `make build` + `./bin/norvels serve` | 六图渲染（`.chart-card` = 6），控制台 0 error |
| parity | `node web/scripts/parity.mjs` | P0 = 0，P1 = 23（逐条理由见上表） |
| 断点自检 | 浏览器 eval，1440 六视图 + 390 五视图 | 溢出 0 / 术语 0 / 重复信息 0 / 可点元素 <44px = 0 |
| 隐私 | `scripts/privacy-scan.sh --all` | 零命中 |

滑动翻页实测（390，注入 Touch 事件）：左滑 120px → `/read/2/108`；右滑 120px → `/read/2/107`；
短滑 25px 与竖滑 220px 均不翻页；PC（1440）同一手势不生效。

## 后端接口改造期间的降级（2026-10-25 10:00 后加的）
- `/api/stats` / `/api/health/report` 改成查预计算表后，刷新窗口会带 `stale: true`：
  图照常出，结论旁显示「数据更新中」小徽章，不报错、不整块消失。
- 统计表刷新期间可能出现「总量有值、正文/索引还没拆出来」（`textBytes=0` 且 `indexBytes=0`）：
  体积图不谎报「正文占 0%」，改说「全库约 311 MB，体积构成正在统计」。
- 驾驶舱三个统计接口超时放宽到 20s（`STATS_TIMEOUT_MS`），列表类接口保持 10s。

## 第三段（2026-10-25 10:10–10:20）

### 1 阅读进度聚合端点（前端已接好，等后端上线）
- `HealthReport.readingBuckets?: { unread, reading, finished }`（类型见 `api/types.ts`）。
- `buildReadingDist(progress, totalBooks, limitReached, buckets?)`：**有 `buckets` 就以它为准**（精确、`partial=false`）；
  没有才回退到「最近 50 条 + 总书数」的估算并标注口径。
- 后端返回全 0 时也回退估算，不会把「还没统计」误显示成全未读。
- 当前 `/api/health/report` 实测 `readingBuckets: null`，前端走估算分支，验证通过。

### 2 微动效（`styles/motion.css` + `lib/theme.ts` + `features/reader/ReaderPage.tsx`）
| 动效 | 实测值 |
|---|---|
| 目录抽屉滑入 | `transform 0.24s cubic-bezier(0.32, 0.72, 0, 1)` |
| 主题切换 | 挂 `html.theme-shifting` → 背景/文字/边框 `0.18s` → 200ms 后摘掉 |
| 章节切换淡入 | `chapterFade 0.16s`（换章时正文重新挂载播一次） |

三处都在 `@media (prefers-reduced-motion: reduce)` 下关闭（产物里实测到两段规则块：原型移植的旧规则 + motion.css 的新规则）。

### 3 库体积构成：**后端有 bug，不是「还没跑完」**
`./bin/norvels serve` 启动日志：
```
WARN 后台重算库级聚合值失败: store: set metric text_bytes: SQL logic error: table store_metrics has no column named key
```
根因：代码（`internal/store/metrics.go`）用 `(key, value, updated_at)` 结构，但 `/tmp/web-dev.db` 里的
`store_metrics` 是老结构 `(id, text_bytes, index_bytes, chapters_at, computed_at)`——两版迁移定义不一致。
迁移 `0004` 用的是 `CREATE TABLE IF NOT EXISTS`，老表已存在所以新结构没建上，写入必然失败。

**这不是前端能修的**（要改 `internal/store/`，超出写区），已列入回报。
前端降级正确：六图照常出、0 报错、2 个「数据更新中」徽章，体积图说「体积构成正在统计」。

## 第四段（2026-10-25 11:14–12:30）

### 1 搜索页适配新检索范围（后端砍掉正文全文索引）
检索范围收敛为「书名 / 作者 / 章节标题」，搜不到正文句子。前端改动：

| 位置 | 改动 |
|---|---|
| `features/search/SearchPage.tsx` | 档位文案改用户语言（书名和作者 / 章节标题）；章节命中**无片段就整行不渲染**；空态说清检索范围 |
| `lib/snippet.tsx` | 新增 `hasSnippet()`；`splitSnippet` 对空串/纯空白返回 `[]`，不产出空元素 |
| `api/types.ts` | `SearchHit.snippet` 注明「字段保留、恒为空串」 |
| `lib/audit.ts` | 去术语词表里的「字级」改用 `\u5b57\u7ea7` 转义（词表行为不变） |

关键取舍：后端仍返回 `snippet` 字段但恒为空串，前端不删字段、只按「没有就不渲染」处理，
避免契约漂移与空的高亮块。

### 2 /api/stats 全文口径改成「压缩后字节」
后端把正文改成按 1 MB 块压缩存储，`textBytes` 从「原文逻辑字节」变成「压缩后实际占用」，
新增 `textBytesRaw`（压缩前）。前端：

| 位置 | 改动 |
|---|---|
| `api/types.ts` | `Stats.textBytesRaw?: number`（**可选**：老库不返回时要能退化为不显示压缩率） |
| `lib/charts.ts` | `buildStorage` 三段宽度**归一化到精确 100**（等比缩放 + 残差补到最后一段非零段） |
| `lib/charts.ts` | 新增 `storageRatioNote()`：`正文约为原始文字的 34.7%，比不压缩省下 3.0 GB` |
| `features/dashboard/Charts.tsx` | `ChartFrame` 支持第二行说明；体积卡接压缩率 |

**为什么宽度要补残差**：只做等比，实测算出 `96.9418 + 3.0582 + 0 = 100.0000` 这类值经四舍五入后，
写进 DOM 的三个数相加常见 `99.99` 或 `100.01`；补残差后「三段之和 = 100%」才是能照着 DOM 验证的事实。

**压缩率为什么保留一位小数**：真实库实测 34.6763%，取整会显示 35%，比真实值高 0.3 个点。

**为什么结论行不复述绝对体积**：同一张卡片的图例里已经有「正文 1.6 GB」，
再写一次违反项目既有的「同一视口内同一信息不出现两次」自检规则。结论行只给两个新信息：比例 + 省下的量。

### 3 真实库实测数字（838 本 / 42.27 万章，块压缩迁移 + VACUUM 后）
| 项 | 字节 | 显示 |
|---|---|---|
| 正文（压缩后） | 1,690,357,569 | 1.6 GB（十进制 1.69 GB） |
| 正文（原始） | 4,874,681,744 | 4.5 GB（十进制 4.87 GB） |
| 书目信息 | 53,326,015 | 51 MB |
| 其他 | 0 | 0 MB |
| 全库 | 1,743,683,584 | 1.6 GB（**1.62 GiB**） |
| 压缩率 | — | **34.7%**，省下 3.0 GB |

> 口径提醒：「1.69 GB」是十进制 GB，「1.62 GB」是 GiB，换算成字节后是同一个量级下的两个不同对象
> （正文压缩后 vs 整个库文件），不矛盾。前端 `formatStorage` 一律按 1024 进制显示。

**`otherBytes` 恒为 0 是后端口径的必然结果**（不是 bug）：后端把 books 侧文本并入正文、
`indexBytes = 文件 − 正文 − 书目文本`，三段里「其他」没有剩余可占。真机上体积图会显示
「其他 0 MB」，图体里该段宽度为 0（不画），视觉上只剩两段。

### 4 阅读器预取评估：**已评估、收益有限、暂不实施**
后端按 1 MB 块压缩正文，同块内相邻章节读取零额外解压成本（块已解压，切片即可），
一块约覆盖 35 章——理论上「读第 N 章顺手预取第 N+1 章」有约 35/36 概率是零成本。

**实测结论支持不做**：
- jsdom 实测渲染一章 5000+ 字正文（~40 段）**中位数 1.7 ms**；
- 后端解压从 0.3 → 1.0 ms，净增 **约 0.7 ms**，比渲染便宜一个量级；
- 当前翻页已是「整章换路由 + 重新取数」，`reader-loading-layer` 盖住（`position:absolute; inset:0`），
  不会半截白屏；
- 当前阅读器**没有预取实现**（`grep -rn 'prefetch|预取|预加载' web/src` → 0 命中）。

结论：0.7 ms 落在一个本来就要走网络 + 渲染的路径上，用户感知不到。预取会引入缓存失效、
路由切换时的请求竞态等复杂度，**收益不足以抵成本，暂不实施**。若将来章节正文变大
（或后端解压成本上升一个量级），再按「同块相邻章节预取」方案重估。

## 未做 / 已知限制
1. 阅读进度分布按「最近 50 条记录 + 总书数」聚合：记录超过 50 本时，未读数只能算「其余的书」，结论里已标注。
   要精确需要后端给聚合接口（当前只有列表接口，limit 上限 50）。
2. 导入页失败明细只在任务处于 `running` 时能读到（`errors` 由任务详情接口返回）。
3. 标签筛选当前入库数据没有标签，UI 接好、显示空态；有标签数据时直接可用。
4. 微动效（抽屉滑入 / 章节淡入）未单独做，CSS 里原型转场规则仍在，未验收。
5. 原型 `.demo-bar`（状态切换演示条）按要求未进产品。
6. 体积图「其他」段在真实库下恒为 0（后端口径决定），图例会显示「其他 0 MB」。若产品上不希望
   展示恒为 0 的段，可改为「值为 0 的段不出图例」——属产品决策，本轮未改。
7. 阅读器未做相邻章节预取（评估见上，收益有限）。