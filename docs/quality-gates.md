# 质量门禁（quality-gates）

> 本文件是 norvels 的门禁口径说明 + 当前实测基线。**门禁只认命令与输出**，
> 自述「已达标」不算数；每项都可用 `make gate-<项>` 单独复跑。

## 一条命令跑全部

```bash
make gate          # 逐项跑完并打印结果；任何一项未通过 → exit 1
make gate --stop   # 等价 scripts/gate-all.sh --stop：遇失败立即停
make verify        # 与 make gate 等价（提交前入口）
```

单项复跑：

| 目标 | 门禁 | 度量方式 |
|---|---|---|
| `make gate-acceptance` | 2 Gherkin 验收 | godog（`tests/features/*.feature` + `tests/acceptance`） |
| `make gate-complexity` | 4 圈复杂度 | golangci-lint 的 gocyclo；工具缺失时回退 `tests/tools/gostats` |
| `make gate-size` | 5 模块大小 | `tests/tools/gostats`（go/ast 精确统计） |
| `make gate-deps` | 6 依赖结构 | `tests/tools/gatedeps`（`go list -json` + DFS 找环） |
| `make gate-cover` | 7 覆盖率 | `go test -coverprofile` + `go tool cover` |
| `make privacy` | 9 隐私扫描 | `scripts/privacy-scan.sh --all` |

## 阈值口径

| # | 门禁 | 阈值 | 说明 |
|---|---|---|---|
| 1 | 单元测试 | 全部通过 | `go test ./... -count=1`（不用缓存结果） |
| 2 | Gherkin 验收 | 场景全绿 | 每个用户主流程一个 `.feature`；godog `Strict: true` |
| 3 | QA 流程 | 单元 → 验收 → 回归 | 由 `make gate` 的固定顺序保证；回归=全量套件 |
| 4 | 圈复杂度 | 单函数 **≤ 10** | McCabe 口径，与 gocyclo 一致（`if`/`for`/`range`/`case`/`&&`/`\|\|` 各 +1，基础 1） |
| 5 | 模块大小 | 单函数 **≤ 70 行**、单文件 **≤ 400 行** | 见下节口径 |
| 6 | 依赖结构 | 无循环依赖 + 方向单向 | 白名单在 `tests/tools/gatedeps/main.go` 的 `allowedDeps` |
| 7 | 覆盖率 | 总行 **≥ 85%**，核心包（store/parser/importer/api）各自 **≥ 85%** | `go tool cover` 行覆盖率 |
| 8 | 回归测试 | bug 修复必带复现测试 + 反向验证 | 见下节 |
| 9 | 资源护栏 | 写放大断言（落盘 ≤ 事件本体 × 1.6） | 适用于持久化/高频监听；纯查询不适用 |

### 门禁 5 的行数口径

- **单函数 70 行**：从 `func` 关键字所在行到函数体结尾大括号所在行的**物理行跨度**（含签名、注释、空行）；
- **单文件 400 行**：文件物理总行数；
- 只统计 `.go` 文件；`internal/webui/dist`、`node_modules`、`vendor`、`testdata`、隐藏目录不参与；
- 统计工具走 `go/ast`（不是 `wc -l` + 正则），避免 raw string 与多行签名把函数边界算错。

### 白名单豁免（逐个说明原因）

当前 **没有** 大小/复杂度的白名单豁免项。两条容易被误会的：

- `internal/store/schema.sql`（140 行）是 `go:embed` 的**生成物**（SQL schema），不是 Go 源码，不参与门禁 5（门禁 5 只统计 `.go`）。它的正确性由 store 包的迁移测试覆盖；
- `tests/tools/*` 是门禁自身的测量工具，参与门禁但只有标准库依赖（`gatedeps` 会拒绝它们反向依赖 `internal/*`）。

需要新增豁免时：在本文件登记「路径 + 原因 + 补偿措施」，并同步 `gatedeps` 的 `allowedDeps` 或脚本内的白名单常量。**不允许不登记就静默放过**。

## 依赖方向约定

```
cmd/norvels      → internal/{api,importer,parser,store}
cmd/healthcheck  → （仅标准库）
internal/api     → internal/{importer,parser,store,webui}
internal/importer→ internal/{parser,store,model}
internal/parser  → internal/model
internal/store   → internal/model
internal/model   → 叶子（无内部依赖）
internal/webui   → 叶子（仅 embed）
```

任何新增内部包必须先在 `tests/tools/gatedeps/main.go` 的 `allowedDeps` 登记，
否则 `make gate-deps` 会以「未登记」判失败（防止新包悄悄绕过分层）。

## 覆盖率现状（如实记录）

门禁 7 的阈值是 **85%**，当前代码**未达标**，因此 `make gate` 会在这一项 exit 1。
这是预期行为：脚本打印真实数字，不调阈值、不 skip、不用 `|| true` 掩盖。

测量命令与基线数字（每次改动后重跑 `make gate-cover` 更新）：

```bash
make gate-cover      # 或 scripts/gate-cover.sh --detail
```

| 范围 | 基线（commit aba2751 时的测量） | 阈值 | 状态 |
|---|---|---|---|
| 总行覆盖率 | 40.5% | ≥ 85% | ❌ |
| internal/store | 78.3% | ≥ 85% | ❌ |
| internal/importer | 83.7% | ≥ 85% | ❌ |
| internal/api | 76.6% | ≥ 85% | ❌ |
| internal/parser | 1.1% | ≥ 85% | ❌ |

拉高覆盖率的主要缺口在 `internal/parser`（大量分支未覆盖）。补齐属于 parser 重构任务的范畴，
不在门禁基建任务内；本任务只负责让差距**可见且不可绕过**。

## 门禁 8：防复发（回归测试）

修复 bug 时必须：

1. 先修（或先写失败测试，RED→GREEN 为等价路径）；
2. 补一个能复现该 bug 的测试，在修复后代码上运行**为绿**；
3. **反向验证**：临时回滚修复（`git stash` / revert 该 commit），该测试**必须为红**，且失败原因正是该 bug；
4. 恢复修复，测试永久保留。

没有反向验证记录的修复视为门禁 8 未通过。

## 门禁 9：资源护栏

适用范围：持久化写入、高频事件监听、轮询、日志落盘。
判定：写放大复现测试，高频 N 次事件落盘字节 ≤ 事件本体字节 × 1.6（含容差）。

不适用时在 PR 描述里写「不适用 + 理由」（一次性 CLI、纯计算、纯查询）。

norvels 的 `store` 写入路径（`SaveBook`）与 `api` 的导入任务状态更新属于适用范围。

## 环境要求

- Go 1.27+（`go test`、`go list`、`go tool cover`）
- golangci-lint v2.x（门禁 4 首选；**缺失时自动回退**到 `tests/tools/gostats`，不会跳过门禁）
- godog 已在 `go.mod` 声明（`github.com/cucumber/godog`），`go test` 直接跑，无需额外安装
- 脚本兼容 macOS 自带 bash 3.2（不用 `mapfile` / 关联数组）

## 隐私约定（与门禁 9 交叉）

验收测试的 fixture 一律**合成**：`tests/acceptance` 里的书名、作者、正文都是造出来的字符串，
不读取任何真实语料。`scripts/privacy-scan.sh --all` 零命中是提交前硬条件（见 `docs/privacy.md`）。
