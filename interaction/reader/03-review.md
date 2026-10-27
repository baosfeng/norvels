# 03 评审 · 阅读域（reader）

> 判据：`designing-interactions` 的 `SKILL.md` + `reviewer.md` + `references/interaction-contract.md`；证据全部出自评审方自跑命令。
> 修订链：r1 FAIL → r2 FAIL → **r3 PASS-WITH-TODO**（2026-10-27）。本文只留结论、覆盖度与剩余项，过程记录已归档。

## 结论

**PASS-WITH-TODO** —— 三轮评审的 P0 全部关闭（抽屉出屏、四个「标现有但产品没有」的能力、C-58 一族撤销口径、
C-59/C-60 编造行号、C-61/C-62 假渲染、长按残留），可放行开发且已实现。剩余均为契约文本质量问题，不影响用户行为。

## 覆盖度

- 契约 **252 行 / 62 锚点**，人工全过 62，跳过 0；focus 覆盖 62/62。
- `ixcheck` exit 0（P0=0/P1=0/P2=0）；`odops verify` exit 0，13 项 ok / 1 warn（12px 元信息，保留）。
- 真点 5 组共 14 个状态读数：确认框、目录失败重试（真的转 loading）、平铺（真的隐藏卷头）、搜索空态、恢复提示。
- 未覆盖：夜间 / 羊皮纸主题、1024 断点（预算）；这两项由实现侧的样式用例兜底。

## 剩余项

| 对象 | 问题 | 处置 |
|---|---|---|
| C-16 演示档 | 「无上次记录」分支无法在原型演示 | 保留（不影响产品） |

2026-10-27 收口：字号 15–26 → 14–28、行距 1.3–2.2 → 1.4–2.4 已回填契约；C-13 改名「返回书籍详情」
（产品按钮文案同步改）；C-58 结果列改为系统结果；目录空态在原型拆成「还没有目录 / 没有匹配的章节」两块。

## 可复跑命令

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/reader --platform web   # exit 0
~/.local/bin/odops verify interaction/reader/02-prototype.html --platform web --browser              # exit 0
node scripts/check-anchors.mjs --domain reader                                                       # 62/62
```
