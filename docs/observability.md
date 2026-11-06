# 运行时性能观测

「慢」要能被衡量：本页说明服务端采集了哪些性能指标、怎么读、怎么用来定位慢接口。
所有指标都是**进程内内存态**（重启清零、不落库、零第三方依赖），入口是 `GET /api/metrics`。

## 采集了什么

| 维度 | 粒度 | 数据来源 | 用途 |
|---|---|---|---|
| HTTP | 路由**模板**（`GET /api/books/{id}`），上限 512 | api 路由包装（`observe.Wrap`） | 哪一类接口慢（次数/分位/错误数） |
| 慢请求 | 单条请求，环形 200 条 | 超过 `NORVELS_SLOW_MS` 的请求 | 带路径与 request id 的现场复现 |
| SQL | 归一化**模板**，上限 512 | SQLite 计时驱动（`sqlite-observed`） | 是接口慢还是某条 SQL 慢 |
| 运行时 | 进程级快照 | `runtime` + 已登记连接池的 `sql.DBStats` | goroutine 泄漏 / 堆 / GC / 连接池排队 |

## 口径（读数字前先看这几条）

- **按模板聚合，不按真实路径**：`/api/books/12` 与 `/api/books/13` 同属 `GET /api/books/{id}`；否则序列数会被书籍 id 撑爆。
- **分位是分桶插值**：估算值落在样本所在的桶内线性插值，误差上限是**一个桶宽**；再用实测最大值收口（分位不会超过 max）。样本很少时以 max 为准更可靠。
- **错误**：`errors` 是 5xx，`clientErrors` 是 4xx（含「书不存在」这类 404 与前端主动取消导致的 500）。
- **SQL 计时**：从驱动调用开始到返回为止（`QueryContext`/`ExecContext`），**不含** Rows 的逐行 Scan——那一层看 CPU profile。
- **SQL 归一化**：单引号字面量、数字字面量 → `?`；`IN (?, ?, ?)` → `IN (?)`；连续空白折叠；超过 400 字节截断。`books2` 这类标识符里的数字不被替换。
- **基数上限 512**：路由或 SQL 模板超过上限后归并到 `<其他>`，内存上限 < 8 MB。
- **本次请求不在快照里**：快照在请求结束前生成，`GET /api/metrics` 自己只统计「已经结束」的请求。

分桶边界（毫秒，桶 i = `[边[i-1], 边[i])`，末桶 `>= 60000`）：

```
0.5  1  2.5  5  10  25  50  100  250  500  1000  2500  5000  10000  30000  60000
```

## 开关

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `NORVELS_METRICS` | （空） | `off` 关闭全部采集：`Observe*` 变成零分配空操作，快照里 HTTP/SQL/慢请求都是空数组 |
| `NORVELS_SLOW_MS` | `500` | HTTP 慢请求阈值；超过即进慢请求环、并在访问日志补一行 `api: SLOW …` |
| `NORVELS_SQL_SLOW_MS` | `100` | 慢 SQL 阈值，只影响 `QueryStat.slowCount` |

非法值（非数字 / ≤0）按「不覆盖」处理，保留默认阈值——写错环境变量不该把观测关掉。

## 定位慢接口的三步法

1. **看分位，不看平均**：`curl -s localhost:8080/api/metrics | jq '.http | sort_by(.p95Ms) | reverse | .[:10]'`。
   p95 高而 p50 低 = 少数请求被拖慢（锁/大查询/冷启动）；p50 也高 = 每次都慢，先怀疑 SQL 与索引。
2. **分账：接口时间 vs SQL 时间**：在 `slowRequests` 里找到那条请求的 `sqlCount` / `sqlMs`。
   `sqlMs` 接近 `durMs` → 瓶颈在库；`sqlMs` 很小 → 瓶颈在 Go 侧（序列化、聚合、模板渲染）。
3. **按模板复现**：`slowRequests[].path` 是含 query 的真实路径，直接重放；再看 `sql[]` 里该模板的
   `p95Ms`/`slowCount`，对可疑语句跑 `EXPLAIN QUERY PLAN` 确认是否走索引。

访问日志里超过阈值的那一行带有同样的字段，可直接 `grep 'api: SLOW'` 对齐时间点。

## 成本与边界

- 每条 SQL 约 100 ns 固定开销（两次 `time.Now` + 一次模板查找 + 一次计数；同一条语句重复执行走连接级缓存）。
  3.5 µs 级的查询实测 +2.8%（`BenchmarkNativeQuery` vs `BenchmarkWrappedQuery`）、写语句 +1.4%
  （`-benchtime 100000x`）；真实查询（百微秒级）影响 < 1%。
- 包装驱动**完整转发** `QueryerContext`/`ExecerContext`/`ConnBeginTx`/`SessionResetter`/`Pinger`/`Validator`/`NamedValueChecker`：
  少转发一个，`database/sql` 就退化成 Prepare + Query，加了观测反而更慢。
- 不替代 CPU/heap profile：这里回答「慢在哪一类接口 / 哪条 SQL」，不回答「哪一行代码」。
- 不跨重启、不聚合历史：要趋势请接外部时序库，本模块刻意不存历史（避免两套口径）。
