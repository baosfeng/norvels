# 阅读域（reader）· 评审报告

> 评审对象：`docs/ux/reader.md`（94 行 / 76 行元素表）· `.open-design/ux/reader.html`（109 处 `data-od-id` / 75 个唯一锚点）· shots 三张。
> 判据：`docs/ux/README.md` §6 冻结的 12 条（不另立标准）。作者自述一律不作为证据，下列结论均出自本轮自跑命令。
> **本报告只判缺陷，是否放行由 leader 决定。**

## 1. 12 条判据逐条结论（过 6 / 不过 6）

| # | 级别 | 结论 | 证据（本轮自跑） |
|---|---|---|---|
| 1 | P0 | **不过** | 标签准确性：L73 `reader:restore` 标「现有」，但该文案在源码 0 命中（见 P0-4） |
| 2 | P0 | 过 | 76 行元素表逐行看触发/结果齐备；46 行无失败表述者均为承载/展示元素（topbar、title、overlay、bottombar），无失败面 |
| 3 | P0 | 过 | 五面板可开：toc/bookmarks/settings/search/help 全部 `is-open=true`；溢出实测 1440 `scrollWidth=1440`、390 `scrollWidth=390`；状态切换按钮 `data-demo-state` 可用 |
| 4 | P0 | **不过** | 书签「加载中」、目录「加载中」两态在原型缺失（见 P0-1/P0-2） |
| 5 | P0 | **不过** | 反向对表 72/72 通过；正向缺上述两态演示，且 L37 把「再显示 60 章」记在 `reader:toc-list` 而原型是独立锚点 `reader:toc-more` |
| 6 | P0 | 过 | `grep -nE 'SQLite|FTS|API|bookId|localStorage|虚拟滚动|SHA-256|用时'` 两文件均 0 命中；命中项只有 `aria-*` 属性，不属实现词 |
| 7 | P0 | 过 | denylist 逐条比对命中 0；`/Users/`、`/Volumes/` 均 False；原型书名不在本地库 20 本书内（虚构） |
| 8 | P0 | **不过** | H5 下 `reader:restore` 的「知道了」50×24（<44），见 P0-3；其余 PROD 目标 390 下均 ≥44（顶栏 44×44、底栏 h48、设置面板 0 项超标） |
| 9 | P1 | **不过** | 76 行中 67 行状态列无「按下」态 |
| 10 | P1 | **不过** | 删除书签无二次确认（L90 明写不做），判据字面要求二次确认 |
| 11 | P1 | 过 | 滑动翻章有按钮/键盘替代路径；观察：`useSwipePaging` 全 `passive` 监听，边缘右滑无法阻止浏览器返回，与 L93 声明略有出入 |
| 12 | P1 | 过 | 正文空态有「先读下一章」、错误态有「重试/换一章」、目录空态有「清空搜索」、书签空态指向上方添加按钮，均可重试 |

## 2. P0 缺陷（4 条）

| # | 对象 | 问题 | 证据 | 建议改法 |
|---|---|---|---|---|
| P0-1 | 原型 `reader:bookmarks-drawer` | 文档 L39 声明「加载中 / 正常 / 空 / 错误 / 收起」五态，原型只有空/错误/列表，无加载中，判据 4、5 不过 | `grep -c '正在读取书签' .open-design/ux/reader.html` → `0`（范围：该文件全 1390 行）；源码 `BookmarksDrawer.tsx:64-65` 确有 `LoadingPanel` | 原型补 `#bm-loading` 状态块 + 演示条加「书签·加载中」档 |
| P0-2 | 原型 `reader:toc-drawer` | 文档 L32 声明「加载中（正在读取目录）」，原型无此态 | `grep -c '正在读取目录' .open-design/ux/reader.html` → `0`（同范围）；源码 `TocDrawer.tsx:106-107` 有 `LoadingPanel` | 原型补目录加载态并可切档 |
| P0-3 | 原型 `reader:restore`（`#restore-dismiss`） | H5 触控目标不达标：50×24，inline 写死 `min-height:24px` | 页面内 `getBoundingClientRect()` → `{"w":50,"h":24,"style":"min-height:24px;padding:0 6px"}`（reader.html:485）；390 视口下唯一 `#PROD` 超标项 | 改成 `.btn` 并给 `min-height:44px`，或把「知道了」并入提示条整块可点 |
| P0-4 | 文档 L73 / L83（同一锚点） | 同一 `reader:restore` 两行两标签：L73「现有」、L83「改进」；且 L73 声明的「已回到上次读到的地方」在产品里不存在 | `grep -rn '已回到上次读到的地方' web/src` → `0`；`grep -rn '知道了' web/src/features/reader` → `0` | 合并为一行；标签改「新增」；或先实现该提示再回填「现有」 |

## 3. P1 缺陷（记待办，随交付）

| # | 对象 | 问题 | 证据 | 建议改法 |
|---|---|---|---|---|
| P1-1 | 文档全表 | 67/76 行状态列无「按下」态 | 逐行统计（见 §5 命令） | 按钮类元素补「按下」 |
| P1-2 | `reader:bookmark-delete` | 破坏性操作无二次确认 | L42、L90 | 保持「不做确认」也可以，但需在 L42 撤销列写明「误删不可恢复，需重新添加」已具备——判据 10 按字面仍不过，建议 leader 决定是否接受 |
| P1-3 | `reader:autoscroll-speed` | 档位三处不一致：文档 L64「0.5–3」、原型 `SPEED_MAX=3`（reader.html:1273）、源码 `SPEED_MAX=5`（readerSettings.ts:55） | 三处 grep | 先定口径（建议以源码 0.5–5 为准），同步文档与原型 |
| P1-4 | `reader:font-family` / `reader:width` | 标签不一致：原型「无衬线」「中」vs 源码「黑体」「标准」 | reader.html:679/694 vs readerSettings.ts:35-36 | 定一套文案 |
| P1-5 | 源码行号引用 | 67 条引用抽检，18 条超 ±3 行（见 §4） | 脚本逐条比对 | 按 §4 右列改正 |
| P1-6 | 目录抽屉 | 源码 `TocDrawer` 只接 `loading`，`book.error` 无分支，目录加载失败时界面无反馈 | `TocDrawer.tsx:106-111` 三元只有 loading/空/列表 | 补错误态或明确降级口径 |
| P1-7 | `reader:toc-volume` / `reader:toc-more` | 引用 `TocDrawer.tsx / ChapterList.tsx`，但两文件均无卷折叠、无「再显示 60 章」实现（`grep -rn '再显示|toc-more|60 章|已显示' web/src` → 0） | 同上 | 新增项写「新增 · 暂无实现」，不要给不存在的源码位置 |
| P1-8 | 双向对表粒度 | L37 把「再显示 60 章」记在 `reader:toc-list`，原型另有 `reader:toc-more` 独立锚点 | `grep -n 'reader:toc-more' reader.html` → 1 处（608 行） | 文档拆成独立锚点行 |

## 4. 源码行号抽检（67 条全查，18 条错行）

命令：脚本逐条比对「文档引用行」与「该元素的真实行区间」，容差 ±3；`TOTAL=67 HIT(+-3)=49 MISS=18`。

| 锚点 | 文档引用 | 实际内容 | 正确行 |
|---|---|---|---|
| `reader:bottombar` | ReaderBottomBar.tsx:88 | `settingsOpen: boolean;` | **94** |
| `reader:toc-search` | TocDrawer.tsx:64 | `>` | **77** |
| `reader:toc-current` | TocDrawer.tsx:47 | 空行 | **87** |
| `reader:bookmarks-drawer` | BookmarksDrawer.tsx:22 | `onDelete,` | **39** |
| `reader:bookmark-add` | BookmarksDrawer.tsx:50 | 关闭按钮 | **55** |
| `reader:bookmark-empty` | BookmarksDrawer.tsx:64 | loading 三元 | **69** |
| `reader:bookmark-error` | BookmarksDrawer.tsx:60 | `disabled={!canAdd…}` | **67** |
| `reader:search-panel` | ReaderSearchPanel.tsx:42 | 空行 | **47** |
| `reader:settings-panel` | ReaderSettingsPanel.tsx:62 | `onChange,` | **80** |
| `reader:font-size` | ReaderSettingsPanel.tsx:91 | `关闭` | **95** |
| `reader:follow-system-font` | ReaderSettingsPanel.tsx:100 | `</button>` | **108** |
| `reader:leading` | ReaderSettingsPanel.tsx:110 | `<Switch` | **117** |
| `reader:help-panel` | ReaderHelpPanel.tsx:47 | `<dt>` | **28** |
| `reader:toc-btn-h5` | ReaderBottomBar.tsx:95 | 上一章按钮 | **99** |
| `reader:bookmarks-btn-h5` | ReaderBottomBar.tsx:99 | 目录按钮 | **103** |
| `reader:search-btn-h5` | ReaderBottomBar.tsx:103 | 书签按钮 | **107** |
| `reader:bar-prev` | ReaderBottomBar.tsx:91 | `searchOpen: boolean;` | **95** |
| `reader:autoscroll-bar` | ReaderAutoScrollBar.tsx:12 | `running: boolean;` | **19** |

> 边界项（不计入 18）：`reader:toc-list` 引 ChapterList.tsx:30 落在 ±3 内，但那是文件头注释、组件本体在 37 行；`reader:restore` 引 399 是 effect 前一段注释、代码体在 402-413。
> `ReaderBottomBar.tsx` 的 5 条（95/99/103/91/117）呈系统性 −4 偏移，疑按旧版行数推算；其余为散点偏移。

## 5. 锚点双向对表与取证命令

```bash
grep -oE '`(reader|demo):[a-z0-9-]+`' docs/ux/reader.md | tr -d '`' | sort -u   # 73（含 demo:control）
grep -o 'data-od-id="[^"]*"' .open-design/ux/reader.html | sed 's/data-od-id="//;s/"//' | sort -u  # 75
comm -23 doc html   # 文档有、原型无 → 空
comm -13 doc html   # 原型有、文档无 → demo:bp-pc / demo:bp-h5（断点按钮，README 4.4 不要求登记，不计缺陷）
```

浏览器取证（会话 `norvels-ux-review-reader`）：`document.querySelectorAll('[data-od-id]').length` → 79 处 / 75 唯一。

## 6. 我主动排除的误报

- 「抽屉关闭按钮点不动」→ `#toc-close.click()` 后 `is-open=false`，关闭正常（agent-browser CLI 报 covered 是落点判定，非用户视角）。撤回。
- `demo:bp-pc`/`demo:bp-h5` 未登记进文档 → README 4.4 只要求演示控制条存在，撤回。
- `reader:duration` 在原型里是 `hidden` span → 文档 L72 明写「无可见按钮」，撤回。
- 46 行「无失败表述」→ 多为承载/展示元素，无失败面，撤回（只保留 P0-1/P0-2 两处真实状态缺口）。
- 隐私扫描整跑未拿到稳定 stdout，改用 denylist 逐条比对（命中 0）+ 路径扫描，结论不受影响。

## 7. 需 leader 决策

1. **P0-4 定级**：把不存在的交互标「现有」我判 P0（会误导开发跳过实现）；若认为标签错误归 P1，请降级并转给作者。
2. **P1-2**：删除书签坚持不做二次确认是否接受（有「可重新添加」理由）。
3. **P1-3**：速度档位以源码 0.5–5 还是原型的 0.5–3 为准，定了才好同步三处。
4. **观察项（不入缺陷表）**：产品源码里 reader 域只有 5 个 `data-od-id`（`reader`/`reader-top`/`reader-progress`/`reader-bookmarks`/`reader-drawer`），命名是短横线而非文档的 `reader:xxx` 冒号体系，两套锚点无法直接对账——是否要求实现方按文档锚点改名，请定。
