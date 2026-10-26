# 评审 · 发现域交互设计（discovery）

> 评审对象：`docs/ux/discovery.md`（111 行）· `.open-design/ux/discovery.html` · `shots/discovery-1440.jpg`(1440×900) · `shots/discovery-390.jpg`(390×844)。
> 判据：`docs/ux/README.md` §6 的 12 条（P0 8 / P1 4）。取证会话：`AGENT_BROWSER_SESSION=norvels-ux-review-discovery`。
> 结论：**P0 4 / P1 3 / 12 条过 7**。只出证据与结论，不判定最终通过。

## 一、12 条判据逐条

| # | 级别 | 结论 | 证据 |
|---|---|---|---|
| 1 | P0 | **不过** | 锚点 93=93 无缺，但源码可交互元素 93，文档只覆盖 `home:recent` 一个区块，首页「全部书架 →」`HomePage.tsx:92`、「全部 →`HomePage.tsx:117`、`h5-row` 两类行链接（:99/:128）与「最近入库」整块（:114-139）无行 |
| 2 | P0 | 过 | 97 行「触发」「结果」列均非空：`python3 统计 → 触发: 空 0 行 / 结果: 空 0 行` |
| 3 | P0 | 过 | `odops verify` exit 0（11 区块带锚点 / 响应式 1440·1024·768·390 齐 / 无裸色值）；`scrollWidth==clientWidth` 于 1440 与 390 均成立 |
| 4 | P0 | **不过** | 书架（加载/空/错误/追加中/追加失败/到底）、搜索（未输入/过短/加载/空/错误）齐全；首页「最近入库」缺加载与错误态——源码 `HomePage.tsx:121-124` 有 `LoadingPanel`/`ErrorPanel`，原型 `home` 视图只有 `home-empty` 一个 state 块（实测 `blocks:["home-empty:hidden"]`） |
| 5 | P0 | **不过** | 锚点层面双向 0 差异（见第二节），但 H5/PC「不出现」在原型里未演示：H5 下控制条仍可切出系列页（实测 `seriesW=358 seriesH=463 visibleInH5=true`）；PC 下可切出首页整页（`PC切home后 home:page 可见=true`）。源码 `routes.tsx:85/94/101` 走 `TierRoute` 重定向到 `/home`，`:32` 到 `/shelf` |
| 6 | P0 | 过 | `grep -nE 'SQLite|FTS|API|bookId|localStorage|虚拟滚动|SHA-256|用时'` → 文档 0 命中、原型 0 命中 |
| 7 | P0 | 过 | 全部书名由 `PRE`/`SUF` 数组组合生成、作者为 `AUTHORS` 虚构串；上述虚构词在 `.privacy-denylist.txt` 均 0 命中 |
| 8 | P0 | **不过** | H5 首页 `h5-section-head a.meta`（「全部书架 →」「全部 →」）实测 21px 高 < 44；源码有对应规则 `globals.css:112-121`，原型缺 `.h5-section-head .meta{min-height:44px}` |
| 9 | P1 | **不过** | 五态覆盖偏科：97 行里「悬停」28、「聚焦」31、「选中」20，但「按下」仅 11；65 行 PC/两档元素中 62 行状态列无「按下」 |
| 10 | P1 | 过 | 破坏性操作仅「批量设作者」一处：确认按钮文案随内容变为 `清空 N 本作者`（`BatchAuthorDialog.tsx:128`），运行中取消禁用（:130），文档 L52 写全二次确认与失败重试 |
| 11 | P1 | 过 | 无限滚动的非手势替代：`ShelfPage.tsx:529-533` 无 `IntersectionObserver` 时渲染「加载更多」按钮，文档 L68 写明仅老浏览器出现；抽屉/弹层 Esc 替代路径 L24/L36 已写 |
| 12 | P1 | 过 | 空态均带下一步：书架 L61（清除筛选 / 去导入）、搜索 L81-83（补字 / 换说法）、系列 L91、历史 L102、404 L30（回书架）；错误态全部给「重试」 |

## 二、必做复核（leader 抽检点）

**① 锚点双向对表** — 命令与计数：

```
python3: da=set(re.findall(r'`([a-z0-9-]+:[a-z0-9-]+)`', doc)); pa=set(re.findall(r'data-od-id="([^"]+)"', proto))
→ doc 93 / proto 93 / diff 0
→ 文档复用锚点：shelf:page×2、shelf:card×2、search:too-short×3
grep -c 'data-od-id' .open-design/ux/discovery.html → 95（2 处重复渲染，unique 93）
```

反向抽查 10 个原型锚点是否都进文档：`demo:bar` `shell:drawer-scrim` `shelf:card-rating` `shelf:more-load-more` `search:pagination` `series-detail:crumbs` `history:row` `shell:notfound-back` `home:search` `shelf:sort-sheet` → **10/10 命中**，复用的 3 个锚点各段语义确实不同（加载态/卡片两处渲染/搜索三种空态）。

**② 源码行号抽查 ≥10（实际全量 84 条）**：67 条在 ±2 行内；**13 条错行，全部在 Sidebar.tsx**（文档用的是删掉 H5 首页入口之前的行号）：

| 锚点 | 文档 | 正确 | 锚点 | 文档 | 正确 |
|---|---|---|---|---|---|
| `shell:brand` | :28 | :23 | `shell:nav-history` | :55 | :46 |
| `shell:nav-home` | :36 | :30 | `shell:nav-admin` | :58 | :49 |
| `shell:nav-shelf` | :42 | :34 | `shell:nav-dashboard` | :61 | :52 |
| `shell:nav-count` | :44 | :36 | `shell:nav-authors` | :64 | :55 |
| `shell:nav-search` | :48 | :38 | `shell:nav-import` | :73 | :59 |
| `shell:nav-series` | :52 | :43 | `shell:continue-card` | :80 | :69 |
| `shell:side-stats` | :94 | :81 | | | |

另 1 条：`shell:topbar` 文档 :26（是 `submit` 的闭括号），正确 `:29 <header className="topbar" data-od-id="topbar">`。其余全对，抽样：`shell:nav-toggle` Topbar:31 ✓、`shell:drawer-scrim` AppShell:42（文档 :44 是 `type="button"`，差 2 行）、`shelf:sort-trigger` ShelfPage:340（文档 :341 是 `className`，差 1）、`search:hit-snippet` SearchPage:138 ✓、`history:table` HistoryPage:52 ✓。

## 三、P0 缺陷表

| # | 对象 | 问题 | 证据 | 建议改法 |
|---|---|---|---|---|
| P0-1 | 原型 · 分档 | H5/PC「不出现」未演示：H5 下控制条能切出系列/系列详情/历史整页；PC 下能切出首页整页。这是全文档最核心的分档声明（L15-20、L87、L98、L108），原型恰好不演示它 | H5: `{"tier":"h5","view":"series","seriesW":358,"seriesH":463,"visibleInH5":true}`；PC: `"PC切home后 home:page 可见=true"`；源码 `routes.tsx:82-105`+`TierRoute.tsx:18-23` 是 `Navigate` 重定向 | 控制条切到不存在的档位时，渲染一个「该页在 H5 不出现 → 落到首页」的落点演示态，或直接禁用该按键并给出同一句话 |
| P0-2 | 文档 L79 / 原型 :760 | 搜索片段口径对不上：文档写「命中词加粗显示」，原型 :760 的 `search:hit-snippet` 演示的是「没有可显示的命中片段，结果只列书名与章节标题」；源码 `snippet.tsx:7-8` 注释「检索范围收敛为书名/作者/章节标题，后端仍返回 snippet 但恒为空串」 | 文档 L79「命中词加粗显示」vs 原型 `search-hit-snippet-note` 文案 vs `snippet.tsx:12`「空串与纯空白都算没有片段」 | 文档改成「片段只在检索范围含正文时出现；当前范围只到标题，整行不出现」，并在「新增」栏注明扩到正文的代价 |
| P0-3 | 文档 · 首页 | 「最近入库」整块与 3 个可交互元素无行：`HomePage.tsx:92`「全部书架 →」、`:117`「全部 →」、`:99/:128` 两类行链接；`data-od-id` 在首页只有 `home`(29)、`home:resume`(48) 两处，`recent` 区块没有任何锚点 | `grep -c 'data-od-id' HomePage.tsx → 2`（第 29、48 行）；原型 `home` 视图同样无该区块锚点 | 补 `home:recent-all` / `home:recent-row` 两行（含「全部 →」与行链接），并把「最近入库」的加载/错误态写进 L54 的状态列 |
| P0-4 | 原型 · 触控 | H5 首页两个 `meta` 链接实测 21px 高，低于 44：`{"t":"全部书架 →","h":21}`、`{"t":"全部 →","h":21}`。源码是对的，原型没照抄这条规则 | 实测 H5 390 视口；源码 `globals.css:112-121` `.h5-section-head a.meta{min-height:44px}`（在 `@media (max-width:768px)` 内） | 原型补 `body[data-tier="h5"] .h5-section-head .meta{min-height:44px;display:inline-flex;align-items:center}` |

## 四、P1 与观察项

| 级别 | 位置 | 现状 → 期望 | 证据 |
|---|---|---|---|
| P1 | 文档 · 状态列 | PC 元素「按下」态大面积缺失（65 行里 62 行无）→ 逐个补 | 关键词统计：按下 11/97，悬停 28，聚焦 31 |
| P1 | 文档 L86 搜索分页 | 标了「新增」但没写实现代价（只写「源码当前一次列全部命中」）→ 补一句代价与不做的后果 | `grep -n '实现代价|代价' docs/ux/discovery.md` → 无命中；`grep -nE 'pagination|page' SearchPage.tsx` → 无分页实现 |
| P1 | 文档 L48 `shelf:card-rating` | 仍为「新增」，但 leader 已判删除；源码书架无 `RatingStars`（`grep -n 'RatingStars' ShelfPage.tsx` → 无）→ 待 leader 拍板删行还是保留 | 该行源码列写「(详情页同款 RatingStars)」，无文件:行号 |

观察项（P2，不进修复轮）：文档 L70 与 L47 复用 `shelf:card` 锚点表达「滚动恢复」，原型侧靠 JS 行为演示而非独立锚点，可接受但不够自解释；`docs/ux/review-ops.md:6` 写入了本机绝对路径，隐私扫描 exit 1（`bash scripts/privacy-scan.sh --all` → `[命中] 本机绝对路径（含用户名）`），不在本域 scope，请 leader 转派。

## 五、本域额外核查（6 项）

1. **外壳覆盖度**：源码可交互元素 93（16 个文件 `<(button|NavLink|Link|a|select|input|textarea)` 统计），文档 97 行 / 93 锚点，外壳 23 行覆盖 Sidebar 11 个可交互元素 + Topbar 5 + AppShell 1 + toast 1 + Redirects 2（404 面板与回书架）→ **数量够，但错行 13 条见第二节**。
2. **H5/PC 差异表**：文档逐行写「PC 出现 / H5 不出现 / 仅 H5」口径清晰（L15-20、L108），与源码一致；**原型不演示「不出现」**（P0-1）。
3. **书架两套分页**：PC 每页 24（`ShelfPage.tsx:44`）、H5 每页 30 + 240px 预取（`:46`、`:221`）与文档 L62/L65 一致；「加载更多失败」L67 + `ShelfPage.tsx:516-524`（`role="alert"` + 重试）、「已到末尾」L69 + `:536-538`（「没有更多了 · 共 N 本」）→ **两态齐全**。
4. **搜索**：「关键词太短」L81（文档写「至少 2 个字」，源码 `SearchPage.tsx:85` 文案是「至少输入 2 个字，才能找到有意义的匹配」——语义一致）、L82 未输入、L83 无结果、L79 片段受限（**口径矛盾，见 P0-2**）。
5. **回书架恢复滚动**：L70 — 300ms 防抖写 `{scrollTop,lastBookId}`（`ShelfPage.tsx:176-189`）、返回后只在首次恢复（:197-211）、`scrollTop<=0` 才居中、高亮 1.5s（:210）→ **机制写明且与源码一致**。
6. **「新增」项**：文档仅 2 处「新增」— L48 评分星标（leader 已判删除，悬置）、L86 搜索分页（代价未写，P1）。书架评分星标已由 leader 判定删除这一点文档未同步。

## 六、没跑 / 未覆盖

- 真实服务 `http://127.0.0.1:8080` 只做只读核对，未与服务端数据比对（本任务是原型+文档评审）。
- `odops verify` 未加 `--browser`（渲染层点击区/对比度由 agent-browser 手测覆盖，非 verify 计数）；官方 `lint` 需 daemon，未跑，不算通过。
- 1440/390 两张 shots 只核尺寸与是否破图，未逐像素与当前原型 diff（原型文件 mtime 与 shots 同为 22:35，可视为同版）。
