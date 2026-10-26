# 运营域（ops）返工台账

> 一条意见一行，状态只有「已改」/「保留（理由）」。意见来源：`03-review.md`（评审）；首轮尚无评审时以 `ixcheck` 的 P0/P1/P2 为准。

| # | 轮次 | 意见（原话摘要） | 级别 | 处理 | 改在哪 | 状态 |
|---|---|---|---|---|---|---|
| 1 | r0 | 旧版 `docs/ux/ops.md` 两条 P0：单书页状态下拉无效（无三态面板）、缺「新建章节」入口说明 | P0 | 新版契约补齐 `#bookStates` 三态与 C-54 新建章节；upgrade 时随原型迁移，不回退 | 02-prototype.html `#bookStates` / 01-contract.md C-54 | 已改 |
| 2 | r1 | `ixcheck` T020：C-12「删除这条规则」既无撤销也无二次确认 | P0 | 「移除/关闭后」写明撤销窗口：不点「保存规则」不生效，重加或放弃编辑都能复原 | 01-contract.md C-12 | 已改 |
| 3 | r1 | `odops verify --browser`：11 个交互元素 <44px（演示条三页签、状态下拉、PC/H5 分段、侧栏导航、勾选框） | P1 | 统一提到 `min-height:44px`（勾选框命中区 44、视觉仍 20）；复测 under44=0 | 02-prototype.html CSS | 已改 |
| 4 | r1 | 五态：契约每锚点写 click / hover / focus 三行，未单列「按下」「选中」 | P2 | 保留：按下态与选中态并入 click 行的反馈与边界（按钮按下态在 CSS `:active` 有定义，选中态在勾选类行写明半选/全选）；单列会让 79 个锚点再涨 158 行而信息重复 | 01-contract.md 说明 | 保留（理由） |
| 5 | r2 | P0-1：C-60 移动到卷 / C-61 删除所选前置写「已选至少 1 章」，但未选章时 `disabled=false`，同组只有合并做了禁用 | P0 | 两个按钮加 `disabled` 并在未选章时保持禁用；旁注 `.inline-note` 常驻「先勾选章节，才能移动或删除」；勾 1 章后三按钮同时解锁。契约边界列同步 | 02-prototype.html `#chMoveBtn` / `#chBatchDeleteBtn` / `#chBatchHint`；01-contract.md C-59·C-60·C-61 边界列 | 已改 |
| 6 | r2 | P0-2：批量删除弹层「永久删除 2 本」写死；标题只有 `#batchDelCount` 动态，正文与按钮写死 | P0 | 标题「删除这 N 本书？」、正文「这 N 本书及其全部章节将被永久删除」、按钮「永久删除 N 本」三处统一由 `syncBatchDelete()` 填充；勾 1 本三处都是 1、勾 3 本都是 3；顺带删掉挂在数字 span 上的空 `addEventListener` 死代码 | 02-prototype.html `#batchDelTextCount` / `#batchDelBtnCount` / `syncBatchDelete()` | 已改 |
| 7 | r2 | P1-3：「禁用」二字出现 172 行但只有 9 行写原因 | P1 | 逐条补「为什么禁用 + 旁边常驻什么文案」；复测含「禁用」的 175 个边界格全部带可见原因（175/175） | 01-contract.md 边界列 | 已改 |
| 8 | r2 | P1-1：反馈列缺时长 8 条（C-09 / C-22 / C-24 / C-46 / C-52 / C-58 / C-68 / C-72） | P1 | 每条补文案切换时长（100ms 进入加载态、240ms 呈现结果、4000ms 消失）；复测 79 个 click 行全部含时长 | 01-contract.md 反馈列 | 已改 |
| 9 | r2 | P1-2：有移除语义 18 行里只有 4 行覆盖 ≥3 问 | P1 | 按撤销窗口 / 焦点去向 / 滚动位置 / 计数联动 / 列表归属 五问补齐；复测 18/18 覆盖 ≥3 问 | 01-contract.md 移除/关闭后列 | 已改 |
| 10 | r2 | P1-4：`routes.tsx` 行号系统性偏 5 行（:36 实为 :41、:48 实为 :58、:52 实为 :66） | P1 | 用 `grep -n` 全量重核 30 条证据行号，不沿用旧表；`routes.tsx` 与各组件行号已更新 | interaction/ops/00-features.md | 已改 |
