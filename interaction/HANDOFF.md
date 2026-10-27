# 交接 · 交互契约实施（2026-10-27 晨）

> 设计在 `interaction/`（冻结），实现已合并到 `main`。本文只写「做完什么、证据、还差什么」。

## 1 一页结论

四域交互契约已按锚点实现并合并到 main：

| 域 | 锚点 | 新增控件 | 定向测试 | 状态 |
|---|---|---|---|---|
| reader | 62/62 | 10 | 308 通过 | 已合并 |
| discovery | 71/71 | 2 | 域内全绿 | 已合并 |
| library | 32/32 | 9 | 137 通过 | 已合并 |
| ops | 79/79 | 6 | 域内全绿 | 已合并 |

全仓前端：**75 文件 / 678 用例通过**；`typecheck` 0 错；`lint` 0 error（8 条既有 react-refresh warning）。
全量 `make gate`（无并发服务复跑）：**九项全绿**，含隐私扫描、构建一致性与前端 vitest。
锚点静态对账：`node scripts/check-anchors.mjs` → **244/244 OK**。

## 2 怎么验

```bash
node scripts/check-anchors.mjs          # 244/244，exit 0
cd web && pnpm test && pnpm typecheck && pnpm lint
cd .. && make gate                      # 九项（含 Go / Gherkin / 隐私 / 前端）
```

prd parity：`scripts/parity-fixture.sh all`（12 本虚构书 → /tmp，后端 :8099 + Vite :5173）；
逐页对账与 1440/390 截图见 `/tmp/norvels-parity/`，操作手册见 [PARITY.md](PARITY.md)。

## 3 实现要点

- 锚点：每域 `data-od-id="C-nn"` 与契约逐字相同；旧描述性 `data-od-id` 保留，二者并存。
- 触发：click / hover / focus / 键盘逐条；触屏长按/滑动按契约；禁用原因用常驻 `.inline-note`。
- 移除后五问：焦点回触发元素、滚动保持、计数联动、列表归属、撤销/二次确认按冻结决策。
- 演示件：`web/src` 中 `demo-only` 命中 0。
- 共享样式：`.inline-note`、按下态、H5 44px 命中区集中在 `globals.css`；四域各一份 `ix-*.css`。

## 4 parity 结果（未通过，如实记录）

合并后最终运行 13 页：9 页拿到数据、4 页工具无输出；合计 P0 231 / P1 456。
P0 分类：color 67 / tap-target 52 / radius 50 / font-size 48 / anchor-missing 45 / background 35。
- 样式差异（八成以上）：原型与实现的字号 / 颜色 / 圆角 / 底色；tap-target 按触屏 44 判，PC 口径 32。
- anchor-missing：均为默认夹具未造出的条件控件（分页、批量条、清空按钮）；源码锚点 244/244。
- 4 页无输出：agent-browser/odops 会话限制，非实现缺失。

结论：**P0≠0，不宣称 parity 通过**；归为 A（翻译损耗）/ C（状态未造）。逐页表与复跑方式见 [PARITY.md](PARITY.md) §8。

## 5 决策记录（lead，2026-10-27）

按「契约不推翻产品现状、共享文件不轻动、能量化就修复」逐条定；实现侧已按此收口。

| # | 争议 | 决定 |
|---|---|---|
| 1 | reader 字号/行距：契约 15–26 / 1.3–2.2，源码 14–28 / 1.4–2.4 | 保留源码范围（更宽、已被 308 条用例覆盖）；契约边界待作者收敛 |
| 2 | reader C-13「返回目录」实际跳书籍详情 | 接受现状：详情页即目录入口，字面与行为不矛盾 |
| 3 | library C-09 书架没读 `?tag=` | **已修**：书架首帧读地址标签，标签变化以 replace 回写地址；链接可复制、可直接打开 |
| 4 | library C-07 保存中「可聚焦但不可改」 | 接受置灰不可聚焦：原生 select 的 disabled 与可聚焦互斥，自建 combobox 不划算 |
| 5 | library C-13 卷跳转「3 卷以上」vs 现状「2 卷即出」 | 保持现状：2 卷时跳转省一次滚动，既有用例按此断言 |
| 6 | reader C-29 提示 4s（契约 2s） | 保持全局 4s：全站 toast 一致优先于单点数值 |
| 7 | 共享抽屉 420px / H5 面板 74% | 不动 `prototype.css`：跨域共享样式的回归风险大于视觉收益 |

parity 定位：`odops parity` 的点击区按触屏 44px 硬编码、无视口与阈值参数，「改 PC 32px」不可行。
因此 parity 只作设计评审工具，**不进 CI 门禁**；本轮交付判据是锚点静态对账 244/244 + 定向用例 + 全量门禁。

## 6 UI 走查与修复（2026-10-27）

四域 14 页在 1280×577 / 390×844 用 agent-browser 逐页走查（只读、虚构夹具、每页截图）：

| 批次 | 页面 | 发现 |
|---|---|---|
| discovery + library | 10 | P1=3 / P2=10 |
| ops + reader | 4 | P0=1 / P1=4 / P2=6 |

修复并交叉复验（换人复核）：ops+reader 通过 9 / 误报 1（书签抽屉 Esc）/ 不通过 0；discovery+library 通过 7 / 不通过 0。
- P0 标签管理弹层：限高 545px + 内部滚动 + Esc 关闭 + 焦点回 C-34（原高 1141px、关闭钮在视口外）。
- 书架角标点击不再连带跳详情（C-31）；错档落点补 4 秒提示（C-04/C-05）。
- 焦点归还：C-42 回全选、C-34 回入口、C-70「留下」回返回列表。
- C-53 无匹配文案 + 卷头误导消失；翻章浮条「已翻到第 n 章」；批量条文案、选中行底色。
- 点击区 PC ≥32：角标 / chip / 图例 / 面包屑 / 署名作者 / 评分星 / 勾选 label。
- 评分清零星标回退；C-40 锚点归位；搜索 Esc 复位；C-16 焦点落末行；C-23 清筛选；系列卡用日期；导入错误态不叠空态；不存在书籍给回列表出口。
- 原报 reader 书签抽屉 Esc 为**误报**：class/aria-hidden/几何三判据确认抽屉已收起、焦点已回 C-04。

不修（记录理由）：锚点跨域编号重复（架构既定，改号牵动契约与全部 spec）；热力图格 11×11（图表语义）；PC 下 H5 控件仍在 DOM（需重排外壳）。
走查报告与截图在 `/tmp/ix-walkthrough/`，不入库。

## 7 清理

- `docs/ux/` 已删除（内容并入 `interaction/` 契约与 `03-review.md`）。
- 临时夹具、截图全部在 `/tmp/norvels-parity/`，不入库。
- `impl/{discovery,reader,library,ops}` 分支与 `../norvels-wt-*` worktree 已随合并清理；main 保留 28 个未推送提交。
