# 运行时观测

口径、分桶边界与定位方法见 [observability.md](../observability.md)；本页只写接口契约。

## GET /api/metrics

本刻的进程内性能快照。**需要登录**（不在免鉴权名单里；开放模式无感）。

```json
{
  "startedAt": "2026-11-07T00:00:00+08:00",
  "uptimeSec": 812,
  "http": [
    {
      "route": "GET /api/health/report",
      "count": 20, "errors": 0, "clientErrors": 0,
      "avgMs": 38.2, "maxMs": 202, "p50Ms": 25, "p95Ms": 100, "p99Ms": 202,
      "buckets": [0, 2, 5, 8, 3, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    }
  ],
  "slowRequests": [
    {
      "at": "2026-11-07T00:05:00+08:00",
      "route": "GET /api/health/report", "method": "GET",
      "path": "/api/health/report?x=1", "status": 200,
      "durMs": 202, "sqlCount": 27, "sqlMs": 25.4, "requestId": "6f1c…"
    }
  ],
  "sql": [
    { "query": "SELECT COUNT(*) FROM chapter_fingerprints", "count": 3, "errors": 0,
      "slowCount": 1, "avgMs": 66.3, "maxMs": 97.5, "p95Ms": 66.3 }
  ],
  "runtime": {
    "goVersion": "go1.27.3", "goroutines": 18, "heapMB": 5.6, "numGC": 71,
    "pools": [
      { "name": "main", "open": 7, "inUse": 0, "idle": 7, "waitCount": 0, "waitMs": 0 }
    ]
  }
}
```

字段：

| 字段 | 含义 |
|---|---|
| `http[]` | 按路由**模板**聚合；`buckets` 长度是分桶边界数 + 1；`errors` = 5xx，`clientErrors` = 4xx |
| `slowRequests[]` | 超过 `NORVELS_SLOW_MS` 的请求，**最新在前**，最多 200 条；`path` 含 query，可直接重放 |
| `sql[]` | 按归一化 SQL 模板聚合，按次数降序；`slowCount` 是超过 `NORVELS_SQL_SLOW_MS` 的次数 |
| `runtime.pools[]` | 已登记连接池（`main` 主库 / `content` 正文库）；`waitMs` 是累计等待毫秒 |
| `startedAt` / `uptimeSec` | 进程启动时间与本刻运行秒数（快照重启清零的锚点） |

语义：

- `http` / `sql` / `slowRequests` 恒为数组，空集是 `[]` 不是 `null`。
- `http` 按 `route` 字典序、`sql` 按 `count` 降序，便于前端稳定渲染。
- 本次 `GET /api/metrics` 自己不在 `http` 里：快照在请求结束前生成。
- `NORVELS_METRICS=off` 时三个数组恒为空，`runtime` 照常返回（仍能看 goroutine / 连接池）。

状态码：`200` 正常；启用口令但未登录 `401`；非 GET `405`（带 `Allow: GET`）。
