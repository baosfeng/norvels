# interaction/ — 交互设计契约与原型

> 交互设计的唯一事实源。一个控件一个 `C-xx` 锚点，文档 / 原型 / 实现共用同一个键，可机器对账。
> 规范：`designing-interactions` 技能（`SKILL.md` + `references/interaction-contract.md`）。
> 面向读者：**开发 agent 照契约实现，不得改设计**；改动只能进 `04-ledger.md` 并重新评审。

## 产物（每域五件）

| 文件 | 是什么 | 谁写 |
|---|---|---|
| `00-features.md`（本目录根） | 功能盘点：系统有什么、入口在哪、证据 | 作者 |
| `<域>/01-contract.md` | 交互契约：一行 = 一个控件的一种触发 | 作者 |
| `<域>/02-prototype.html` | 可点原型：控件带 `data-od-id="C-nn"` | 作者 |
| `<域>/03-review.md` | 评审结论：分级 + 覆盖度 + 证据命令 | 评审 |
| `<域>/04-ledger.md` | 返工台账：每条意见 → 改了什么 → 状态 | 作者 + 评审 |

## 域划分

| 域 | 范围 | 契约 |
|---|---|---|
| [reader](reader/) | `/read/:bookId/:chapterId` 全部阅读交互 | 阅读是产品核心，能力两端等价 |
| [discovery](discovery/) | 外壳 + `/home` + `/shelf` + `/search` + `/series` + `/history` | 最常用路径 |
| [library](library/) | `/book/:bookId` + `/authors` + `/dashboard` | 决策页 |
| [ops](ops/) | `/import` + `/admin` + `/admin/books/:bookId` | PC 专属，窄屏不出现 |

## 门禁（改完必跑，退出码 0 才算过）

```bash
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/<域> --platform web
odops verify interaction/<域>/02-prototype.html --platform web --browser
```

- `ixcheck` **P0 必须清零**；P1 要么改掉，要么在 `04-ledger.md` 写「保留（理由）」；P2 可留。
- 锚点契约：`C-01`…`C-nn` 顺序编号，**一经发布不改号**；同一控件的多种触发共用同一锚点（契约里多行）。
- 演示件（切状态 / 切断点 / 切页）统一 `data-od-id="demo-only:*"`，不计入契约，**产品不得带入**。

## 「符不符合直觉」的四条判据

| 判据 | 合格线 |
|---|---|
| 可发现 | 入口不依赖悬停才可见；触屏不得以悬停为唯一入口 |
| 可撤销 | 不可逆操作二次确认并写明后果；能撤销的给撤销而非确认框 |
| 可达 | 点击区 ≥44×44 CSS px，键盘可达且 `focus-visible` 可见 |
| 可解释 | 禁用必须有原因文案；空 / 加载 / 错误 / 成功四态齐备，错误保留用户输入 |

判冗余用同一把尺子：**这个控件删掉，用户哪一步会卡住？** 答不出来 = 冗余，删。

## 分档规则（与实现一致）

- H5 ≤768px、PC ≥1024px（768–1024 归 PC），判定入口 `web/src/lib/tier.ts:20`。
- 窄屏下管理类入口（导入 / 作者 / 驾驶舱 / 书籍管理）**不出现**，不是隐藏。
- 阅读器两端能力等价，只换入口形态（目录全屏抽屉、设置底部面板）。
- 契约「边界」列写清两档差异；只有一档存在的控件，在 `控件` 名后缀标 `（仅 PC）` 或 `（仅 H5）`。

## 与视觉规范的关系

- 视觉与风格走 `open-design`（warm-editorial，token 见 `.open-design/02-style.md`）。
- 本目录只管**行为与状态**：控件怎么响应、点了发生什么、关掉之后去哪。
- 原型必须单文件自包含、只用 token、无外部 CDN。

## 文档长度豁免

本目录的 `01-contract.md` 是**机器读取的对账表**，不受仓库「单文档 ≤120 行」约束
（同 `docs/quality-gates.md` 的豁免登记口径）。其余文档仍按 120 行控制。

## 旧版交互说明

`docs/ux/` 是上一版按域写的交互说明（含评审报告），内容已并入本目录的契约；
保留作历史对照，**不再作为开发依据**——以 `interaction/` 为准。
