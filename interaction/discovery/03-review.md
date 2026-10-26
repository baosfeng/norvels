# discovery 域 · 交互契约独立评审（r2 · 交叉复审）

> 对象：`interaction/discovery/`（`01-contract.md` 280 行 / 70 锚点 · `02-prototype.html` 70 个 `C-nn` + 11 个 `region:*` + `demo-only:bar` · `00-features.md` · `04-ledger.md` · `shots/`）。
> 判据：`designing-interactions` 的 `SKILL.md` + `reviewer.md` + `references/interaction-contract.md`。**不复述作者自述与台账，证据全部自跑。**
> 取证会话：`AGENT_BROWSER_SESSION=norvels-ixrev-disc`；只读，未改任何产物。

## 结论

**FAIL** —— 3 条 P0：首页「最近入库」的加载/错误态在源码与契约里都有、原型切不出来；搜索「命中词在标题里加粗」源码没有这个能力（上一轮 P0 声称按源码统一，实际没落地）；toast 被契约写成有 click/hover/focus/键盘 四种触发的控件，原型里它不可聚焦也不响应悬停。

## 覆盖度

- 契约锚点 **70** / 人工审过 **70** / 跳过 **0**
- 合同行 **280**，触发分布 click 70 / hover 70 / focus 70 / 键盘 70；**长按、双击、右键、滑动拖拽、输入 各 0 行**
- `ixcheck`：`契约行 280 / 锚点 70 / 原型锚点 70 / focus 覆盖 70/70`，**P0=0 P1=1 P2=0**，退出码 **0**
- `odops verify`：退出码 **0**，13 项机械检查全 ok（自包含 / 56 槽 token / 0 裸色值 / 4 断点 / 无横向溢出 / 对比度达标）；该命令**未输出** screensScanned / taps / contrast 计数，渲染计数记**未覆盖**
- 断点实测：1440 `scrollWidth=1440`；390 `scrollWidth=390`（无横向溢出）
- 截图：`interaction/discovery/shots/discovery-1440.jpg` · `discovery-390.jpg`（存在；作者产出，未作为我的证据）

## P0 缺陷

| # | 对象 | 现状 → 期望 | 证据（命令 + 输出行） |
|---|---|---|---|
| P0-1 | C-47 / C-48 首页「最近入库」 | 契约边界列写了「书目正在读取时本段头仍显示，正文给加载面板」「加载失败时整段换错误面板并给「重试」」，源码也有；**原型切不出这两态** → 期望演示控制条的「加载 / 错误」在 home 视图真的渲染 | `grep -c 'id="home-loading"\|id="home-error"' interaction/discovery/02-prototype.html` → **0**；源码 `web/src/features/start/HomePage.tsx:121-124`（`recent.loading ? <LoadingPanel…> : recent.error ? <ErrorPanel…>`）；浏览器在 `view=home` 连点「加载」「错误」→ `{"state":"ok","homeRows":5}`，`renderHome` 只判 `empty`（`02-prototype.html:1278-1291`） |
| P0-2 | C-49 / C-51 / C-52 搜索 | 契约 3 处断言「命中词在标题里加粗 / 只作用在标题与作者名上」，原型也真渲染 `<b>`；**源码没有这个能力** → 期望契约按源码改写，或把该能力明确标成「新增（需前端加）」。这同时说明上一轮 P0「统一按源码」**没有真正落地** | `grep -cE '<b>|<strong>|highlight' web/src/features/search/SearchPage.tsx` → **0**；`grep -n 'hasSnippet(hit.snippet)' …` → 仅 1 处、只用于章节片段；`web/src/lib/snippet.tsx:31-34` 的 `<b>` 只来自后端 snippet，而 `internal/store/search.go:116` 写明「正文检索已下线，无法给正文片段」。原型 `02-prototype.html:1504-1507` 自己造了 `highlight()` 注入 `<b>`，实测 `<h3 class="book-title"><b>长夜</b>未明</h3>` |
| P0-3 | C-17 提示条 toast | 契约给了 toast 四行（click / hover / focus / 键盘），焦点行写「外圈出现 4px 焦点环」、悬停行写「主按钮底色由 var(--accent) 转 var(--accent-hover)」；**原型 toast 不可聚焦、无悬停态** → 期望改成「无交互、不可聚焦、不可点击」，只留播报与自动消失 | `grep -c 'tabindex' interaction/discovery/02-prototype.html` → **0**；`02-prototype.html:969` `<div class="toast" role="status" aria-live="polite" hidden>` 是 div，无 tabindex、无 hover 规则；对照 `01-contract.md:86-88` 三行反馈 |

## P1 缺陷

| # | 对象 | 问题 | 证据 |
|---|---|---|---|
| P1-1 | C-35 随机一本 | 契约「结果」列写「列表滚到该书卡并高亮 1.5 秒…**不跳详情页，留在书架**」→ 源码直接跳详情页；原型只弹一句 toast，既不滚动也不高亮 | `web/src/features/shelf/RandomBookButton.tsx:26-27` `const book = await api.randomBook(...); navigate(...)`；原型 `02-prototype.html:1390` 只 `showToast('随机挑到《…》。')` |
| P1-2 | C-05～C-10 窄屏错档落点 | 契约与原型都写「落回书架」；源码窄屏访问 PC 专属页重定向到 **`/home`** | `web/src/layouts/TierRoute.tsx:18-20` `if (minTier === "pc" && tier === "h5") return <Navigate to="/home" replace …/>`；原型 `02-prototype.html:1083` `var fallback = st.tier === 'h5' ? 'shelf' : 'shelf';`；契约 `01-contract.md:37` 边界列。实测 H5 点「系列」→ `{"view":"shelf","tier":"h5"}`（**重定向语义本身已实现**，仅落点不同） |
| P1-3 | 全表反馈列 | 模板化：70 行 focus 完全同一句、32 行 hover 同一句（含「主按钮底色…」）、42 行出现重复的「，150ms，150ms」；click 行 70 行里 55 行无文案原文 | `python3` 统计：`focus x70` / `hover x32`（同一串）/ `，150ms，150ms` 42 行 / `反馈无文案原文: 265` |
| P1-4 | 全表键盘行 | 70/70 键盘行**无时长、无文案**，且与 click 不同步：C-21（排序选项）的键盘行拷贝了「列表归属按新筛选重算」 | `按触发类型统计：键盘 rows=70 无时长=70 无文案=70`；`01-contract.md:104` |
| P1-5 | 全表移除/关闭后 | 70 个 click 行只有 21 行填了，49 行写「—」；其中 C-31（阅读状态角标）会把书移出当前筛选列表，属「让东西消失」，应写五问 | `python3 → click 行 70 / 写 — : 49`；`01-contract.md:141` 第 7 列为 `—`。实测筛选「想读」16 张 → 点角标 15 张，无撤销入口 |
| P1-6 | 全表输入触发 | 四个搜索框（C-12 / C-43 / C-49 / C-56）都属「输入」，契约 0 行输入触发；「防抖 300–400ms」也没写 | `grep -c '| 输入 |' interaction/discovery/01-contract.md` → **0**；原型有 5 个 `type="search"` |
| P1-7 | C-19～C-27 | 这些是原生 `<select>`，click 行写「选中后列表立即只保留…」——click 只展开，真正生效在 change，反馈缺「展开态」 | `01-contract.md:93` / `:105`；源码 `web/src/features/shelf/ShelfPage.tsx:361` 走 `onChange` |
| P1-8 | 源码行号 | 69 条引用中 **18 条不可靠**（明细见下表） | `python3` 逐条比对：`unique refs: 69 可疑: 18` |

### P1-8 明细（文档引用 → 实际内容 → 正确行）

| 锚点 | 引用 | 实际内容 | 正确行 |
|---|---|---|---|
| C-54 / C-55 | SearchPage.tsx:200 | 文件只 161 行，**越界** | 该能力在源码不存在 |
| C-18 | Redirects.tsx:14 | 空行 | :21 |
| C-17 | toastStore.ts:25 | `}` | :27 |
| C-01 / C-03 / C-09 | Sidebar.tsx:26 / :40 / :58 | `</NavLink>` | :23 / :38 / :55 |
| C-10 | Sidebar.tsx:66 | `{resume ? (` | :59 |
| C-13 | Topbar.tsx:51 | `{showViewToggle…` | :52 |
| C-21 | ShelfPage.tsx:566 | `{isH5 && sheetOpen ? (` | :567 |
| C-27 | CategoryFilter.tsx:51 | 文本「小分类」 | :52 |
| C-44 | HomePage.tsx:94 | `</Link>` | :92 |
| C-48 | HomePage.tsx:127 | `.map((book) => (` | :128 |
| C-50 | SearchPage.tsx:67 | `{data && !isH5 ? (` | :68 |
| C-53 | SearchPage.tsx:88 | `) : error ? (` | :89 |
| C-57 | SeriesPage.tsx:113 | `.map((card) => (` | :114 |
| C-59 | SeriesPage.tsx:87 | `) : feed.error ? (` | :88 |
| C-63 | SeriesDetailPage.tsx:72 | 组件定义（非渲染点） | :63 |
| C-68 | HistoryPage.tsx:36 | `) : feed.error ? (` | :37 |

## P2

- C-44 / C-47 在原型同一语义上重复标注容器与按钮（`02-prototype.html:686` 与 `:689` 都是 `C-44`）。
- 中英标点混用（`—` 与 `·`）；C-18 的「离开空态」措辞用在 404 页偏题。
- `00-features.md:15` 写「H5 不出现（`TierRoute.tsx:18`）」，实际判断在 :18、`Navigate` 在 :19。

## 不符直觉清单

| # | 现象 | 证据 | 为什么反直觉 |
|---|---|---|---|
| 1 | 阅读状态角标在筛选生效时点一下，这本书**当场从列表消失**，只留 4 秒 toast，无撤销 | 筛选「想读」16 张 → 点角标 → 15 张；`undoBtn: false` | 用户以为在「标记状态」，实际效果是「从当前视图删一行」；对象消失且无回退 |
| 2 | 「随机一本」在原型里只弹 toast、不滚动不高亮；源码直接跳详情页 | 原型 `02-prototype.html:1390`；源码 `RandomBookButton.tsx:27` | 按「随机」的预期是换个书看，不是弹一句话就没了；真实产品却会直接离开书架 |
| 3 | 首页「最近入库」有段头「全部 →」却没有加载/错误态；「最近在读」两段形态相同、三态不齐 | `grep -c 'home-loading\|home-error'` → 0 | 同一屏两段长得一样，一段有三态一段没有，用户无从预期 |
| 4 | toast 上挂了一个「×」关闭按钮，但它 4 秒自动消失，键盘也够不着 | `02-prototype.html:971` 有 `#toast-close`；`tabindex` 计数 0 | 一个只能点一次、4 秒后自己没了、键盘到不了的按钮，用户不会想点也不需要 |
| 5 | 搜索页给了两个分栏，但两端都没有命中词高亮，用户看不出为什么这条命中 | 源码 `SearchPage.tsx` 无任何 `<b>`；原型却渲染了 | 原型教会用户「命中词会加粗」，实现后消失，用户会以为搜索坏了 |
| 6 | 「清除筛选」一次清空七项、无确认；同一页「选择模式」退出清空所有勾选、也无确认 | `01-contract.md:133`（可撤销＝重新选回）、`:145`（勾选清空无二次确认） | 两类「批量丢失用户输入」都靠「再点一遍」当撤销；筛选组合越复杂重建成本越高 |

## 我剔除的误报

- `odops verify` warn「70 个交互元素 <44px」→ 实测 390 档**产品控件** <44px 计数 **0**（8 个超标项全在 `demo-only:bar`），PC 按 ≥32×32 口径，撤回。
- `ixcheck` P1 `C001 placeholder` → 命中 `02-prototype.html:141` `.input::placeholder{color:var(--muted-text)}` 是伪元素选择器，不是占位文案。**接受作者保留理由**；但建议把规则从裸词匹配改成匹配 `placeholder="…"` 属性（`ixcheck.mjs:20` 现为裸词表），否则每个带输入框的项目都会永久挂一条 P1。
- 「H5 仍能切出系列整页」→ 实测 `{"view":"shelf","tier":"h5"}` 且浮出「「系列」在窄屏不提供，已回到书架。」，**重定向语义已实现**，撤回（落点差异另记 P1-2）。
- 「抽屉 / 弹层 Esc 失效」→ 实测 Esc 后 `body.className` 由 `h5-sheet-open` 变空、`nav-open` 变 false，撤回。
- 「首页最近入库整块无契约」→ C-44～C-48 五行齐备且 4 触发完整，上一轮该 P0 已解决，撤回。
- 「H5 段头 21px」→ 原型 `02-prototype.html:325` 已给 `min-height:44px`，390 实测产品控件无 <44px，撤回。

## 可复跑命令

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/discovery --platform web
# → 契约行 280 / 锚点 70 / 原型锚点 70 / focus 覆盖 70/70 · P0=0 P1=1 P2=0 · exit 0

odops verify interaction/discovery/02-prototype.html --platform web --browser    # → exit 0，13 项 ok

grep -c 'id="home-loading"\|id="home-error"' interaction/discovery/02-prototype.html   # → 0  (P0-1)
grep -cE '<b>|<strong>|highlight' web/src/features/search/SearchPage.tsx             # → 0  (P0-2)
grep -c 'tabindex' interaction/discovery/02-prototype.html                           # → 0  (P0-3)
sed -n '26,27p' web/src/features/shelf/RandomBookButton.tsx                          # 跳详情 (P1-1)
sed -n '18,23p' web/src/layouts/TierRoute.tsx                                        # 两档落点 (P1-2)
grep -c '| 输入 |' interaction/discovery/01-contract.md                              # → 0  (P1-6)
```

浏览器取证（1440 与 390 各一次 `open`）：`scrollWidth` 分别 1440 / 390；角标四态 `reading→done→未标记→want→reading`；
分页 1→2→3 且「上一页」回启用；H5 无限滚动 30 张 +「加载更多」；排序弹层 Esc 关闭；筛选 60→16→清除后按钮消失；
1 字搜索给「至少输入 2 个字」，2 字搜索 5 本书 / 8 个章节。
