# 03 评审 · 运营域（ops）

> 判据：`designing-interactions` 的 `SKILL.md` + `reviewer.md` + `references/interaction-contract.md`；证据全部出自评审方自跑命令。
> 修订链：r1 FAIL → **r2 增量复审 PASS**。本文只留结论、覆盖度与剩余项，过程记录已归档。

## 结论

**PASS** —— 2 条 P0（未选章时批量按钮可点且弹层自报「0 章」；批量删除弹层正文与按钮写死「2 本」）全部解决，
4 条 P1 全部解决；回归抽查 5 项首轮通过点确认未被返工改坏。可放行开发且已实现。

## 覆盖度

- 契约 **237 行 / 79 锚点**，79 个全过（ixcheck 双向 79/79）；focus 覆盖 79/79。
- `ixcheck` exit 0（P0=0/P1=0/P2=0）；`odops verify` exit 0，12 项 ok / 1 warn（12px 小字，保留）。
- 真点 12 组：三页 × 正常 / 加载 / 空 / 错误、新建章节、拆分、合并、卷管理、标签管理、批量删除、脏数据离开三出口、H5 390。
- 源码抽检 30 条（00-features 的引用逐条 `sed -n`），命中 27 / 偏 3，返工后已改为 41 / 58 / 66 行。
- 未覆盖：1024 / 768 单独截图（预算）、H5 真机（按纪律不占）。

## 剩余项

| 对象 | 问题 | 处置 |
|---|---|---|
| C-47 移除封面 | 单击即生效、无确认无撤销 | 保留（已报备取舍：代价低、可重传） |

2026-10-27 收口：C-01 空值提交改为「可点 + 提示」、C-57 取消勾选后批量条保留为禁用态、C-42 / C-44 边界列
补禁用条件、C-73 / C-79 写明空名提示在点按后才出现；C-12 / C-13 / C-73 / C-79 补【新增】标记。

## 可复跑命令

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/ops --platform web   # exit 0
~/.local/bin/odops verify interaction/ops/02-prototype.html --platform web --browser              # exit 0
node scripts/check-anchors.mjs --domain ops                                                       # 79/79
```
