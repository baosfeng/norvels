# 03 评审 · 书目域（library）

## 结论

**复审修订（2026-10-26 晚 · 增量复审）：PASS-WITH-TODO** —— 我报的 4 条 P0 全部「已解决」、P1-4/P1-5 已解决；剩 P1-1/P1-2/P1-3/P1-6/P1-7 属主要项待办，随交付记账。原判 **FAIL** 保留作历史（见下）。

<details><summary>原结论（首轮，2026-10-26 22:5x）· 保留作历史</summary>

**FAIL** —— 机器门禁全绿（ixcheck exit 0、verify exit 0），但契约把 8 个产品里并不存在的交互一律写成「现有」且零新增标注，另有 3 处契约与原型实测对不上（卷折叠焦点、热力图读数、作者页搜索框）。开发照契约实现必然分叉。

</details>

## 覆盖度

- 契约 **125 行 / 31 锚点**，人工全过 31 个锚点，跳过 0
- ixcheck：契约行 125 / 锚点 31 / 原型锚点 31 / focus 覆盖 31/31，**P0=0 P1=0 P2=0，exit 0**
- odops verify：**exit 0**，12 项 ok / 3 项 warn（emoji、51 个 <44px、12px 小字——逐条已核，见「我剔除的误报」）
- 真点 14 项：评分星点 4 再点 4 清零 · 卷折叠 · 目录搜索 · 无匹配空态 · 清空搜索 · 再显示 60 章 · 删除弹层抄错/抄对 · 热力格悬停 · 图例过滤 · 作者页搜索 · 状态四档 · 390 三页档位
- 源码抽检 **23 条**：契约 9 条全查 + 00-features.md 52 条中抽 14 条，逐条 sed -n 核对
- 未覆盖：1024 / 768 仅由 verify 声明「断点齐全」，**未单独截图核对**（原因：预算，优先保 8 列质量与 14 项真点）；H5 真机未跑（原因：模拟器为独占资源，按纪律不占）

## P0 缺陷

### P0-1 八个「产品里不存在」的交互，契约全写成现有、零新增标注

> **状态：已解决**（复审实测）｜`grep -c 【新增】 interaction/library/01-contract.md` → **33**；C-06/C-09/C-11/C-22/C-26/C-27/C-28/C-30 八锚点各自 4–5 行**全部**带【新增】；边界列均改写为「现状：…（源码位置）；需：…」双段式，如 C-06「现状：系列名是 `<dd>` 纯文本不可点（BookPage.tsx:168-175）；需：改为链接并补 /series/:name 跳转」；8 条引用的源码位置本轮抽核 3 条全部命中（BookPage.tsx:277-282 目录区、ReadingHeatmap.tsx:148-153、DashboardPage.tsx:193-198）

契约全文 grep -c 新增 = **1**，且唯一那处是 C-16 边界列里的「新出现的最后一行」，不是能力标注。

| 锚点 | 契约写法 | 源码实况（自跑） | 作者自己的 00-features 怎么写 |
|---|---|---|---|
| C-06 系列名链接 | click 跳系列详情页 | BookPage.tsx:168-175 是 dd 纯文本，无 a 标签 | 「现为纯文本，不可点」 |
| C-09 标签 chip | click 跳书架并按标签筛 | BookPage.tsx:247-252 是 span.chip 静态 | 「现为静态 span，不可点」 |
| C-22 作者名 | click 进该作者作品列表 | AuthorsPage.tsx:64 是 td 纯文本；routes.tsx **无作者作品页路由** | 「现为纯文本，无作品页」 |
| C-11 目录搜索 | 输入即筛 | 00-features.md:23 标「契约新增位」 | 「现状无搜索」 |
| C-26 图例过滤 | click 隐藏该段 | Charts.tsx:181-188 是 span.legend-row 不可点 | 「现不可点」 |
| C-30 热力格 | hover 浮层 + click 固定 5s | ReadingHeatmap.tsx:132 只有原生 title | 「现仅原生 title」 |
| C-27 / C-28 健康卡按钮 | click 跳管理页 / 导入 | DashboardPage.tsx:172-195 健康卡无下一步按钮 | 「现状无下一步入口」 |

> 首轮后果：开发按「现有」理解会跳过建这些能力。现已由【新增】+「现状/需」双段式消除。

### P0-2 C-14 卷折叠：契约写「焦点留在卷头」，原型实测丢到 BODY

> **状态：已解决**（复审实测）｜`hd.focus(); hd.click(); 600ms` → `{"c14":{"pre":"C-14","post":"C-14","aria":"false"}}`，焦点已回卷头，折叠本身仍生效

首轮成因：renderVolumes() 重建整个 host.innerHTML，被点元素被替换，焦点必丢。首轮实测 `{"c14Pre":"C-14","c14Post":"BODY"}`。

### P0-3 作者页渲染了契约明令不做的搜索框，而且真的能用

> **状态：已解决**（复审实测）｜`grep -c authorSearch interaction/library/02-prototype.html` → **0**；契约第 12 行「本轮不加搜索框」与原型现状一致

首轮实况：契约 01-contract.md:12 写「本轮不加搜索框」，原型 02-prototype.html:599 却有可用的 input#authorSearch，输入「林」后 8 行筛成 1 行，04-ledger.md 无报备。

### P0-4 C-30 热力图 91 格悬停全显示同一句读数（闭包 bug）

> **状态：已解决**（复审实测）｜悬停第 1 格 → tip=「7 月 28 日 · 15 分」，第 50 格 → tip=「9 月 15 日 · 没有阅读」，`differ=true`，读数随格变化

首轮成因：02-prototype.html:1205 循环内 `var s2` 闭包共享，91 格悬停全指向最后一格，实测 `{"cellAria":"7 月 28 日 · 15 分","tipText":"10 月 26 日 · 没有阅读","ariaMatchesTip":false}`。

## P1 缺陷

| # | 对象 | 问题 | 证据 | 复审状态 |
|---|---|---|---|---|
| P1-1 | 契约「反馈」列 | 31 条 click 行只有 7 条写文案原文，其余 24 条既没写文案也没声明「无文案变化」（C-01 声明了）——同一槽三种写法 | awk 统计：click 行=31 / 含时长=31 / 含文案=7 | 未解决 |
| P1-2 | C-30 契约 | 契约写「鼠标 120ms 后浮出」，原型 mouseenter 直调 showTip，无任何延迟 | 02-prototype.html:1211 | 未解决 |
| P1-3 | C-26 图例 | 契约键盘行要求「aria-pressed 同步」，原型 legend-row 无该属性，点击只改 class | 实测 ariaBefore / ariaAfter = [null,null,null]，class 变 legend-row is-off | 未解决 |
| P1-4 | 详情页署名 | 产品 BookPage.tsx:232 有 Link to=/authors，原型也是可点 a，但契约 31 个锚点里没有它 | 详情页 40 个产品级可点元素里，唯一落在契约外的就是它 | **已解决**（契约新增 C-32「详情页 · 署名作者名」，4 行齐） |
| P1-5 | 演示件语义 | demo-only: 被打在整个产品外壳上（shell-app / shell-main / page-book），「演示件隔离」判据失效：390 下不排除 demo 子树时产品控件计数为 0 | 不排除 → prod=0；只排除 demo:bar → prod=39 | **已解决**（`demo-only:shell-*` 归零，`region:` 51 处） |
| P1-6 | 390 档 | 唯一超标产品控件：详情页署名链接 36×44（高度达标、宽度不足 44） | 复测 `{"t":"林砚","id":"C-32","w":36,"h":44}`，宽仍 36 | 未解决 |
| P1-7 | C-16 契约 | 只写「按钮消失后焦点落到新出现的最后一行」，没覆盖按钮仍在（本卷还剩 85 章）时焦点去哪；实测丢到 BODY | chaptersBefore=15 → chaptersAfter=75，active=BODY，btnExists=true | 未解决 |

## P2

- 01-contract.md:5 引 web/src/lib/tier.ts:20 说明断点，实际第 20 行是 isManagementRoute；断点常量在 **tier.ts:9**（H5_QUERY = max-width 768px）
- C-26 边界写「该段仍参与总数计算（不改变数字）」，但点图例后除划掉外无「已隐藏」说明，用户无法判断总数为何不变；`04-ledger.md` 把 3 条自修项（C-12 空态粘连 / C-12 焦点 / C-18 焦点）记成 P0，实为自检期发现、非上轮评审意见，建议单列一栏

## 不符直觉清单

1. **热力图 91 格悬停全说同一句**（P0-4）。鼠标移到 7 月任意一天，浮层都回「10 月 26 日 · 没有阅读」，第一反应是这张图坏了。
2. **作者页摆着一个契约说不做的搜索框**（P0-3）。实现方读到「不加搜索框」，打开原型却看到一个能筛能用的输入框，两处口径相反，只能靠猜。
3. **图例点了，数字不动**。点「已读完 38 本」后图例划掉、环上那段变淡，但 38 与总数一字不变；用户以为过滤没生效，实际是设计如此（边界列写了），界面上却没有一句「已隐藏」的说明。
4. **折叠一卷，键盘用户被踢回页首**（P0-2）。用 Enter 折叠第一卷后焦点落到 body，下一次 Tab 从页面第一个可聚焦元素开始，浏览位置被重置。
5. **产品区块里摆着「打开删除确认（演示）」按钮**。它和「管理这本书」「加入书单」并排在同一 aside，只靠一行小字说明详情页不提供删除；契约本域边界与 BookPage.tsx:152 都明写详情页无删除入口，视觉上却给了入口。
6. **「再显示 60 章」点一次从 15 章变 75 章**，另两卷仍折叠，按钮文案只从「还有 145 章」变「还有 85 章」——一次点击插入 60 行却无位置锚定，视线丢失。

## 复审覆盖度

- 本轮只查我首轮报的 **4 条 P0 + P1-4/P1-5**（增量复审，不重跑全量）：4/4 P0 已解决、2/2 P1 已解决
- 每条的验证方式：P0-1 用 `grep -c 【新增】` + 八锚点逐条 + 边界列「现状/需」双段式抽核 3 条源码位置；P0-2/P0-4 用 agent-browser 真点取样；P0-3 用 `grep -c` 计数；P1-4 查契约新增 C-32 锚点；P1-5 查 `demo-only:shell-*` 归零、`region:` 51 处
- 没查：带 `demo-only:` 的 61 个演示锚点逐个复核、其余 25 个契约锚点的八列质量、1024/768 截图（原因：**预算**，本次上限 30 步且只做增量）；H5 真机（原因：**纪律**，模拟器为独占资源）
- 未重跑：`odops verify`（首轮 exit 0，本轮改动集中在契约文本与 4 处 JS/CSS，未新增顶层区块）

## 我剔除的误报

| 疑似 | 排除理由 |
|---|---|
| verify 报 51 个交互元素 <44px | 平台 = web，PC 口径是 ≥32×32；且多数落在 demo-only:bar。390 下只排除演示条后产品控件 39 个、超标仅 1 个（已记 P1-6） |
| verify 报 7 个 emoji | 命中 ★ / ☆ / ← / →，承担评分档位与翻页方向语义，均有 aria-label；台账已报备，换 SVG 会破坏单文件自包含 |
| 原型 grep 得 92 个唯一 data-od-id 但契约只有 31 | 差集全是 demo-only:*（61 个），SKILL 规定不计入契约；ixcheck 双向核对 31/31 通过 |
| C-16「按钮消失后焦点落到最后一行」实测焦点是 BODY | 本次实测按钮没消失（本卷还有 85 章），契约条件未触发；改记 P1-7（契约未覆盖该分支），不算违约 |
| 00-features.md 52 条源码引用抽 14 条是否对得上 | 逐行 sed -n 核对：AuthorsPage 20/29/45、BookPage 134/168/237、Charts 117/181、DashboardPage 96/172、ReadingHeatmap 132、RatingStars 13、readStatus 15、routes 38 全部命中，不记缺陷 |

## 可复跑命令

（在仓库根执行）

    node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/library --platform web; echo EXIT=$?
    ~/.local/bin/odops verify interaction/library/02-prototype.html --platform web --browser; echo EXIT=$?

    # 新增标注（复审后应为 33）
    grep -c '【新增】' interaction/library/01-contract.md                    # → 33

    # 八列质量
    awk -F'|' '/^\| C-[0-9]+ /{n++;t=$4;gsub(/ /,"",t);if(t=="click"){c++;if($6~/[0-9]+ ?(ms|秒)/)ms++;if($6~/「/)tx++}}END{print "click="c" 含时长="ms" 含文案="tx}' interaction/library/01-contract.md

    # 原型真点（书目域专用会话）
    export AGENT_BROWSER_SESSION=norvels-ixrev-lib
    agent-browser open file://$PWD/interaction/library/02-prototype.html
