# 运营域（ops）实现说明 · impl-notes

> 写者：impl-ops（实现方）。设计已冻结，本文件只记录「怎么落的地」与「哪里没落」，
> 不改契约、不改原型、不改评审结论。
> 分支 `impl/ops`，基线 `f586c70`。写域：`web/src/features/{import,admin}/**`、`web/src/styles/ix-ops.css`。

## 1 结论

| 项 | 结果 |
|---|---|
| 契约锚点 | **79 / 79 全部落地**（`data-od-id="C-nn"` 逐字同键，脚本 0 缺失） |
| 【新增】控件 | **4 个全部实现**（C-12 删除这条规则 · C-13 新增一条规则 · C-73 新建卷 · C-79 新建标签） |
| 四类触发 | click / hover / focus / 键盘逐条落实；长按与滑动本域契约未要求（无对应行），未实现 |
| 移除后五问 | 逐条落实，见 §4 |
| 边界态 | 禁用一律有常驻可见原因（`.inline-note`）；空 / 加载 / 错误 / 成功四态齐备；错误保留用户输入 |
| 测试 | 定向 vitest **144 passed**；`tsc --noEmit` **0 错**；`eslint` 本域文件仅剩 1 条基线遗留（见 §6） |

## 2 锚点 → 文件:行 对照表

> 一个锚点挂一个控件节点；同一控件多处出现（如逐行勾选框 C-36、逐行操作 C-62…C-65、
> 卷上移/下移 C-75、新建卷与新建标签的「输入 + 按钮」）与 `02-prototype.html` 的粒度一致。

| 锚点 | 落点 |
|---|---|
| C-01 | `import/ImportPage.tsx:186` |
| C-02 | `import/ImportPage.tsx:200` |
| C-03 | `import/ImportPage.tsx:210` |
| C-04 | `import/ImportPage.tsx:224` |
| C-05 | `import/ImportPage.tsx:242` |
| C-06 | `import/ImportPage.tsx:252` |
| C-07 | `import/ImportPage.tsx:261` |
| C-08 | `import/UploadPanel.tsx:296` |
| C-09 | `import/UploadPanel.tsx:224` |
| C-10 | `import/ChapterRulesPanel.tsx:237` |
| C-11 | `import/ChapterRulesPanel.tsx:249` |
| C-12 | `import/ChapterRulesPanel.tsx:257` |
| C-13 | `import/ChapterRulesPanel.tsx:168` |
| C-14 | `import/ChapterRulesPanel.tsx:171` |
| C-15 | `import/ChapterRulesPanel.tsx:194` |
| C-16 | `import/ChapterRulesPanel.tsx:201` |
| C-17 | `import/TagRulesPanel.tsx:128` |
| C-18 | `import/TagRulesPanel.tsx:140` |
| C-19 | `import/TagRulesPanel.tsx:161` |
| C-20 | `import/DryRunReport.tsx:119` |
| C-21 | `import/ImportPage.tsx:294` |
| C-22 | `import/ImportProgressCard.tsx:55` |
| C-23 | `import/ImportProgressCard.tsx:95` |
| C-24 | `import/FailedFilesPanel.tsx:76` |
| C-25 | `import/ImportSuccessPanel.tsx:26` |
| C-26 | `admin/AdminFilterBar.tsx:68` |
| C-27 | `admin/AdminFilterBar.tsx:78` |
| C-28 | `admin/AdminFilterBar.tsx:96` |
| C-29 | `admin/AdminFilterBar.tsx:114` |
| C-30 | `admin/AdminFilterBar.tsx:131` |
| C-31 | `admin/AdminFilterBar.tsx:148` |
| C-32 | `admin/AdminFilterBar.tsx:160` |
| C-33 | `admin/AdminFilterBar.tsx:164` |
| C-34 | `admin/AdminFilterBar.tsx:157` |
| C-35 | `admin/AdminBooksPage.tsx:165` |
| C-36 | `admin/AdminBooksPage.tsx:322` |
| C-37 | `admin/AdminBooksPage.tsx:330` |
| C-38 | `admin/AdminBatchBar.tsx:31` |
| C-39 | `admin/AdminBatchBar.tsx:34` |
| C-40 | `admin/AdminBatchBar.tsx:37` |
| C-41 | `admin/AdminBatchBar.tsx:40` |
| C-42 | `admin/AdminBatchBar.tsx:43` |
| C-43 | `admin/AdminBooksPage.tsx:289`、`admin/AdminBooksPage.tsx:301` |
| C-44 | `admin/AdminBooksPage.tsx:269` |
| C-45 | `admin/AdminBookPage.tsx:131` |
| C-46 | `admin/AdminCover.tsx:81` |
| C-47 | `admin/AdminCover.tsx:92` |
| C-48 | `admin/AdminMetaForm.tsx:23` |
| C-49 | `admin/AdminMetaForm.tsx:174` |
| C-50 | `admin/AdminBookPage.tsx:193` |
| C-51 | `admin/DeleteBookConfirm.tsx:74` |
| C-52 | `admin/DeleteBookConfirm.tsx:57` |
| C-53 | `admin/AdminChapters.tsx:167` |
| C-54 | `admin/AdminChapters.tsx:180` |
| C-55 | `admin/AdminChapters.tsx:189` |
| C-56 | `admin/AdminChapters.tsx:199` |
| C-57 | `admin/ChapterTable.tsx:119` |
| C-58 | `admin/ChapterTable.tsx:94` |
| C-59 | `admin/ChapterBatchBar.tsx:49` |
| C-60 | `admin/ChapterBatchBar.tsx:54` |
| C-61 | `admin/ChapterBatchBar.tsx:62` |
| C-62 | `admin/ChapterTable.tsx:159` |
| C-63 | `admin/ChapterTable.tsx:168` |
| C-64 | `admin/ChapterTable.tsx:177` |
| C-65 | `admin/ChapterTable.tsx:186` |
| C-66 | `admin/ChapterTable.tsx:202` |
| C-67 | `admin/AdminBookPage.tsx:224` |
| C-68 | `admin/AdminBookPage.tsx:237` |
| C-69 | `admin/AdminBookPage.tsx:242` |
| C-70 | `admin/AdminBookPage.tsx:250` |
| C-71 | `admin/ChapterBodyDrawer.tsx:102` |
| C-72 | `admin/ChapterBodyDrawer.tsx:173` |
| C-73 | `admin/VolumeManageDialog.tsx:266`、`admin/VolumeManageDialog.tsx:277` |
| C-74 | `admin/VolumeManageDialog.tsx:214` |
| C-75 | `admin/VolumeManageDialog.tsx:224`、`admin/VolumeManageDialog.tsx:234` |
| C-76 | `admin/VolumeManageDialog.tsx:243` |
| C-77 | `admin/TagManageDialog.tsx:233` |
| C-78 | `admin/TagManageDialog.tsx:246` |
| C-79 | `admin/TagManageDialog.tsx:272`、`admin/TagManageDialog.tsx:287` |

## 3 【新增】控件清单（产品原本没有，按契约新做）

| 锚点 | 控件 | 契约要求 | 实现位置 | 用例 |
|---|---|---|---|---|
| C-12 | 删除这条规则按钮 | 行 150ms 内收起、行数减一；**保存前不生效**（撤销窗口）；焦点回「新增一条规则」；最后一条也能删，空列表显示「还没有自定义规则，导入一律走内置切章」 | `ChapterRulesPanel.tsx` `RuleRow` + `removeRow` | `ImportPage.test.tsx`「C-12 删除这条规则…」 |
| C-13 | 新增一条规则按钮 | 末尾 150ms 内出现空规则行，**焦点进入名称输入框**；满 20 条禁用并常驻「最多 20 条，先删掉用不上的再加」 | `ChapterRulesPanel.tsx` `addRow` | `ImportPage.test.tsx`「C-13 新增一条规则…」 |
| C-73 | 新建卷（卷名输入 + 新建卷按钮） | 点「新建卷」后列表 240ms 内新增一行「卷三 · 归途 0 章 空卷」并提示「已新建空卷「卷三 · 归途」」；空名「先填一个卷名。」、重名「同名卷已经有了，换个名字。」；新建出来是空卷 | `VolumeManageDialog.tsx` `.vol-new` | `VolumeUI.test.tsx`「C-73 新建卷是「输入 + 新建卷按钮」一对…」 |
| C-79 | 新建标签（名称输入 + 新建按钮） | 点「新建」后列表 240ms 内新增一行「江湖 0 本」并提示「标签「江湖」已创建。」；空名「先填一个标签名。」、重名「已经有同名标签了，换个名字再试。」 | `TagManageDialog.tsx` 新建区 | `TagManageDialog.test.tsx`「C-79 新建标签是「名称输入 + 新建按钮」一对…」 |

## 4 「移除 / 关闭后」五问自查

| 问 | 导入域 | 书目域（列表 + 单书） |
|---|---|---|
| 撤销窗口 | C-12 删除规则**不点保存就不生效**（本地草稿）；C-21 预检无写入、无需撤销；C-22 取消任务二次确认（不是撤销） | 删书 / 删章 / 删卷 / 删标签 / 批量删除一律二次确认、均不可撤销；删封面即时生效且不加确认（代价低、可重传）；批量改元信息不加确认、无撤销窗口（可再改一次） |
| 焦点去向 | C-12 → 「新增一条规则」；C-21 → 「只预检」；C-22 → 进度卡「取消任务」；C-20/C-23 收起的只是展示，焦点留在折叠条 | C-33 → 搜索框；C-42 → 表头全选；C-50 → 「删除这本书」；C-52 → 列表标题（`h1` 加 `tabIndex=-1`）；C-65 → 本行「删除」；C-70 选「留下」→ 「返回列表」；C-78 → 本行「删除」（确认期间列表只隐藏不卸载，ref 仍在） |
| 滚动位置 | 规则行删除、预检收起都不动滚动；失败/告警明细超 20 条改为**列表内滚动**（`max-height`），不推页面 | 列表操作后保持原位；C-52 删书后按契约**滚回顶部**（唯一显式要求）；C-61 删章后停在被删区间前一章 |
| 计数联动 | 已选章数、失败数、告警数、任务计数都由同一份数据派生（`pickedList` / `job.failed` / `errors.length`），不另存一份 | 已选本数在批量条与四个弹层标题里**同一来源**（`picked.size` → `AdminBatchBar`/`AdminBatchDialog`，无写死数字）；标签增删后筛选下拉与本数同步 `reload`；删书后总数减一 |
| 列表归属 | C-12 删的规则只是在编辑列表里移除，保存前不改变导入行为 | 被删的书/章/卷/标签立即移出列表；正在筛的标签被删除时**自动重置为「全部」并提示「刚才筛的标签已被删除」**；移卷只改归属，章号/正文/字数不变 |

## 5 证据（命令 + 期望输出）

```bash
# 1) 锚点对账（用主仓库新版脚本，按域扫描）
node ../norvels/scripts/check-anchors.mjs --root . --domain ops
#   ops         79/79  OK
#   ✅ 所选域全部锚点已落地        ← exit 0

# 2) 定向用例
pnpm -C web exec vitest run src/features/import src/features/admin
#   Test Files  12 passed (12)
#   Tests       144 passed (144)   ← exit 0

# 3) 类型
pnpm -C web exec tsc --noEmit -p tsconfig.json
#   （无输出）                     ← exit 0

# 4) 静态检查（本域两个目录）
pnpm -C web exec eslint src/features/import src/features/admin
#   ✖ 1 problem（1 error）：AdminBookPage.test.tsx:25 '_step' 未使用
#   ← 该条是基线 f586c70 就有的（git show f586c70:… | sed -n '25p' 同键），非本轮引入，
#     且 `pnpm lint` 全仓另有 11 条同类基线问题（reader / shelf / book 域），留给各自域收口。
```

契约文案一致性抽查（本域新增/改写的文案，无实现词）：

```bash
grep -rn "SQLite\|localStorage\|后端\|端点\|bookId" web/src/features/import web/src/features/admin --include=*.tsx | grep -v "\.test\."
#   仅命中注释里的「后端」与字段名，界面文案 0 命中
```

## 6 未覆盖 / 偏差（逐条给原因）

| # | 项 | 状态 | 原因与建议 |
|---|---|---|---|
| 1 | **C-01 与 C-06 的冲突** | 偏差（已选一边） | C-01 边界写「空值提交时按钮禁用」，但 C-06 边界写「路径为空时点击提示「先填一个目录或 txt 文件路径。」」——按钮禁用就点不成。按 C-06（可点 + 提示）实现，同时在输入框下常驻同一句原因。建议作者在 C-01 边界里改成「按钮保持可点，点了给提示」。 |
| 2 | **C-57 与 C-59/C-60/C-61 的冲突** | 偏差（已选一边） | C-57 写「取消勾选后批量条收起」，但三个批量按钮的边界要求「未选章时禁用并旁注常驻」。若整条收起，按钮与旁注都不存在。实现为：未勾选时批量条保留、按钮禁用、原因常驻「先勾选章节，才能移动或删除」，勾上即解锁（与 `03-review.md` P0-1 的复验口径一致）。建议作者在 C-57 改口径为「收起为提示态」。 |
| 3 | **C-27 / C-28 分类筛选** | 已实现但有前提 | 列表接的是既有分类树（两级）；没有分类数据时两个下拉只剩「全部」且保持可用。小分类在未选大分类时禁用并常驻「先选一个大分类」——契约只写「未选大分类时显示「全部」」，未写是否禁用，按「可解释」判据补了原因。 |
| 4 | **C-29 作者筛选的「佚名」归并** | 部分 | 下拉里作者带本数（`作者名（N 本）`）；「没有作者的书归到「佚名」」由数据层决定，界面只渲染拿到的作者项。若需要「佚名」作为一个可选筛选项，要接口先能按空作者筛。 |
| 5 | **C-14 / C-19 的 4 秒消失提示** | 已实现 | 用 `useTimedNote`（4000ms）。vitest 未逐条断言计时，靠「提示出现即通过」覆盖（假计时器断言会把用例绑在毫秒上）。 |
| 6 | **长按 / 滑动拖拽触发** | 不适用 | 本域 237 行契约只有 click / hover / focus / 输入四类；上传区做了原生拖入（dragover/drop）与键盘 Enter/Space 等价入口。 |
| 7 | **parity 数值对表** | 未跑（按 lead 安排） | 由 lead 用固定夹具统一收口；本域只保证锚点齐、触发实现、文案一致。 |
| 8 | **`pnpm lint` 全仓 12 条存量报错** | 非本域 | 11 条在 reader / shelf / book / layouts 域（含 `AppShell.tsx` 的 rules-of-hooks），1 条是本域测试文件的基线遗留 `_step`。未顺手改（禁止顺手重构无关代码）。 |
| 9 | **`scripts/check-anchors.mjs` 本 worktree 是旧版** | 需共享改动 | 旧版把四域锚点混算（各域 C-01 独立编号会互相顶掉）。本轮统一用主仓库新版 `node ../norvels/scripts/check-anchors.mjs --root . --domain ops`；worktree 里那份随主分支合入即可，本域未改 `scripts/`。 |

## 7 需要的共享改动（本域未越界，交 lead）

| # | 文件 | 要改什么 | 为什么 |
|---|---|---|---|
| 1 | `scripts/check-anchors.mjs` | 同步主仓库的「按域扫描」版本 | 旧版全局混算，四域独立编号会互相判缺失（本域未改 `scripts/`） |
| 2 | `web/src/features/book/DeleteBookDialog.tsx` | **删除**（已无引用） | 它唯一的调用方是 `AdminBookPage`，本轮改用本域的 `web/src/features/admin/DeleteBookConfirm.tsx`（契约 C-51/C-52 的锚点与文案只能落在 ops 写域内）。文件本身在 `features/book/**`＝书目域写域，故本域未动；留着即死代码 |
| 3 | `web/src/lib/volumeOps.ts` 的 `VOLUME_NAME_TAKEN_TEXT` / `VOLUME_NAME_EMPTY_TEXT` | 可选：把常量文案改成契约口径（重名「同名卷已经有了，换个名字。」、新建空名「先填一个卷名。」） | 契约 C-73/C-74 的文案与 lib 常量不一致；本域在 `VolumeManageDialog` 里做了一层映射（不改 lib），若 lib 也改则映射可删 |
| 4 | `web/src/features/admin/AdminBookPage.test.tsx:25` | 把 `_step` 改成不用参数或加 eslint-disable | 基线遗留的 lint 报错，本域未顺手改 |
| 5 | 契约侧（作者） | §6 的 #1、#2 两处口径冲突 | 实现方不重新做设计判断，只按「更具体的一条 + 评审复验口径」落地并在此记录 |

## 8 变更清单（本轮新增/改写的文件）

新增：`features/import/{ImportProgressCard,ImportSuccessPanel,ImportHistory,useTimedNote}`、
`features/admin/{AdminFilterBar,AdminBatchBar,batchLimit,DeleteBookConfirm}`、`styles/ix-ops.css`（内容）。
改写：导入页四个面板（`ImportPage` / `UploadPanel` / `ChapterRulesPanel` / `TagRulesPanel` / `DryRunReport` / `FailedFilesPanel`）、
书目两个页面与八个弹层（`AdminBooksPage` / `AdminBookPage` / `AdminMetaForm` / `AdminCover` / `AdminChapters` /
`ChapterTable` / `ChapterBatchBar` / `ChapterBodyDrawer` / `VolumeManageDialog` / `TagManageDialog`）。

测试改动（均为对齐已冻结契约，不是「改测试掩盖缺陷」）：

| 用例 | 改了什么 | 依据 |
|---|---|---|
| `ImportPage.test.tsx` 取消任务两条 | 先点「取消任务」再点「确认取消」 | C-22 要求二次确认 |
| `ImportPage.test.tsx` 非法正则一条 | 「没闭合」与「写得不对」分成两句断言 | C-11 要求未闭合单独一句 |
| `ImportPage.test.tsx` 预检一条 | 先点折叠条再断言告警明细 | C-20 要求告警默认收起 |
| `TagRulesPanel.test.tsx` 一条 | 断言「词表没通过校验」 | 禁实现词（原文含「后端」） |
| `AdminChapters.test.tsx` / `VolumeUI.test.tsx` 批量条相关 | 「未勾选时整条不渲染」→「未勾选时禁用 + 常驻原因」 | C-59/C-60/C-61 边界 |
| `VolumeUI.test.tsx` 卷名两条 | 空名/重名改用契约原话 | C-73/C-74 |
| `AdminBooksPage.test.tsx` | mock 里补 `useCategories` / `useAuthors` | 列表新增两级筛选与作者筛选（C-27/C-28/C-29） |
