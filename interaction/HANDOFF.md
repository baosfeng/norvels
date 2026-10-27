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

全仓前端：**72 文件 / 651 用例通过**；`typecheck` 0 错；`lint` 0 error（8 条既有 react-refresh warning）。
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

## 4 parity 结果（未达 P0=0，如实记录）

对合并后的 main 逐页跑 `odops parity`（视口固定 1280×577）：

- 差异以 **样式翻译差** 为主：字号 14→15、文字色 `--fg-2`→`--fg`、圆角 10→4、PC 按钮 40px（parity 按触屏 44 判）。
- `anchor-missing` 集中在默认夹具状态未造出的控件（分页、状态抽屉、确认框等），不是源码缺锚点。
- 结论：P0 非零，**不写「已按设计实现」**；按 DEVELOPER.md 三分法归类为 A（翻译损耗）/ C（状态未造）。
  后续可逐页造态 + 收口样式，或把 parity 的平台阈值改为 PC 32px。

## 5 已知偏差 / 需产品决定

1. reader 字号 15–26 / 行距 1.3–2.2：源码与既有测试是 14–28 / 1.4–2.4，按源码保留。
2. reader C-13 名称「返回目录」实际跳书籍详情；契约未改名。
3. library C-09 书架 `?tag=` 需要 discovery 侧 `useSearchParams` 初始化（本域已带参数跳转）。
4. library C-07「保存中可聚焦但不可改」：原生 select 两者互斥，实现为置灰不可展开。
5. library C-13 卷跳转前置「3 卷及以上」与产品现状（2 卷即出）冲突，保持现状。
6. reader C-29 提示 4s（契约 2s）：全局 toast 常量未改。
7. 共享抽屉 420px / H5 设置面板 74% 未改 `prototype.css`（避免跨域冲突）。

## 6 清理

- `docs/ux/` 已删除（内容并入 `interaction/` 契约与 `03-review.md`）。
- 临时夹具、截图全部在 `/tmp/norvels-parity/`，不入库。
- 分支 `impl/{discovery,reader,library,ops}` 保留作对账，可删。
