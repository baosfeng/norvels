# 质量门禁（quality-gates）

> **编号事实源是 `scripts/gate-all.sh`**：实际执行 **9 项**，编号 1/2/4/5/6/7/9/10/11 —— 3 与 8 是历史空号，不对应任何门禁。
> 门禁只认命令与输出，自述「已达标」不算数。

## 跑全部

```bash
make gate         # 逐项打印结果，任一项未过 → exit 1
make gate --stop  # 遇失败立即停
make verify       # 等同 make gate（提交前入口）
```

单项复跑：`make gate-acceptance`（2）、`gate-complexity`（4）、`gate-size`（5）、`gate-deps`（6）、`gate-cover`（7）、`privacy`（9）。
1/10/11 没有单项 make 目标：10 = `scripts/check-build-parity.sh`，11 = `scripts/gate-frontend.sh`，1 只在 `gate-all.sh` 内部（跑整体 `make gate` 即可）。

## 9 项与阈值

| # | 门禁 | 阈值 | 度量方式 |
|---|---|---|---|
| 1 | 单元测试 | 全绿 | `go test ./... -count=1`（不用缓存）+ gofmt + go vet |
| 2 | Gherkin 验收 | 场景全绿 | godog（`tests/features/*` + `tests/acceptance`，Strict） |
| 4 | 圈复杂度 | 单函数 ≤ 10 | gocyclo（golangci-lint；缺失回退 gostats） |
| 5 | 模块大小 | 函数 ≤ 70 行、文件 ≤ 400 行 | gostats（go/ast 精确统计） |
| 6 | 依赖结构 | 无循环 + 方向单向 | gatedeps（`go list` + DFS 找环） |
| 7 | 覆盖率 | 不低于登记基线（硬门禁），85% 为渐进目标 | `go tool cover` 行覆盖率 |
| 9 | 隐私扫描 | 零命中 | `scripts/privacy-scan.sh --all` |
| 10 | 构建一致性 | 容器工具版本满足 `web/` 要求 | `scripts/check-build-parity.sh` |
| 11 | 前端单元测试 | 全绿 | vitest（`cd web && pnpm test`） |

## 没有门禁 3 与 8

- **3**：历史文档写的「QA 流程」不是门禁，是 `make gate` 的固定顺序（单元 → 验收 → 全量套件），由脚本本身保证，不占号。
- **8**：历史文档写的「防复发」也不占号 —— 它与「资源护栏」是**评审规则，未纳入 gate-all**（一个要人工反向验证，
  一个要按写入路径定制断言，都跑不成固定命令）。这两条的要求就在这里：
  - **防复发**：补一个能复现该 bug 的测试（修复后必绿）；临时回滚修复时它必须红、且失败原因正是该 bug。没有反向验证记录的修复视为未完成。
  - **资源护栏**：持久化写入 / 高频事件 / 轮询 / 日志落盘适用；N 次事件落盘字节 ≤ 事件本体 × 1.6（含容差）。
    一次 CLI、纯计算、纯查询在 PR 里写「不适用 + 理由」。本库的 `SaveBook` 与导入任务状态更新适用。

## 门禁 5 的行数口径

- **函数 70 行** = `func` 行到函数体结尾大括号行的物理跨度（含签名、注释、空行）；**文件 400 行** = 物理总行数；
- 只统计 `.go`（跳过 `internal/webui/dist`、`node_modules`、`vendor`、`testdata`、隐藏目录）；
  走 `go/ast` 而不是 `wc -l`，避免 raw string 与多行签名把函数边界算错。

## 豁免登记表（门禁 4 / 5 的唯一事实源）

> 豁免不是免检：`scripts/gate-complexity.sh` 与 `scripts/gate-size.sh` 读下表，命中项在输出里**逐条点名**（含实测值与登记值）；
> 未登记的超标项一律报错，已登记项**不允许变差**（实测超过登记值同样报错）。每条豁免写**具体理由**与**复审条件**，新增需 PR 说明 + reviewer 确认。
> 当前 cyclo 38 条 / size-func 1 条 / size-file 0 条；门禁工具自身（gostats、gatedeps）零豁免。
> 口径：默认 ≤ 10 / ≤ 70 行；确属问题域复杂度的（中文 txt 标题与元信息的判定矩阵）逐条登记后豁免 —— 强行拆分会把判定依据散落多处，可读性更差。

| 类型 | 函数/文件 | 路径 | 登记实测 | 豁免理由 | 复审条件 |
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

## 依赖方向

`tests/tools/gatedeps/main.go` 的 `allowedDeps` 是事实源：cmd → internal/{api,importer,parser,store,source}；api → importer/parser/store/source/webui；
importer → parser/store/source/model；parser,store → model；model、webui 为叶子。新增内部包必须先登记，否则 `make gate-deps` 以「未登记」判失败。
`internal/source`（书源规则包）是叶子包：store、api、importer 可依赖它，它不依赖任何内部包（纯数据模型 + 校验）。

## 覆盖率：基线 + 渐进目标

85% 是目标不是基线（首版 77.9%、当前 83.2%）：设成硬门禁首版就发不出去，而调低阈值或写空测试比覆盖低得多更糟。
两级判定：**硬门禁** = 总覆盖率与核心包不得低于登记基线（值写在 `scripts/gate-cover.sh` 顶部，低于即 exit 1）；
**报告项** = 打印距 85% 的差距，不判红。**基线只升不降**：涨了上调到新实测（无需审批）；下调必须 leader 明确同意并在脚本注释写原因。

当前基线（2026-10-31 实测上调，与 `scripts/gate-cover.sh` 顶部常量一一对应）：总行覆盖率 83.2% / parser 88.4% / importer 90.4% / store 82.3% / api 86.3%。
验证防退化：临时移走一个测试文件 → `scripts/gate-cover.sh` 必须 exit 1（实测 总覆盖 76.4%、api 71.6%）。

## 环境要求

- Go 1.26（以 `go.mod` 的 `go 1.26.0` 为准；`go test` / `go list` / `go tool cover`）；godog 已在 `go.mod` 里，`go test` 直接跑。
- golangci-lint v2.x（门禁 4 首选；**缺失自动回退** gostats，不跳过门禁）。`gate-complexity.sh` 测量前自带 `cache clean`：
  缓存按绝对路径存取，会拿已删 worktree 的旧文件报「幻影超标」。脚本兼容 macOS 自带 bash 3.2（不用 `mapfile`）。
- 验收测试 fixture 一律**合成**（书名/作者/正文都是造出来的串），不读真实语料；`scripts/privacy-scan.sh --all` 零命中是提交前硬条件（见 [privacy.md](privacy.md)）。
- IP 类只放行 RFC 1918/3927/5737/4193 的私有、链路本地与文档保留段；公网地址一律拦截。
