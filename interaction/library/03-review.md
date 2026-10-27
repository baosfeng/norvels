# 03 评审 · 书目域（library）

> 判据：`designing-interactions` 的 `SKILL.md` + `reviewer.md` + `references/interaction-contract.md`；证据全部出自评审方自跑命令。
> 修订链：r1 FAIL → **r2 增量复审 PASS-WITH-TODO**（2026-10-26 晚）。本文只留结论、覆盖度与剩余项，过程记录已归档。

## 结论

**PASS-WITH-TODO** —— 4 条 P0 全部解决：八个「产品里不存在」的交互补了【新增】与「现状 / 需」双段式、
C-14 卷折叠焦点回卷头、作者页多余搜索框删除、热力图 91 格读数闭包 bug 修复。P1-4 / P1-5 同时解决。
可放行开发且已实现；剩余项为契约文本与原型细节。

## 覆盖度

- 契约 **129 行 / 32 锚点**（评审时 31，实现期新增 C-32），人工全过，跳过 0；focus 覆盖 32/32。
- `ixcheck` exit 0（P0=0/P1=0/P2=0）；`odops verify` exit 0，12 项 ok / 3 warn（emoji、44px、12px 小字，逐条已核）。
- 真点 14 项：评分星清零、卷折叠、目录搜索、无匹配空态、清空搜索、再显示 60 章、删除弹层、热力格悬停、图例过滤、作者页、状态四档、390 三页档位。
- 源码抽检 23 条（契约 9 + 00-features 抽 14），逐条 `sed -n` 核对。
- 未覆盖：1024 / 768 单独截图（预算）、H5 真机（模拟器独占资源，按纪律不占）。

## 剩余项（不阻塞交付）

| 对象 | 问题 | 处置 |
|---|---|---|
| C-07 保存中 | 契约「可聚焦但不可改」，原生 select 做不到 | 已按 `disabled` 实现，见 [../IMPLEMENTATION-NOTES.md](../IMPLEMENTATION-NOTES.md) §2 |
| C-29 提示时长 | 契约 2 秒，实现用全站 4 秒 | 已接受，见同上 §2 |
| C-13 卷门槛 | 契约「3 卷及以上」，实现 2 卷即出 | 已接受，见同上 §2 |
| C-30 / C-26 | 原型悬停无 120ms 延迟、图例缺 `aria-pressed` | 原型细节，产品行为不受影响 |
| 反馈列 / 行号 | 31 条 click 行只有 7 条写文案原文；`tier.ts` 行号偏移 | 契约文本质量，随下次改契约处理 |

## 可复跑命令

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/library --platform web   # exit 0
~/.local/bin/odops verify interaction/library/02-prototype.html --platform web --browser              # exit 0
node scripts/check-anchors.mjs --domain library                                                       # 32/32
```
