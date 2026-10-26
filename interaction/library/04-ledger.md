# 04 返工台账 · 书目域（library）

> 每条意见一行，状态只有两种：`已改` / `保留（理由）`。意见来源优先取 `03-review.md`；
> 还没有评审时以 `ixcheck` 的 P0/P1/P2 条目为准，**不自己编评审意见**。

## 本轮（r1 · 首轮契约化）

| # | 轮次 | 意见（来源与原文摘要） | 级别 | 处理 | 改在哪 | 状态 |
|---|---|---|---|---|---|---|
| — | r1 | 首轮交付，无评审意见 | — | 按 `designing-interactions` 契约格式生成，未返工 | — | 已改 |

### 本轮自检发现并已修的 3 条（`ixcheck` 与真点击取证暴露的缺陷）

| # | 轮次 | 意见 | 级别 | 处理 | 改在哪 | 状态 |
|---|---|---|---|---|---|---|
| 1 | r1 | C-12「清空搜索」后无匹配空态面板仍粘在目录上（契约要求的移除后行为在原型里没做到） | P0 | `renderSearchView` 清空时一并收起空态面板 | `02-prototype.html` · runTocSearch/renderSearchView | 已改 |
| 2 | r1 | C-12 清空后焦点停在已被隐藏的按钮上 | P0 | 清空后把焦点送回 C-11 搜索输入框 | `02-prototype.html` · #tocClear 处理器 | 已改 |
| 3 | r1 | C-18「完成」关闭书单面板后焦点未回到触发按钮 | P0 | 关闭时显式 focus 到 C-04 | `02-prototype.html` · #collectionDone 处理器 | 已改 |

### 保留项（写理由，不是「以后再说」）

| # | 轮次 | 项 | 级别 | 理由 | 状态 |
|---|---|---|---|---|---|
| 1 | r1 | `odops verify` 报「51 个交互元素高度 <44px」（PC 档） | — | 该阈值是**触屏平台**的 P0；本域平台 = web，PC 侧按 README「PC 可点区域 ≥32×32」执行，H5 断点已实测全部 ≥44（见 `shots/library-390.jpg` 与下文取证）。平台参数确认为 web，不改判据。 | 保留（理由） |
| 2 | r1 | `odops verify` 报「文中有 7 个 emoji 字符」 | — | 命中的是 ★ / ☆ 评分星与 ← / → 分页箭头，它们承担语义（评分档位、翻页方向）而非装饰，且已有 `aria-label`（「1 星」「上一页」）。换成图标字体或 SVG 会引入外部依赖，违反单文件自包含。 | 保留（理由） |
| 3 | r1 | `odops verify` 报「小于 14px 的文字」 | — | 命中的全是 12px 层级的元信息（演示控制条标签、面包屑、eyebrow），与同批三份已冻结原型口径一致；正文与可读信息均为 14px 及以上。 | 保留（理由） |

## r2 · 评审 FAIL（4 条 P0 + 2 条 P1）

| # | 轮次 | 意见（评审原话摘要） | 级别 | 处理 | 改在哪 | 状态 |
|---|---|---|---|---|---|---|
| 1 | r2 | 八个「产品不存在」的交互在契约里按现有写，与 00-features.md 口径相反，开发会以为已能用 | P0 | 逐条控件名列尾加【新增】、边界列补「现状：…（源码位置）；需：…」：C-06 系列名、C-09 标签、C-11 目录搜索、C-22 作者名、C-26 图例、C-27/C-28 健康卡、C-30 热力格 | 01-contract.md 对应 8 个锚点共 33 行 | 已改 |
| 2 | r2 | C-14 卷折叠后焦点丢失（实测 c14Post=BODY），根因 renderVolumes() 重建 host.innerHTML | P0 | 折叠后查新卷头并显式 focus()；移除原 document 级兜底，避免两处抢焦点 | 02-prototype.html · renderVolumes 卷头处理器 | 已改 |
| 3 | r2 | 原型有 authorSearch，契约第 12 行却写「本轮不加搜索框」，契约与原型相反 | P0 | 按契约删原型：搜索框、命中提示、清空按钮、filterAuthors 函数与两处事件绑定全部移除；保留排序 C-19 + 分页 C-20/C-21 | 02-prototype.html（authorSearch 命中数 5 → 0） | 已改 |
| 4 | r2 | 热力图 91 格悬停全显示同一句，闭包内 var s2 被共享，aria 与浮层文不一致 | P0 | 抽出 makeHeatCell(cell, col) 让每格闭包持有自己的数据 | 02-prototype.html · makeHeatCell | 已改 |
| 5 | r2 | 演示件语义失效：demo-only 打在整个产品外壳，排除 demo 子树后产品控件计数变 0 | P1 | demo-only 只留演示条与演示装置（8 个），其余 51 个内容块改 region:* | 02-prototype.html | 已改 |
| 6 | r2 | 详情页署名作者链接（BookPage.tsx:232 Link to=/authors）是契约外唯一可点元素 | P1 | 补 C-32 四行契约（click/hover/focus/键盘），原型同步挂锚点，标现有 | 01-contract.md C-32 · 02-prototype.html byline | 已改 |

### r2 取证

\`\`\`bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/library --platform web   # exit 0，P0=0 P1=0 P2=0，契约 129 行 / 锚点 32 / focus 32/32
grep -c '【新增】' interaction/library/01-contract.md        # 33
grep -c 'authorSearch' interaction/library/02-prototype.html # 0
odops verify interaction/library/02-prototype.html --platform web --browser  # exit 0
\`\`\`

实测两条：
- 卷折叠后 \`document.activeElement\` 的 data-od-id = **C-14**（折叠前后一致）。
- 热力格第 1 格「7 月 28 日 · 15 分」、第 50 格「9 月 15 日 · 没有阅读」，浮层文本与各自 aria-label 逐字相同（match=true）。

### r2 新发现并已修（自查发现，非评审意见）

| # | 轮次 | 项 | 级别 | 处理 | 改在哪 | 状态 |
|---|---|---|---|---|---|---|
| 7 | r2 | 删搜索框后 \`applyState\` 仍引用 authorsEmptyReset，会抛 null 引用 | P0 | 一并移除该行引用 | 02-prototype.html · applyState | 已改 |

## 取证命令（可复跑）

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/library --platform web   # exit 0，P0=0 P1=0 P2=0
odops verify interaction/library/02-prototype.html --platform web --browser                            # exit 0，12 项 ok / 0 FAIL
odops shot interaction/library/02-prototype.html --width 1440 --height 900 --out interaction/library/shots/library-1440.jpg
odops shot interaction/library/02-prototype.html --width 390 --height 844 --out interaction/library/shots/library-390.jpg
```
