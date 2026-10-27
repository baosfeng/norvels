# 03 评审 · 发现域（discovery）

> 判据：`designing-interactions` 的 `SKILL.md` + `reviewer.md` + `references/interaction-contract.md`；证据全部出自评审方自跑命令。
> 修订链：r1 FAIL → **r2 FAIL（3 条 P0）** → 返工后由 lead 复核关闭（2026-10-27）。本文只留结论、覆盖度与剩余项。

## 结论

**PASS（复核后）** —— r2 的三条 P0 已按裁决修复，lead 逐条复核：

| # | r2 问题 | 修复 | 复核证据 |
|---|---|---|---|
| P0-1 | 首页「最近入库」加载 / 错误态切不出来 | 原型补 `#home-loading` / `#home-error`，演示条可切 | `grep -c 'id="home-loading"\|id="home-error"'` → 2 |
| P0-2 | 契约承诺「命中词加粗」，产品无此能力 | 契约改为「不做关键词加粗」，原型删 `highlight()` | `grep -cE '<b>|<strong>|highlight' web/src/features/search/SearchPage.tsx` → 0 |
| P0-3 | toast 被写成可悬停 / 可聚焦 / 可键盘 | 契约只留「系统触发、不可交互」，原型删关闭按钮 | `grep -c tabindex` → 0；`grep -c toast-close` → 0 |

C-71（首页错误态「重试」）作为修复的一部分新增，锚点总数 70 → 71。

## 覆盖度

- 契约 **281 行 / 71 锚点**（评审时 70，修复新增 C-71），人工全过；focus 覆盖 70/71。
- `ixcheck` exit 0，**P1=2 / P2=1 均为已知误报**：C-17 提示条按裁决改为「不可交互」，规则仍要求 focus / hover 行；
  `placeholder` 命中的是 CSS 伪元素选择器（`ixcheck` 裸词匹配的已知缺陷）。
- `odops verify` exit 0（13 项机械检查）。
- 真点：1440 / 390 双档，角标四态、分页前后、H5 无限滚动、排序弹层 Esc、筛选与清除、1 字 / 2 字搜索、随机一本、错档重定向。
- 未覆盖：C-58 / C-59 等条件态锚点（需造态），已在 parity 里 deferred。

## 剩余项

| 对象 | 问题 | 处置 |
|---|---|---|
| 输入触发 | 四个搜索框未单列「输入」行 | 保留：输入行为写在各自 click 行的反馈与前置列，单列会与该行重复 |

2026-10-27 收口：C-54 / C-55 标注改【新增】并改指真实行号；C-17 证据改指 `layouts/ToastHost.tsx:26`；
12 条越界行号清零；契约补列约定——移除后列写「—」＝不改列表归属与计数，反馈列未写文案原文＝文案不变。

## 可复跑命令

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/discovery --platform web   # exit 0
~/.local/bin/odops verify interaction/discovery/02-prototype.html --platform web --browser              # exit 0
node scripts/check-anchors.mjs --domain discovery                                                       # 71/71
grep -c 'id="home-loading"\|id="home-error"' interaction/discovery/02-prototype.html                   # 2
```
