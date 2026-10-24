# 交接：质量门禁基建（task-3 / gates-agent）

> 续接先读这份，不要重复验证已确认事实。基线 commit：`aba2751`（工作区未提交，由 leader 统一提交）。

## 任务契约

补齐 quality-gates 9 项门禁里缺失的自动化检查。写区：`scripts/gate-*.sh`、`tests/`、`Makefile`、
`.golangci.yml`、`docs/quality-gates.md`（本文件在 `tests/` 内）。**internal/ 一律没动**。

## 交付物

| 文件 | 作用 |
|---|---|
| `scripts/gate-all.sh` | 全量门禁总入口（`make gate` 调它），逐项跑完再汇总，如实返回退出码 |
| `scripts/gate-complexity.sh` | 门禁 4 圈复杂度 ≤ 10；golangci-lint/gocyclo 优先，缺失回退 gostats |
| `scripts/gate-size.sh` | 门禁 5 单函数 ≤ 70 行 / 单文件 ≤ 400 行 |
| `scripts/gate-deps.sh` | 门禁 6 依赖方向 + 循环依赖 |
| `scripts/gate-cover.sh` | 门禁 7 覆盖率（总 ≥ 85%，核心包各自 ≥ 85%） |
| `.golangci.yml` | golangci-lint v2 配置：gocyclo(10) / funlen(70 行, 50 语句) / gocognit(20) |
| `tests/tools/gostats/` | go/ast 统计函数行数与圈复杂度（+ 单测） |
| `tests/tools/gatedeps/` | go list -json 依赖图 + DFS 找环 + 方向白名单（+ 单测） |
| `tests/features/*.feature` | 3 个 zh-CN Gherkin：reading / search / import_failure（共 8 场景） |
| `tests/acceptance/` | godog 步骤定义 + 真实 store/api/httptest 链路，fixture 全合成 |
| `docs/quality-gates.md` | 门禁口径、阈值、依赖约定、覆盖率基线、豁免登记规则 |
| `Makefile` | 新增 `gate` / `gate-*` 目标；`verify` 改为等同 `make gate` |

## 已确认事实

- `go.mod` 新增直接依赖 `github.com/cucumber/godog v0.16.0`（`go mod tidy` 后由 indirect 提为直接）。
- `go test ./tests/... -count=1` 全绿：8 scenarios / 30 steps 全通过。
- `make gate` 汇总：1 ✅ / 2 ✅ / 4 ❌ / 5 ❌ / 6 ✅ / 7 ❌ / 9 ✅，整体 exit 1（覆盖率不达标属预期）。
- 门禁 4：最高复杂度 `Run` = 51（`internal/importer/importer.go:90`），超阈值 50 个函数。
- 门禁 5：最大文件 `internal/parser/chapter.go` 1165 行；最长函数 `TestSampleRealCorpus` 203 行；超长函数 13 个、超大文件 5 个。
- 门禁 6：无循环依赖，方向全部合规。
- 门禁 7：总 40.5%；store 78.3% / importer 83.7% / api 76.6% / parser 1.1%。
- `scripts/privacy-scan.sh --all` 零命中；`gofmt -l ./cmd ./internal` 为空。

## 设计决定（续接时别推翻）

- **统计走 go/ast 不走 awk/wc**：raw string 与多行签名会把函数边界算错。
- **门禁不早停**：`gate-all.sh` 刻意跑完全部项再汇总，一次能看到所有差距；`--stop` 才遇错即停。
- **BSD sed 的 BRE 不支持 `\+`**：脚本里的文本解析统一用 awk（兼容 macOS bash 3.2）。
- **godog 会先剥掉「假定/当/那么/并且」关键字**：步骤正则只匹配剩余正文，别加关键字前缀。
- **验收 fixture 的文件名即书名**：parser 优先用文件名做书名，所以合成书稿文件名要用合成书名。
- **没有大小/复杂度白名单豁免**：`schema.sql` 是生成物但非 Go 文件，本来就不入口径。

## 遗留风险

- `internal/parser` 覆盖率 1.1%，是门禁 7 的最大缺口；补覆盖属于 parser 重构任务，不在本任务范围。
- 门禁 4/5 的超标函数全部在 `internal/`，本任务无法修（写区不含 internal），只能如实报告。
- 取证时工作区可编译；之后 parser 重构进入编辑中间态（`registry.go` 等临时缺符号），
  `make gate` 需等 parser 重构收敛后可复跑。
