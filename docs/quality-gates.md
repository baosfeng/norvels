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
| 4 | 圈复杂度 | 单函数 **≤ 10**（已登记豁免除外） | McCabe 口径，与 gocyclo 一致（`if`/`for`/`range`/`case`/`&&`/`\|\|` 各 +1，基础 1）；豁免见「豁免登记表」 |
| 5 | 模块大小 | 单函数 **≤ 70 行**、单文件 **≤ 400 行**（已登记豁免除外） | 见下节口径 |
| 6 | 依赖结构 | 无循环依赖 + 方向单向 | 白名单在 `tests/tools/gatedeps/main.go` 的 `allowedDeps` |
| 7 | 覆盖率 | **不低于登记基线**（防退化，硬门禁）；85% 为渐进目标（报告项） | `go tool cover` 行覆盖率；见「覆盖率：为什么用基线」 |
| 8 | 回归测试 | bug 修复必带复现测试 + 反向验证 | 见下节 |
| 9 | 资源护栏 | 写放大断言（落盘 ≤ 事件本体 × 1.6） | 适用于持久化/高频监听；纯查询不适用 |

### 门禁 5 的行数口径

- **单函数 70 行**：从 `func` 关键字所在行到函数体结尾大括号所在行的**物理行跨度**（含签名、注释、空行）；
- **单文件 400 行**：文件物理总行数；
- 只统计 `.go` 文件；`internal/webui/dist`、`node_modules`、`vendor`、`testdata`、隐藏目录不参与；
- 统计工具走 `go/ast`（不是 `wc -l` + 正则），避免 raw string 与多行签名把函数边界算错。

## 豁免登记表

> **豁免不是免检**：`scripts/gate-complexity.sh` 与 `scripts/gate-size.sh` 会读取下表，
> 命中豁免的项在输出里**逐条点名列出**（含实测值与登记值），不会被静默跳过；
> 未登记的超标项一律报错。豁免项**不允许变得更差**：实测超过登记值时同样报错。

### 为什么允许豁免

圈复杂度与函数长度度量的是「控制流分支密度」。norvels 的核心问题是**从格式混乱的中文 txt 里
识别章节与元信息**——这类问题域天生就是「12 种标题标记 × 强弱两级 × 边界预查 × 全角半角」的判定矩阵。
把它强行拆成若干小函数，只会让判定依据散落到多个函数里，**可读性与可维护性反而变差**
（本仓库第一批 200 行的 `parseHeading` 已经踩过这个坑）。

因此口径是：**默认要求 ≤ 10 / ≤ 70 行；确属问题域复杂度的，逐个登记理由与复审条件后豁免。**
豁免是「已知并接受的复杂度」，不是「不检查」。

**登记规则**：

1. 表格是本门禁的**唯一豁免事实来源**，脚本按「类型 + 函数名 + 文件」匹配（不按行号，避免重构后失效；
   方法名两侧的 `()*` 会被归一化，所以 gocyclo 的 `(*cleaner).x` 与 gostats 的 `cleaner.x` 都能匹配）；
2. 每个豁免必须写**具体理由**（为什么拆不开），不接受「太复杂了」「时间紧」这类空话；
3. 每个豁免必须有**复审条件**（什么情况下应该重新尝试拆分）；
4. 新增豁免需在 PR 里说明，并由 reviewer 确认理由成立；
5. **豁免项不允许变差**：实测超过登记值时门禁报错，需重构或重新登记并说明原因。

### 当前豁免统计

| 类型 | 条数 | 说明 |
|---|---|---|
| `cyclo`（圈复杂度 > 10） | 38 | `internal/parser` 36 个、`internal/store` 2 个；全部在 `internal/`，`tests/` 与 `cmd/` 零豁免 |
| `size-func`（函数 > 70 行） | 1 | `TestSampleRealCorpus` 诊断测试 |
| `size-file`（文件 > 400 行） | 0 | 当前无超 400 行的 Go 文件 |

> 门禁工具自身（`tests/tools/gostats`、`tests/tools/gatedeps`）不含任何豁免：
> 它们原本的 `main`/`cyclomatic` 曾超阈值，已重构到 ≤ 10。

### 豁免明细

| 类型 | 函数/文件 | 路径 | 登记实测 | 豁免理由 | 复审条件 |
|---|---|---|---|---|---|
|---|---|---|---|---|---|
| cyclo | `TestSampleRealCorpus` | internal/parser/sample_real_test.go | 50 | 诊断用测试：对真实语料逐条断言书名/作者/卷数/章数/字数，本质是平铺的检查清单，不是业务分支；拆分会把「一本书的完整期望」割裂成多个难以对照的片段 | 真实语料测试改为表驱动断言后再评估拆分 |
| cyclo | `metaFromPreamble` | internal/parser/meta_preamble.go | 30 | 正文头部元信息抽取需识别书名/作者/简介三类标记 × 全角半角冒号 × 带「【】」与否 × 「著」后缀，是组合判定而非流程分支；拆分后每段都要重复做同一套行预处理 | 新增第 4 类元信息标记或标记总数 > 12 时重审 |
| cyclo | `cleaner.cleanContentAt` | internal/parser/cleaner.go | 26 | 正文清洗按「行首/行中/行尾」三种位置分别处理广告、水印、分隔线、重复标点，位置上下文无法抽成独立函数（每个判定都要用同一份位置信息） | 清洗规则改为规则表驱动后重审 |
| cyclo | `trimLine` | internal/parser/normalize.go | 21 | 行首尾装饰字符清理需按 Unicode 类别（全角/半角/符号/中文标点）逐类判定，判定集本身就是字符表，拆成小函数只是把同一张表切成几份 | 装饰字符集改为查表实现后重审 |
| cyclo | `scanCandidates` | internal/parser/split_scan.go | 20 | 章节候选行扫描需覆盖 12 种标题标记 × 强弱两级特征 × 边界预查，是一张判定矩阵；拆分会把「一行到底算不算标题」的判断依据散落到多处 | 标题标记改为声明式规则集后重审 |
| cyclo | `decodeByBOM` | internal/parser/encoding.go | 20 | 按 BOM 前缀分派 5 种编码，并分别做合法性校验与回退，是查表式分派；每个分支都只有 2~4 行，拆分收益小于跳转成本 | 编码探测改为 codec 注册表后重审 |
| cyclo | `validateCandidatesInner` | internal/parser/split_scan.go | 19 | 候选校验需同时满足章号单调递增、正文字数下限、标题长度上限三类约束，且三者互相影响（前一项失败时后一项不再判定） | 校验约束改为可插拔校验链后重审 |
| cyclo | `newCleaner` | internal/parser/cleaner.go | 19 | 清洗器构造按档位装配正则与字符集，是「一个档位一组配置」的平铺装配，拆开会让配置与档位的对应关系变模糊 | 清洗档位配置外置为数据文件后重审 |
| cyclo | `postProcess` | internal/parser/split_build.go | 18 | 切分后处理需合并过短章、丢弃空章、修正卷归属，三步共享同一份章节切片且顺序敏感 | 后处理改为有序步骤列表后重审 |
| cyclo | `runSplitter` | internal/parser/builtin_splitter.go | 18 | 切分器主循环需处理「有章节标记」与「无标记兜底切分」两条完全不同的路径，以及卷章两级聚合 | 兜底切分独立成 splitter 后重审 |
| cyclo | `textQuality` | internal/parser/encoding.go | 16 | 文本质量评分按可打印率、中文占比、替换符比例三项加权，阈值判定交织在同一循环里 | 评分改为可配置权重表后重审 |
| cyclo | `stripTrailingTags` | internal/parser/meta_filename_clean.go | 15 | 文件名尾部标签剥离需处理圆括号/方括号/广告词三类组合，且可连续出现多层 | 标签规则外置后重审 |
| cyclo | `splitTitleAuthor` | internal/parser/meta_filename_clean.go | 15 | 从「书名-作者」串里切分需排除 20+ 个状态词（全本/连载/精校/未删节…），排除表与切分逻辑强耦合 | 状态词表与切分逻辑解耦后重审 |
| cyclo | `TestHelpersCoverage` | internal/parser/heading_parse_test.go | 15 | 辅助函数覆盖率测试：对多个输入逐项断言解析结果，是平铺的检查清单 | 改为表驱动后重审 |
| cyclo | `countWords` | internal/parser/words.go | 14 | 字数统计需区分 CJK/拉丁/数字/标点四类字符并各自计数，是同一循环里的分类累加 | 分词器抽象后重审 |
| cyclo | `TestDecodeBytesBranches` | internal/parser/edge_test.go | 14 | 编码解码分支测试：对多种 BOM/字节序组合逐项断言，是平铺的检查清单 | 改为表驱动后重审 |
| cyclo | `promoteVolumeOnly` | internal/parser/builtin_splitter.go | 14 | 仅有卷无章时把卷提升为章，需处理嵌套空卷与无正文卷两种退化情况 | 卷结构规范化后重审 |
| cyclo | `isCJK` | internal/parser/words.go | 13 | Unicode 区间判定，本质是多区间查表；拆成「是否基本区/是否扩展区」只会增加调用层级 | Unicode 区间表外置后重审 |
| cyclo | `titleFromFilename` | internal/parser/meta_filename.go | 13 | 文件名推书名按 5 级优先级依次尝试（书名号/整体括号/作者标记/去装饰/兜底），优先级顺序本身就是逻辑，拆开看不出先后 | 优先级改为有序解析器链后重审 |
| cyclo | `parseHeading` | internal/parser/heading_parse.go | 13 | 标题行解析入口按「前缀式/内联式/纯数字」三类分派，分派前的归一化必须共享 | 分派策略独立后重审 |
| cyclo | `parsePrefixedHeading` | internal/parser/heading_inline.go | 13 | 「第一章 xxx」式标题解析需同时处理全角数字、中文数字、阿拉伯数字、罗马数字四种数字形态 | 数字形态统一解析后重审 |
| cyclo | `cleaner.isNoiseLine` | internal/parser/cleaner.go | 13 | 噪声行判定需覆盖 URL/水印/分隔线/重复标点/纯符号多类，是一张排除清单 | 噪声规则改为正则集合后重审 |
| cyclo | `splitChapterContent` | internal/parser/postprocess.go | 12 | 章节正文切分需处理标题与正文粘连、正文首行为标题重复两种情况 | 粘连判定独立后重审 |
| cyclo | `TestParsePipeline` | internal/parser/pipeline_test.go | 12 | 解析流水线端到端测试：对多个样例断言各阶段产物，是平铺的检查清单 | 改为表驱动后重审 |
| cyclo | `parseVolumeHeading` | internal/parser/heading_parse.go | 12 | 卷标题解析需处理「第N卷」「上部」「Part I」「卷N」多种形式及无标题卷 | 卷标记规则集外置后重审 |
| cyclo | `isMarkerBoundary` | internal/parser/heading.go | 12 | 标记边界判定需处理标点/空白/括号三类边界，且全角半角都要覆盖 | 边界字符集外置后重审 |
| cyclo | `decodeUTF16` | internal/parser/encoding.go | 12 | UTF-16 解码需处理 BOM 有无、字节序、代理对、奇数长度截断四种异常 | 统一走 x/text 后重审 |
| cyclo | `TestRegistryBuiltins` | internal/parser/registry_test.go | 11 | 注册表内置项测试：逐个断言内置 codec/filter/splitter 已注册，平铺清单 | 改为表驱动后重审 |
| cyclo | `TestRefineSplitKeepsNonOutliers` | internal/parser/quality_test.go | 11 | 切分修正测试：构造多组离群/非离群章节断言修正结果，平铺清单 | 改为表驱动后重审 |
| cyclo | `isPlausibleAuthor` | internal/parser/meta_preamble.go | 11 | 作者名合法性判定需排除状态词、长度越界、字符集不符三类非法情形 | 状态词表外置后重审 |
| cyclo | `stripTitleTailTags` | internal/parser/meta_filename_tags.go | 11 | 书名尾部标签剥离需处理「(上)」「【完结】」「- N章」三类尾缀组合 | 尾缀规则外置后重审 |
| cyclo | `stripFileDecor` | internal/parser/meta_filename_clean.go | 11 | 文件名装饰剥离按序号/广告词/括号三类迭代清理，最多 6 轮，轮次内三类互相影响 | 装饰规则声明式化后重审 |
| cyclo | `parseCNDNumeral` | internal/parser/heading_num.go | 11 | 中文数字解析需处理「两」「廿」「〇」变体与个/十/百/千位进位 | 中文数字库替换后重审 |
| cyclo | `headingReverseMeta.Extract` | internal/parser/builtin_meta.go | 11 | 反向元信息抽取（文末书名/作者）需处理尾部多种落款格式 | 抽取规则外置后重审 |
| cyclo | `utf16BECodec.Try` | internal/parser/builtin_codec.go | 11 | UTF-16BE 探测需按高字节分布、零字节比例、代理对合法性综合判定 | 探测改为统一启发式后重审 |
| cyclo | `utf16LECodec.Try` | internal/parser/builtin_codec.go | 11 | UTF-16LE 探测需按低字节分布、零字节比例、代理对合法性综合判定 | 探测改为统一启发式后重审 |
| cyclo | `saveBookInTx` | internal/store/save.go | 11 | 单事务写入书/卷/章/标签四类记录，需按「新建 or 覆盖」分派各自的插入与更新语句 | 写入改为批量 UPSERT 后重审 |
| cyclo | `Store.readMetrics` | internal/store/metrics.go | 11 | 指标读取需处理 NULL 列、空表、时间列零值三种缺失形态 | 指标读取改用 sql.NullX 统一封装后重审 |
| size-func | `TestSampleRealCorpus` | internal/parser/sample_real_test.go | 203 | 诊断用测试：对每份真实语料逐字段断言（书名/作者/卷/章/字数/告警），行数来自断言条数而非逻辑长度；拆成多个测试函数会丢失「同一本书的完整期望」这一对照关系 | 真实语料测试改为表驱动断言后再评估拆分 |

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

## 覆盖率：为什么用「基线 + 渐进目标」而不是一步 85%

### 问题

`quality-gates` 的门禁 7 标准是行覆盖 ≥ 85%。但 85% 是**目标**，不是当前基线。
第一版发布前，代码实测总覆盖 77.9%，离 85% 差 7.1 个百分点。
如果把它设成硬门禁，第一版就发不出去——而**为了变绿去调低阈值或写空测试，比覆盖率低得多更糟**。

### 做法：两级判定

| 层级 | 判定 | 说明 |
|---|---|---|
| **硬门禁**（防退化） | 总覆盖率与核心包覆盖率**不得低于登记基线**，低于即 exit 1 | 基线写在 `scripts/gate-cover.sh` 顶部常量里 |
| **报告项**（渐进目标） | 打印距 85% 的差距，**不判红** | 让差距持续可见，作为后续排期依据 |

### 防退化机制（核心）

**基线只升不降**。这是这套机制唯一的约束，也是它成立的前提：

1. 基线值写死在 `scripts/gate-cover.sh` 顶部（`BASELINE_TOTAL` / `BASELINE_STORE` / …）；
2. 任何改动导致覆盖下降 → 低于基线 → 门禁红；所以「不低于基线」等价于「覆盖只增不减」；
3. 覆盖提升后，**把基线调到新实测值**（上调无需审批）；
4. **下调基线必须有 leader 明确同意**，并在脚本注释里写原因；
5. 基线值随提交一起评审，diff 里一眼能看出是升是降。

### 当前基线（2026-10-25 登记）

| 范围 | 基线 | 实测 | 硬门禁 | 距 85% 目标 |
|---|---|---|---|---|
| 总行覆盖率 | 77.9% | 77.9% | ✅ | 还差 7.1 个百分点 |
| internal/parser | 87.8% | 87.8% | ✅ | 已达标 |
| internal/importer | 85.1% | 85.1% | ✅ | 已达标 |
| internal/store | 80.2% | 80.2% | ✅ | 还差 4.8 个百分点 |
| internal/api | 80.1% | 80.1% | ✅ | 还差 4.9 个百分点 |

> **基线会随实测上调**：parser 基线首次登记为 86.2%（leader 口径 86.5 测于更早时点，parser 尚在重构），
> 重构收敛后重测为 87.8%，已按「只升不降」上调。总覆盖同理：77.2% → 77.9%。
> 这就是防退化机制在起作用：**基线只会朝着 85% 的目标单向移动**。

### 怎么验证防退化真的有效

```bash
# 删掉一个测试文件 → 覆盖下降 → 门禁必须变红（实测过）
mv internal/api/static_test.go /tmp/
scripts/gate-cover.sh       # → 总覆盖 76.4%（< 77.9%）、api 71.6%（< 80.1%）→ exit 1
mv /tmp/static_test.go internal/api/
```

### 后续怎么收敛到 85%

1. 每个迭代补测试后跑 `scripts/gate-cover.sh`，实测涨了就上调基线；
2. 差距持续收敛到 0 时，把 `TARGET` 与基线合并（届时基线 = 85%，两级判定自然合一）；
3. 不允许的路径：下调基线、写空断言凑覆盖、把包排除出统计。

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