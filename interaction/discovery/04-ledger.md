# 发现域（discovery）返工台账

> 每行 = 一条意见 → 改了什么 → 谁验的。状态只有两种：`已改` / `保留（理由）`。
> 意见来源：`03-review.md`（r2 三条 P0 + P1 七条）+ 本轮 `ixcheck` 输出。改完已重跑两道门禁（结果见文末）。

## r2（本轮：评审 FAIL 后的返工）

| # | 轮次 | 意见（评审原话摘要） | 级别 | 处理 | 改在哪 | 状态 |
|---|---|---|---|---|---|---|
| 1 | r2 | 首页「最近入库」三态写了但切不出来（C-47/C-48 边界列承诺加载与错误面板，原型 renderHome 只判 empty） | P0 | 补 #home-loading（转圈 +「正在读取书目 · 稍等一下，正在取回最近入库的书。」）与 #home-error（「书目没能加载」+「重试」按钮 C-71）；renderHome 按 st.force 或 st.pageState 真的切三态并隐藏列表 | 02-prototype.html 首页区块 + renderHome · 契约 C-47/C-48 边界 · 新增 C-71 | 已改 |
| 2 | r2 | 搜索加粗与源码不符（SearchPage.tsx 全文 <b>/<strong>/highlight 命中 0；正文检索已下线） | P0 | **按裁决删除**：删 highlight() 函数、删所有 <b> 注入、页面说明改为「结果是书名、作者名与章节标题的匹配列表；正文不参与检索，因此不显示正文片段，也不做关键词加粗。」；C-49/C-51/C-52 边界与前置同步改；**未标「新增」**（属既有决策） | 02-prototype.html · 契约 C-51/C-52/C-49 · 00-features.md | 已改 |
| 3 | r2 | toast 不可交互却写了 hover/focus/键盘 三行，focus 还承诺焦点环 | P0 | 删掉 C-17 的 hover / focus / 键盘 三行，只留 1 行 click（系统触发），控件名改「提示条（不可交互，仅供阅读）」；保留 role="status" aria-live="polite" 并在边界写明「只服务读屏，不提供键盘焦点」；原型同步删掉提示条里的关闭按钮与它的绑定 | 01-contract.md C-17 · 02-prototype.html toast 结构 | 已改 |
| 4 | r2 | 随机一本写成「不跳详情页」，源码是 navigate(/book/id) | P1 | 结果列改为「按当前筛选随机取一本并存下，跳到 /book/:bookId 详情页」；原型改为浮出「随机挑到《…》，正在打开详情。」后进入详情 | 契约 C-35 · 02-prototype.html 随机处理 | 已改 |
| 5 | r2 | 错档落点错：TierRoute.tsx:18-20 窄屏落 /home，不是书架 | P1 | 契约与原型统一：窄屏访问系列/历史/系列详情 → 落**首页**并浮出「「系列」在窄屏不提供，已回到首页。」；宽屏访问首页 → 落书架。原型 fallback 改为按档位取值 | 契约 C-04/C-05/C-06/§0 · 02-prototype.html setView | 已改 |
| 6 | r2 | 反馈列模板化：focus 70 行同一句、42 行「，150ms，150ms」笔误 | P1 | 按控件类型给差异化反馈：链接给下划线、导航项给整行底色、按钮给底色+描边、下拉给描边、输入框给边框+光标、分栏给下划线、卡片给浮起、chip 给描边；「150ms，150ms」已清零 | 01-contract.md 全表 | 已改 |
| 7 | r2 | 键盘行无时长无文案 | P1 | 71 个锚点的键盘行全部写明：按哪个键、多久见效、有无视觉反馈 | 01-contract.md 全表 | 已改 |
| 8 | r2 | 49/70 个 click 行的「移除/关闭后」写「—」，但筛选与翻页类必须写五问 | P1 | 补 16 行五问（排序 / 7 个筛选 / 6 组分页）；剩 29 行是纯跳转类（详情、导航、开新页），按规则写「—」合规 | 01-contract.md C-19/C-22～C-28/C-37/C-38/C-54/C-55/C-60/C-61/C-69/C-70 | 已改 |
| 9 | r2 | 输入触发 0 行，搜索框与批量输入框都没写输入行为 | P1 | 4 个输入类控件（C-12 顶栏搜索、C-43 首页搜索、C-49 搜索页、C-56 系列搜索）改为「输入」触发，写清逐键回显 ≤16ms、不做增量检索、输入法组合期间不触发、Esc 清空、空值与 <2 字表现 | 01-contract.md C-12/C-43/C-49/C-56 | 已改 |
| 10 | r2 | 源码行号 18/69 条不可靠（SearchPage.tsx:200 越界、Redirects.tsx:14 是空行） | P1 | **全量重核 71 条**：逐条 grep -n 定位真实行号（SearchPage.tsx:44 搜索框、:68 分栏、:105 书名命中、:132 章节命中、:89 错误重试、:161 文件末（分页为新增，标在末行并注明无实现）；Redirects.tsx:21 回书架按钮；ShelfPage.tsx:679 角标、:726 书卡、:545/:555 翻页） | 01-contract.md 控件列全表 | 已改 |
| 11 | r2 | ixcheck P1 C001：原型出现占位内容 "placeholder" | P1 | **保留**。命中处是 CSS 伪元素选择器 .input::placeholder{color:var(--muted-text)}，不是占位文案；删掉会让输入框占位文字掉回浏览器默认灰（实测不足 4.5:1），而 ixcheck 是纯文本匹配、无法区分伪元素与占位文案 | 02-prototype.html 样式段 | 保留（理由） |
| 12 | r2 | ixcheck P1 T011 + P2 T012：C-17 缺 focus / hover 触发 | P1/P2 | **保留**。工具的豁免判据是控件名含「移动端 / 手机 / 触屏」，而 C-17 是**只读浮层**——源码 toast.tsx:18 是 role="status" aria-live="polite" 的 div，DOM 里没有任何可聚焦元素，补 focus 行等于承诺一个不存在的焦点环（正是本轮 P0-3 要修的问题）。已在控件名写明「不可交互，仅供阅读」并在边界说明 | 01-contract.md C-17 | 保留（理由） |
| 13 | r2 | odops verify warn：69 个交互元素 <44px | 待记 | **保留**。390 窄屏档实测可点元素 <44px 计数 = 0；69 个全部落在 PC 档（侧栏 36px、工具栏 34px），符合本项目「PC ≥32×32、H5 ≥44×44」分档口径 | — | 保留（理由） |

| 14 | r3 | 390 档搜索命中行里书名/章节两个链接只有 23-24px（上一轮漏检） | P0 | 给 .hit-meta a 加 min-height:44px + min-width:44px + 水平内边距；实测 390 档 <44px = 0 | 02-prototype.html 命中行样式 | 已改 |
| 15 | r3 | 390 档分页箭头 36×36 | P0 | body[data-tier=h5] .pagination button 提到 44×44；六个视图在 390 档逐一实测 <44px 均 = 0 | 02-prototype.html 分档样式 | 已改 |

## r1（上一轮：7 列说明表 → C-xx 契约升级，已完成）

| # | 意见 | 级别 | 处理 | 状态 |
|---|---|---|---|---|
| 1 | 原型能切出不合档页面 | P0 | 加 tierAllows() 可达矩阵 | 已改（r2 修正落点为首页） |
| 2 | 搜索片段口径矛盾 | P0 | 统一按源码 | 已改（r2 进一步删掉加粗） |
| 3 | 首页最近入库无契约行 | P0 | 补 C-44～C-48 | 已改（r2 补 C-71） |
| 4 | H5 触控 21px | P0 | 段头文字链改 44px | 已改 |
| 5 | 删书架评分星标 | 决策 | 控件与函数一并删 | 已改 |

## 自测证据（本轮实跑）

```
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/discovery --platform web
→ 契约行 281 / 锚点 71 / 原型锚点 71 / focus 覆盖 70/71
→ 结论: PASS  (P0=0 P1=2 P2=1)   退出码 0

odops verify interaction/discovery/02-prototype.html --platform web --browser
→ exit 0；自包含 / 56 槽 token / 0 裸色值 / 4 断点齐 / lint p1 干净 / 无横向溢出 / 对比度达标
```

浏览器实测（AGENT_BROWSER_SESSION=norvels-ix-discovery）：

- 首页四态：正常 5 行 / 加载显示转圈且列表隐藏 / 错误显示「书目没能加载」+ 重试 / 点重试恢复 5 行。
- 搜索：document.querySelectorAll('#search-hits b, #search-books b').length = 0，仍 8 条章节命中。
- 提示条：#toast 无 tabindex、无关闭按钮，role="status"。
- 错档：390 档切系列 → view=home +「「系列」在窄屏不提供，已回到首页。」
- 书架：24 卡 / 3 页、角标四态、批量「已批量更新 2 本书的作者」、筛选 60 → 20 → 60。
