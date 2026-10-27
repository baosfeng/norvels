# 交互契约实施计划（2026-10-27 夜间）

> 设计已冻结在 `interaction/`，本轮**只实现、不改设计**。写者边界、验收标准、取证命令以此为准。
> 设计缺口写进各域 `impl-notes.md` 并回报，不自行改契约。

## 1 目标（冻结，不再加需求）

| # | 交付 | 判据 |
|---|---|---|
| 1 | 244 个契约锚点落到产品 | `data-od-id="C-nn"` 与契约逐字相同，脚本 0 缺失 |
| 2 | 25 个【新增】控件全部实现 | 控件、文案、边界与契约一致；有 vitest 用例 |
| 3 | 四种触发逐条落实 | click / hover / focus / 键盘；触屏补长按与滑动 |
| 4 | 「移除/关闭后」五问 | 撤销窗口 / 焦点去向 / 滚动位置 / 计数联动 / 列表归属，逐域打勾 |
| 5 | 边界态 | 禁用有常驻原因；空 / 加载 / 错误 / 成功四态齐备 |
| 6 | 质量 | `pnpm test` / `typecheck` / `lint` / `make gate` 全绿 |
| 7 | 演示件隔离 | `web/src` 与发布构建里搜不到 `demo-only` |

## 2 写域（一域一 worktree，互不越界）

> 下表是夜间分工记录；worktree 与 `impl/*` 分支已在合并后清理，仅留作追溯。

| 域 | worktree | 分支 | 写域 |
|---|---|---|---|
| 发现 | `../norvels-wt-discovery` | `impl/discovery` | `web/src/layouts/**`、`features/{start,shelf,search,series,history}/**`、`styles/ix-discovery.css` |
| 阅读 | `../norvels-wt-reader` | `impl/reader` | `features/reader/**`、`lib/{useReaderControls,autoScroll,tocRows,bookmark,readerSearch,readerSettings}.ts`、`styles/ix-reader.css` |
| 书目 | `../norvels-wt-library` | `impl/library` | `features/{book,authors,dashboard}/**`、`lib/charts.ts`、`styles/ix-library.css`、`routes.tsx`（仅新增作者作品路由） |
| 运营 | `../norvels-wt-ops` | `impl/ops` | `features/{import,admin}/**`、`styles/ix-ops.css` |

`web/src/styles/{ix-*.css}` 已建好并 import，**只由本域改**。其余共享文件（`globals.css` / `tokens.css` / `prototype.css` / `routes.tsx` / `api/**`）默认不动；确需改动先回报，由 lead 统一改。

## 3 共享约定

- 锚点用字面量：`<button data-od-id="C-12">`。**不用变量拼**，否则对账脚本抓不到。
- 同一控件多触发共用一个锚点；一个锚点只挂一个节点（同锚点多文件会被报重复）。
- 新增控件优先复用既有类：`.btn` / `.icon-btn` / `.chip` / `.input` / `.seg` / `.table`；禁用原因用 `.inline-note`。
- 文案不出现实现词（SQLite / API / localStorage / 后端 / 端点）。
- 焦点：沿用 `lib/focusTrap.ts`（打开移入、Esc 关闭、关闭还焦点）；关闭后焦点必须回到触发元素。
- 测试：新增文件名 `*.test.tsx`，就近放置；锚点存在性用 `node scripts/check-anchors.mjs`，**不要**把它塞进 vitest。
- 只跑定向测试：`pnpm -C web exec vitest run <path>`；**不跑** `make gate` / `pnpm build`（全量门禁是独占资源，由 lead 收口）。
- 提交：`git add -A && git commit -m "feat(<域>): 按交互契约实现 C-xx…"`，工作区保持干净。

## 4 验收（lead 收口，逐条取证）

```bash
node scripts/check-anchors.mjs                 # 0 缺失
cd web && pnpm test && pnpm typecheck && pnpm lint
cd .. && make gate                             # 九项
grep -rn 'demo-only' web/src | wc -l           # 0
git status --porcelain                         # 空
```

parity：四域原型 → `odops spec`，对临时夹具库逐页 `odops parity`；比不了的页面写明原因与已覆盖锚点数，不写「已按设计实现」。

## 5 禁止

- 改 `interaction/<域>/01-contract.md`、`02-prototype.html`、`03-review.md`、`04-ledger.md`（设计冻结；只写 `impl-notes.md`）。
- 碰 `data/norvels.db`、跑迁移、改生产数据。
- 真实书名 / 作者 / 截图入仓（夹具用虚构名，放 `/tmp`）。
- 用 `--no-verify`、改阈值、跳过用例。
- 顺手重构与契约无关的代码。

## 6 回报格式

```
交付：C-01…C-nn 已落地，新增控件 n 个
证据：命令 + 期望输出（exit code / 计数）
未覆盖：锚点 / 原因 / 建议
待产品决定：…
```
