# 元数据补全与超长章节切分：enrich（2026-11-08）

> 触发：生产 18.2 万本库里 9.7 万本缺简介、8816 个超长章节、分类全部落在「未分类」，
> 作者名里「佚名 / Unknown / 未知 / 不详」并存（字数 Top10 里占位名合计 281.3 亿字）。
> 目标：一条命令按在线书源补元数据，并把占位作者、未分类、超长章节一起收敛。

## 1. 生产实测（7.1 GB 主库副本，dry-run）

| 项 | 值 | 说明 |
|---|---|---|
| 总书数 | 182898 | — |
| 缺简介 | 96949 → 候选 107743 | 与占位作者/未分类有重叠 |
| 缺作者（author_id=0） | 0 | **假象**：导入把空作者落成「佚名」，按 author_id 判永远是 0 |
| 占位作者 | 37510 | 佚名 / 未知 / 不详 / 多人… |
| 未分类 | 100606 | 只按导入目录推断，txt 目录本身没有分类信息 |
| 三类去重候选 | 175295 | 一次运行的规模上限 |
| 超长章节（≥3 万字） | 8816 | 切章失败的兜底痕迹 |
| 切分样例 | 57.1 万字 → 68 段 | `--split-long-chapters --limit 20`，18/20 章可切 |
| 正文库悬空引用 | 15002 | 只报不修（缺正文只能重导），**不在本设计范围** |

## 2. 结论

| 做什么 | 为什么 |
|---|---|
| `norvels enrich` 子命令 + `scripts/enrich.sh` | 运维一条命令；本地与容器同一入口 |
| 复用 `internal/fetch` | SSRF/端口/重定向/QPS/并发四道边界已过 ADR-0004 §3，没有理由另写 HTTP 客户端 |
| 复用 `internal/ruleexec` + `source.Online` | 书源是 Legado JSON，规则求值器已存在；@js 字段降级为空并在报告里点名 |
| 多源投票而非单源取值 | 单源错名错作者常见，投票 + 相似度阈值才挡得住 |
| 默认 dry-run + 变更日志 + `--rollback` | 17.5 万本量级的写必须可逆 |
| 超长章节走 `store.SplitChapter` | 已有实现是「只改 block_off/block_len」的偏移改写，不重压正文 |

| 不做什么 | 为什么 |
|---|---|
| 不抓正文、不追更 | 本次只补元数据；抓正文是另一产品面 |
| 不执行 @js | ADR-0004 §7 已定；降级 + 点名，不静默失败 |
| 不自动删/并重复书 | 有损不可逆，单独立项 |
| 不在 serve 进程内跑 | 长任务跑 CLI/一次性容器，不占服务连接池 |

## 3. 命令形态

```text
norvels enrich [--db ...] [--apply] [--plan] [--fields author,intro,category]
               [--workers 4] [--qps 1] [--max-sources 3] [--timeout 10s]
               [--budget 30m] [--max-requests 20000] [--limit N]
               [--min-title-sim 0.86] [--resume --resume-batch <id>] [--flush-every 200]
norvels enrich --split-long-chapters [--apply] [--min-words 30000] [--target-words 8000]
norvels enrich --rollback <batchID> [--apply]
```

- **默认 dry-run**：只打印「会改什么」，不写库；`--apply` 才落库。`--fields` 同时约束候选、决策与写库——未选中的字段不判定、不写、不产生跳过原因。
- `--plan`：只出待补清单与规模，不联网（秒级，给运维先看量）。
- **流式落库**：`--apply` 先生成一个 batch id，之后每处理满 `--flush-every` 本（默认 200，0 = 只在结束时）就「记 intent → 改数据」一次并打一行进度（已处理/总数、命中、变更、已写库、请求、用时）；整批共用一个 batch id，故 `--rollback` 一次回滚整批，中途被杀只丢最后未满的一批。落库失败计入报告失败数并打到 stderr。

## 4. 分层与依赖方向

```text
cmd/norvels/enrich_cmd.go   编排：取待补清单 → 调 crawl → 调 enrich.Decide → 写库（enrich_stream.go 是其流式循环）
   ├─ internal/crawl        在线检索执行器（叶子：fetch + ruleexec + source + enrich）
   ├─ internal/enrich       纯决策（叶子：占位作者 / 投票 / 分类词表 / 切点）
   └─ internal/store        enrich_changes 落库、待补候选查询、SplitChapter 复用
```

`crawl` / `enrich` 都不依赖 store；store 只在编排层出现。两个新包登记在
`tests/tools/gatedeps/main.go` 的 `allowedDeps`。

## 5. 抓取执行器（并发与限流）

| 面 | 做法 |
|---|---|
| 并发 | 全局 worker 池 `--workers`（默认 4） |
| 限流 | **每个源一个 `fetch.Client`**（自带单源 QPS 匀速 + `MaxConcurrent`）：多源并行、单源匀速 |
| 源轮转 | 一本书按优先级问至多 `--max-sources` 个源，凑够 2 条相似命中即停 |
| 熔断 | 单源连续 5 次网络失败/超时 → 本次会话禁用该源，报告里点名 |
| 预算 | `--budget` / `--max-requests` 任一打满即优雅停：已得的写、未得的跳过（不算错误） |
| 断点 | `--resume --resume-batch <id>` 跳过该批次已处理过的 book_id（依赖流式落库：变更在跑的过程中就已进 `enrich_changes`） |
| 单次 | `--timeout`（默认 10s）；搜索页只取第 1 页 |

## 6. 决策与投票（`internal/enrich` 纯函数）

| 字段 | 规则 |
|---|---|
| 占位作者 | `IsPlaceholderAuthor`：白名单 + 归一后查表（不做「看起来不像人名」的黑名单） |
| 书名匹配 | 归一（去《》【】、标点、空白、全角、大小写）后算编辑距离相似度；低于阈值丢弃 |
| 作者 | 非占位命中按源优先级加权计票；全部占位则不写；书名归一后 ≤ `ShortTitleMaxRunes`（默认 2）rune 时整条放弃并记 `author:title_too_short`——同名不代表同一本，只跳过 author，不动库 |
| 简介 | 取满足 ≥30 字的最长命中；原简介已达新值 80% 长度时不覆盖（不动用户手填的） |
| 分类 | 先 `Classify`（书名 + 简介前 200 字的本地词表）；未命中才用源给的 kind |
| **无命中** | 不是「无事可做」：分类可完全离线判定，所以没有书源命中时仍走词表 |
| 置信度 | 支持票数 / 总命中数；低于 0.6 不写，只记原因（`author:placeholder_only` 等） |

## 7. 超长章节切分

对 `word_count >= --min-words` 的章按 `--target-words` 在**段落边界**上算切点，
调 `store.SplitChapter`。切点计算是 `enrich.SplitCuts` 纯函数（可单测，不碰库）：
切点严格递增、两侧必须有字、最多 99 段、尾段不足目标一半时撤销最后一刀。

**整篇没有换行的不硬切**（一本 40 万字只有 5 个换行的脏数据很常见）：报告里单列「无段落边界」计数交给人处理，硬切字符只会产出碎章。

## 8. 落库与回滚

迁移 `0027_enrich_changes`：`enrich_changes(id, batch_id, book_id, field, old_value,
new_value, source_id, confidence, created_at)` + `(batch_id)`、`(book_id, created_at)` 索引。

回滚按 batch 把 `old_value` 写回，统一走 `store.UpdateBookMeta`（作者变更要同步 `books_authors` 与作者聚合列，裸 SQL 会让 `/api/authors` 的作品数漂掉）。单条失败不中断整批：中断会留下「回滚一半」这种最难收拾的状态。

## 9. 验收判据

1. `--plan` 的候选数字与 `/api/health/report` 对得上（缺简介 / 超长章节）。
2. `--apply` 后 `/api/health/report` 的缺简介数下降且**没有新增**缺作者。
3. `--rollback <batch> --apply` 能把该批次的字段还原（作者书数回落到原值）。
4. 抓取路径单测覆盖：QPS 间隔、熔断、预算耗尽、@js 降级、相似度过滤。
5. `make gate` 九项全绿；`scripts/privacy-scan.sh --all` 零命中。

## 10. 风险与处置

| 风险 | 处置 |
|---|---|
| 书源站不可达/改版 | 熔断 + 报告点名；换书源 JSON 即可，不必改代码 |
| 错配同名书 | 相似度阈值 + 多源投票 + 变更日志可回滚；抓取打满带宽由 QPS 1 / 并发 4 / 预算兜住 |
| 规则包含真实站点 | 仓库只放虚构示例；真实书源留在部署侧 settings |
