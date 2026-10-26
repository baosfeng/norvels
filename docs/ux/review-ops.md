# 运营域（ops）评审报告

> 评审对象 `docs/ux/ops.md` · `.open-design/ux/ops.html` · `shots/ops-1440.jpg` / `ops-390.jpg`。
> 判据 = `docs/ux/README.md` §6 冻结清单 12 条（P0 8 / P1 4），不另立标准。
> 全部结论来自本轮自跑命令（`agent-browser --session norvels-ux-review-ops` 无头会话 + `odops verify` + `grep`/`sqlite3`）；作者自述不作证据。
> 会话：2026-10-26，仓库根目录。

## 1 十二条判据逐条结论

| # | 级别 | 判据 | 结论 | 证据 |
|---|---|---|---|---|
| 1 | P0 | 每个可交互元素都在文档出现且有标签 | **不过** | 原型「新建章节…」按钮运行时可见、可开弹层，文档全文 `grep -c '新建章节' docs/ux/ops.md` = 0；标签列 79 行齐备（见 §3①②） |
| 2 | P0 | 每元素写全触发/结果/失败表现 | **不过** | `book:chapters` 一行只写「滚动 → 按卷分组」，但该锚点下挂着「新建章节…」主流程按钮（触发/结果/状态/撤销四项全缺） |
| 3 | P0 | 能打开 · 主流程点得通 · 状态可切 · 1440/390 无溢出 | **不过** | 打开/切页/抽屉/弹层均通、两档 `overflowX=0`；但单书页状态下拉无效（见 §2 P0-1） |
| 4 | P0 | 加载/空/错误/禁用/选中齐全 | **不过** | 导入页 4 态 + 列表页 4 态可切；单书页 `[data-state]` 面板 = 0（7 个面板全属 `#importStates`/`#adminStates`） |
| 5 | P0 | 文档每条交互在原型找得到锚点（双向对表） | **通过** | `doc=79 proto=79`，`comm -23`/`comm -13` 双向均空（§3③） |
| 6 | P0 | 无实现词外露（SQLite/FTS/API/bookId/localStorage/虚拟滚动/SHA-256/用时） | **通过** | 原型扫描命中 0；文档 3/72 行 `:bookId` 与 README §7 索引写法一致，116 行「端点/后端」不在禁词表 → 记 P2 |
| 7 | P0 | 无真实书名/作者/本机路径 | **通过** | 5 个示例书名对 `data/norvels.db` `select count(*) … in (…) ` = **0**；`林砚` 交集 0；`佚名` 为通用占位符、`.privacy-denylist.txt` 命中 0 |
| 8 | P0 | 触控目标 H5 ≥44×44 · 键盘可达 · 焦点环可见 | **通过** | H5 产品区 5 个控件 `minH=44`、`under44=[]`；可聚焦元素 41；焦点后 `box-shadow: rgba(155,91,50,.24) 0 0 0 4px`（`--focus-ring`）。PC 侧 <32×32 记 P1 |
| 9 | P1 | 悬停/聚焦/按下/禁用/选中 五态对 PC 元素有定义 | **不过** | 79 行中含「悬停」仅 7 行、四态齐（悬停+聚焦+按下|选中+禁用）仅 1 行 |
| 10 | P1 | 破坏性操作二次确认 + 能否撤销 | **部分不过** | §5 六行覆盖删书/批量删/删章/删卷/删标签/取消导入；`admin:batch-tag` 覆盖既有标签属破坏性，无确认、§5 未列（见 §2 P1-2） |
| 11 | P1 | 手势有非手势替代 | **通过** | 管理域无长按/滑动；`import:upload-zone` 拖拽另有「单击选文件 + Enter/空格」 |
| 12 | P1 | 空态给下一步 · 错误可重试 | **部分不过** | 列表空态「去导入」、搜索无匹配「清除筛选」、导入错误「重试」均在；单书页错误态无面板可演示 |

**统计：P0 4 条不过（1·2·3·4）· P1 3 条不过（9·10·12）· 12 条通过 5 条（5·6·7·8·11）。**

## 2 缺陷表

### P0（阻塞交付）

| # | 对象 | 问题 | 证据（命令 + 输出行） | 建议改法 |
|---|---|---|---|---|
| P0-1 | `ops.html` 单书管理页 / 判据 3·4·12 | 状态下拉在单书页完全无效：切到「加载中/错误」后主区元素从 23 个锚点掉到 12 个、正文整块消失，但没有任何三态面板、没有「正在读取」字样、没有「重试」。文档 §4 却写了 `book:ch-body`「加载中 / 错误（可重试）」、`book:ch-split`「加载中 / 错误」、`book:vol-manage-dialog`「加载中 / 错误」→ 文档↔原型对不上 | `applyState()` 只认两页：`var host = page === 'import' ? $('#importStates') : (page === 'admin' ? $('#adminStates') : null);`（ops.html:1190）；`grep -c 'bookStates' ops.html` = **0**；实测 `{normal:{anchors:23,panels:7}, loading:{anchors:12,textHasSpinner:false,anyRetry:false}, error:{anchors:12,anyRetry:false}}` | 给 `#bookStates` 补 loading/empty/error 三块 `data-state` 面板（复用 `components/StatePanel` 形态），并让 `applyState()` 对 `book` 页也挂 host；或删掉文档 §4 里无法演示的状态词 |
| P0-2 | `ops.html` + `ops.md` / 判据 1·2 | 「新建章节…」是单书页主流程入口，原型里可点、能开弹层（章节标题 / 插入位置 / 正文三字段），但文档里完全没有它：全文 `grep -c '新建章节' docs/ux/ops.md` = **0**，触发/结果/状态/撤销四项全缺 | 原型：`ops.html:784 <button class="btn btn-sm" type="button" data-dialog="#dlgNewChapter">新建章节…</button>`，运行时锚点解析为 `book:chapters`；源码侧入口 `web/src/features/admin/AdminChapters.tsx:181`；文档该行只有「滚动 → 按卷分组…撤销 —」 | 文档新增一行 `book:ch-new` 新建章节（触发：单击弹层；结果：标题/插入位置/正文提交后入表；状态：默认/处理中/标题或正文超限被拦；撤销：取消不改动），原型同步给改按钮加该锚点 |
> 只列 2 条。`admin:batch-tag` 的不可逆覆盖面按 P1 记（见 P1-2），不上 P0 —— 避免误报。

### P1（随交付记待办）

| # | 对象 | 问题 | 证据 | 建议改法 |
|---|---|---|---|---|
| P1-1 | `ops.md` 全表 / 判据 9 | 五态覆盖太薄，PC 元素没有系统性给悬停/聚焦/按下/禁用/选中 | `awk -F'|' '$5 ~ /悬停/' docs/ux/ops.md | wc -l` = **7**/79；四态齐 = **1**/79 | 至少对表格里的按钮类元素（`import:start`、`admin:batch-*`、`book:save`、`book:ch-*`）补齐五态；纯展示行可标「—」并注明不适用 |
| P1-2 | `ops.md:64` `admin:batch-tag` | 批量加标签会**替换**这些书已有标签（留空即清空，不可逆），撤销列只写「取消」，§5 破坏性一览表 106–111 行未收录该行 | `grep -n '批量加标签' docs/ux/ops.md` 仅命中 64 行；§5 无对应行 | 二选一并写明：①加二次确认（列出 N 本与被清空的旧标签）；②在撤销列明说「旧标签不可恢复」 |
| P1-3 | `ops.md` §2 / 判据 12 | 预检（dry-run）超时的用户可见表现没写：文档 30/31 行只写「错误」「可重试」，未写服务端 60 秒上限、超时回 503、以及超时后该做什么 | `web/src/api/client.ts:65`「只预检是同步扫描（后端上限 60 秒，超时回 503）」；`ImportPage.tsx:132` 同口径；原型 `import:dry-report` 错误态无超时文案分支 | `import:dry-report` 错误态补一句「预检超过 60 秒会中断，可缩小目录范围重试」，并在撤销列给「重新预检」 |
| P1-4 | `ops.html` PC 档 / §3 分档规则 | PC 可点区 8 处 <32×32，最小 19px 高 | 实测 `{total:119, under32:8}`，样本：面包屑 `书籍管理:48x19`、`从这里拆开:422x30` ×4 | 拆分点条与面包屑补 `min-height:32px`（PC 分档规则要求 ≥32×32） |

### P2（进报告不进修复轮）

| # | 对象 | 问题 | 证据 |
|---|---|---|---|
| P2-1 | `ops.md:116` | 差异清单用「分类聚合端点」「后端补分类查询参数」，属实现侧口吻；三域里 reader/discovery 均 0 次「端点/后端」 | `grep -c '端点\|后端' docs/ux/{reader,discovery}.md` = 0 / 0 |
| P2-2 | `ops.md:3`·`72` | `:bookId` 是 README §7 规定的路由写法，不判外露；但与另两域（0 次）风格不一致 | `grep -c 'bookId' docs/ux/{reader,discovery}.md` = 0 / 0 |

## 3 必做复核（leader 抽检点）

**① 7 列齐备（缺列计数 = 0）**
```bash
awk 'BEGIN{FS="|"} /^\| `?[a-z]/ {print NR": cols="NF-2}' docs/ux/ops.md | grep -vc 'cols=7'
```
79 行全部 `cols=7`，缺列计数 **0**。

**② 「撤销」列对破坏性操作的可执行口径**
| 破坏性操作 | 文档撤销列口径 | 判定 |
|---|---|---|
| 删除本书（81 行 + §5） | 「取消；确认后不可恢复」 | 可执行 |
| 删除单章 / 删除所选（89/95 行 + §5） | 「无法撤销；后续章号自动补上」 | 可执行 |
| 删除卷（§5） | 「卷本身不可撤销；章节正文都还在」 | 可执行 |
| 删除标签（§5） | 「不能恢复，书本身不受影响」 | 可执行 |
| 批量删除（66 行 + §5） | 「无法撤销；未成功的可重试」 | 可执行 |
| 批量改元信息（118 行） | 「不弹确认，结果里给成功/失败本数与逐条重试」 | 可执行（与源码一致） |
| **批量加标签（64 行）** | 「取消」 | **不可执行** → P1-2 |
| 封面移除（77 行） | 「只能重新上传（代价低，故不拦）」 | 可执行（差异表 117 行已报备） |
| 取消导入任务（§5） | 「可重新开一次任务」 | 可执行 |

**③ 与源码差异清单已落文档末尾** — 文档 113–118 行，3 行表格齐全：大分类/小分类/作者三筛选（116 行）、封面移除确认（117 行）、批量改元信息确认（118 行）。核对源码：`AdminBooksPage.tsx` 筛选只有 `admin-search` / `admin-filter-serial` / `admin-filter-tag`（101·118 行两处 `<select>`），`grep 'category\|authorId' AdminBooksPage.tsx` 无筛选参数 → 文档描述属实；`AdminCover.tsx:49 async function remove()` 直接 `api.deleteCover` 无确认 → 属实；`AdminBatchDialog.tsx` 无 `window.confirm`（181 行 `confirm-actions` 是弹层按钮，非二次确认）→ 属实。

**④ H5 390 下三页确实不渲染（DOM 计数）**
```bash
AGENT_BROWSER_SESSION=norvels-ux-review-ops agent-browser set viewport 390 844 && agent-browser eval --stdin <<'EOF'
... querySelectorAll('[data-od-id="ops:import|ops:admin|ops:book|ops:main|ops:sidebar"]')
EOF
```
输出：`{"vw":390,"overflowX":0,"ops:import":[],"ops:admin":[],"ops:book":[],"ops:main":[],"ops:sidebar":[],"ops:app":[{"hidden":true,"w":0}],"ops:h5":[{"hidden":false,"w":390}],"visibleAnchors":["demo:bar","demo:page","demo:state","demo:tier","ops:h5"]}` → 三个管理页在 390 下 **DOM 里根本不存在**（不是 `display:none`），符合 README §3。实现见 `setTier()` 把 `#pcApp` 的 `firstChild` 整体搬进 `DocumentFragment`（ops.html:1213–1228）。

**⑤ 导入页三种异常的用户可见表现**
| 异常 | 文档 | 判定 |
|---|---|---|
| 409 并发冲突 | 29 行 `import:conflict`：「已有任务在跑（任务号 + 开始时间），等它结束再开；并指出只预检不占这次机会」，状态「错误（常驻直到条件消失）」 | ✅ 实测原型切「操作冲突」后该块 `hidden=false, w=1148`，文案含任务号 |
| 上传格式被拒 | 35 行 `import:upload-list`：「.pdf 不支持 / 超 64 MB / 这批超 256 MB / 多于 20 个」，逐条说明 | ✅ 与 `UploadPanel.tsx:38·43·48·71` 文案一致 |
| 预检超时 503 | 30/31 行只写「错误」「可重试」，未写 60 秒上限与 503 | ❌ → P1-3 |

## 4 锚点双向对表（判据 5）

```bash
grep -o 'data-od-id="[^"]*"' .open-design/ux/ops.html | sed 's/data-od-id="//;s/"//' | sort -u   # 78 静态
# 运行时打开全部弹层 + 切三页后收集：79
grep -o '\`[a-z]*:[a-z0-9-]*\`' docs/ux/ops.md | tr -d '\`' | sort -u                    # 79
```
`doc=79 proto=79`；`comm -23`（文档有原型无）= 空、`comm -13`（原型有文档无）= 空 → **双向全对**。
静态 grep 少 1 个是因为 `book:body-drawer` 由脚本动态注入（`ops.html:1513 d.setAttribute('data-od-id','book:body-drawer')`），实测点「正文」后 `{tag:"ASIDE",hidden:false,rect:620}` 存在 → **本项不计缺陷**。
反向抽查 10 个原型锚点均在文档有解释：`ops:app`(8) `import:conflict`(29) `import:dry-report`(31) `import:chapter-rules`(37) `admin:filter-category`(54) `admin:batch-failures`(68) `book:cover-remove`(77) `book:ch-split`(94) `book:leave-guard`(99) `book:tag-manage`(101)。

## 5 我排除的误报（不计缺陷）

| 疑似问题 | 排除理由 |
|---|---|
| 文档写「79 个锚点」但 `grep -o` 得 81 次 | `import:conflict` 出现 4 处（含 JS 选择器）；唯一锚点数运行时 = 79，文档数字**准确** |
| 静态 grep 里 `book:body-drawer` 不存在 | 动态注入，见 §4 |
| `odops verify --browser` 报「39 个交互元素 <44px（触屏平台是 P0）」 | `platform=web`，且 H5 产品区实测 `minH=44`；报出的 32px 全是 `demo:bar` 演示件（不进产品） |
| 原型锚点(79) 远小于源码 `data-testid` 唯一数(224) | 口径不可比：源码 testid 含测试钩子与行内重复按钮（`AdminChapters` 每行 4 个操作按钮共用一个 testid 前缀），README §1 的「覆盖度计数」不是这个口径 → 不据此判 P0 |
| 示例作者「佚名」与库中 436 本同名 | 图书馆通用占位符，非真实作者；`.privacy-denylist.txt` 命中 0 |
| `book:meta` 等行状态列写「默认 / 聚焦 / 脏 / 保存中」不像五态 | 属单书页表单语义态，判据 9 只要求 PC 元素有定义 —— 已并入 P1-1 统计，不另开条目 |

## 6 覆盖度与未覆盖

- `odops verify .open-design/ux/ops.html --platform web --browser` → **exit 0**，10 项 ok（自包含 / token / 无裸色值 / 断点齐 / **无横向溢出** / 对比度达标），2 项 warn（32px 演示件、12px 小字）。
- 断点：1440 与 390 各实测一次（`overflowX=0`/两档），1024 与 768 仅由 verify 声明「断点齐全」，**未单独截图核对**。
- 交互实跑：切三页、开正文抽屉、开新建章节弹层、状态五值切换（normal/loading/empty/error/conflict）均在 1440 实测。
- 截图 `shots/ops-1440.jpg`（104 KB）、`shots/ops-390.jpg`（36 KB）均在 600 KB 内。
- **未跑**：H5 真机/模拟器（按纪律不占独占资源）；键盘 Tab 全链路逐段落位（只验证了可聚焦元素数 41 与焦点环样式）。
