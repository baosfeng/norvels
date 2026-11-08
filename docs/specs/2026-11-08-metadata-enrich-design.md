# 元数据补全与超长章节切分：enrich（2026-11-08）

> 触发：生产 18.2 万本库里 9.7 万本缺简介、8816 个超长章节、分类全部落在「未分类」，
> 作者名里「佚名 / Unknown / 未知 / 不详」并存（字数 Top10 里占位名合计 281.3 亿字）。
> 目标：一条命令按在线书源补元数据，并把占位作者、未分类、超长章节一起收敛。

## 1. 实测基线

生产 `GET /api/health/report` 与本机 838 本样本库 SQL：

| 项 | 值 | 含义 |
|---|---|---|
| 总书数 | 182898 | — |
| 缺简介 | 96949 | `TRIM(intro)=''` |
| 缺作者（author_id=0） | 0 | **假象**：导入把空作者落成「佚名」，所以按 author_id 判永远是 0 |
| 占位作者书数 | 445 / 838（本机样本） | 佚名 436、不详 7、多人 4 …… |
| 超长章节（≥3 万字） | 8816 | 切章失败的兜底痕迹 |
| 分类 | 100%「未分类」 | 只按导入目录推断，txt 目录本身没有分类信息 |
| 正文库悬空引用 | 15002 | 只报不修（缺正文只能重导），**不在本设计范围** |

## 2. 结论

| 做什么 | 为什么 |
|---|---|
| `norvels enrich` 子命令 + `scripts/enrich.sh` | 运维一条命令；本地与容器同一入口 |
| 复用 `internal/fetch` | SSRF/端口/重定向/QPS/并发四道边界已过 ADR-0004 §3，没有理由另写 HTTP 客户端 |
| 复用 `internal/ruleexec` + `source.Online` | 书源是 Legado JSON，规则求值器已存在；@js 字段降级为空并在报告里点名 |
| 多源投票而非单源取值 | 单源错名错作者常见，投票 + 相似度阈值才挡得住 |
| 默认 dry-run + 变更日志 + `--rollback` | 9.7 万本量级的写必须可逆 |
| 超长章节走 `store.SplitChapter` | 已有实现是「只改 block_off/block_len」的偏移改写，不重压正文 |

| 不做什么 | 为什么 |
|---|---|
| 不抓正文、不追更 | 本次只补元数据；抓正文是另一产品面 |
| 不执行 @js | ADR-0004 §7 已定；降级 + 点名，不静默失败 |
| 不自动删/并重复书 | 有损不可逆，单独立项 |
| 不在 serve 进程内跑 | 长任务跑 CLI/一次性容器，不占服务连接池 |

## 3. 命令形态

```text
norvels enrich --db /data/norvels.db [--apply] [--plan]
               [--fields author,intro,category]   # 默认三样全开
               [--workers 4] [--qps 1] [--max-sources 3] [--timeout 10s]
               [--budget 30m] [--max-requests 20000] [--resume]
               [--limit N] [--min-title-sim 0.86]
norvels enrich --split-long-chapters [--apply] [--target-words 8000] [--min-words 30000]
norvels enrich --rollback <batchID> [--apply]
```

- **默认 dry-run**：只打印「会改什么」，不写库；`--apply` 才落库。
- `--plan`：只出待补清单与规模，不联网（秒级，给运维先看量）。
- 每次 `--apply` 生成一个 batch id，写 `enrich_changes`（旧值/新值/来源/批次）。

## 4. 分层与依赖方向

```text
cmd/norvels/enrich_cmd.go   编排：取待补清单 → 调 crawl → 调 enrich.Decide → 写库
   ├─ internal/crawl        在线检索执行器（叶子包：fetch + ruleexec + source）
   ├─ internal/enrich       纯决策（叶子包：占位作者 / 投票 / 分类映射 / 切点计算）
   └─ internal/store        enrich_changes 落库、待补候选查询、SplitChapter 复用
```

- `crawl` / `enrich` 都是叶子包，**不依赖 store**；store 只在编排层出现。
- 新增两个包必须登记进 `tests/tools/gatedeps/main.go` 的 `allowedDeps`。

## 5. 抓取执行器（并发与限流）

| 面 | 做法 |
|---|---|
| 并发 | 全局 worker 池 `--workers`（默认 4）；每本书一个任务，池大小即同时在飞的检索数 |
| 限流 | **每个源一个 `fetch.Client`**（自带单源 QPS 匀速 + `MaxConcurrent` 信号量）：多源并行、单源匀速 |
| 源轮转 | 一本书按优先级依次问至多 `--max-sources` 个源，凑够 2 个高置信命中即停 |
| 熔断 | 单源连续 5 次网络失败/超时 → 本次会话禁用该源，报告里点名 |
| 预算 | `--budget`（墙钟）/ `--max-requests`（请求数）任一打满即优雅停：已得的写、未得的跳过 |
| 断点 | `--resume` 跳过本批次已处理过的 book_id（读 enrich_changes 的 traced 集合） |
| 单次 | 单源单次检索 `--timeout`（默认 10s）；搜索页默认 1 页（`{{page}}` 支持但不自动翻页） |

## 6. 决策与投票（`internal/enrich` 纯函数）

| 字段 | 规则 |
|---|---|
| 占位作者 | `IsPlaceholderAuthor`：佚名/无名/未知/Unknown/不详/多人/作者未知/N/A（大小写与空白归一） |
| 书名匹配 | 归一（去《》书名号、标点、空白、全角）后算相似度；低于 `--min-title-sim` 的命中丢弃 |
| 作者 | 非占位命中按**源优先级加权**计票；平票取优先级高者；全部占位则不写 |
| 简介 | 取「满足 ≥30 字」的最长命中；原简介已 ≥ 新值 80% 长度时不覆盖（不动用户手填的） |
| 分类 | 先 `Classify(title, intro)` 走本地词表（书名+简介，命中即定 L1/L2）；词表未命中才用源给的 kind |
| 置信度 | 命中数 × 权重归一；低于阈值一律不写，只记 `skipped_low_confidence` |

## 7. 超长章节切分

`--split-long-chapters`：对 `word_count >= --min-words`（默认 30000）的章，按目标 `--target-words`（默认 8000）
在**段落边界**上算切点，调 `store.SplitChapter`。约束：`maxSplitPieces` 上限（99 段），切点两侧必须有字，
切完的标题由 store 统一加「（i/N）」。切点计算是 `internal/enrich.SplitCuts` 纯函数（可单测，不碰库）。

## 8. 落库与回滚

迁移 `0027_enrich_changes`：`enrich_changes(id, batch_id, book_id, field, old_value, new_value, source_id, confidence, created_at)`。
索引 `(batch_id)` 与 `(book_id, created_at)`。回滚按 batch 把 `old_value` 写回（作者走 `UpdateBookMeta`，保证作者聚合列同步）。

## 9. 验收判据

1. `--plan` 在真实库上报出三类待补清单，数字与 `/api/health/report` 对得上（缺简介 / 超长章节）。
2. `--apply` 前后各跑一次 `/api/health/report`，缺简介数下降且**没有新增**缺作者。
3. `--rollback <batch> --apply` 能把该 batch 的字段还原到原值（作者书数回落到原值）。
4. 抓取路径有单测：QPS 间隔、熔断、预算耗尽、SSRF 拒绝、@js 降级。
5. `make gate` 九项全绿；`scripts/privacy-scan.sh --all` 零命中。

## 10. 风险

| 风险 | 处置 |
|---|---|
| 书源站不可达/改版 | 熔断 + 报告点名；换书源 JSON 即可，不必改代码 |
| 错配同名书 | 相似度阈值 + 多源投票 + 变更日志可回滚 |
| 抓取把 NAS 带宽打满 | 默认 QPS 1 / 并发 4 / 单次 10s；预算上限兜底 |
| 规则包含真实站点 | 仓库只放虚构示例；真实书源留在部署侧 settings |
