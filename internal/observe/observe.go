// Package observe 采集服务端运行时性能指标，给「慢」一个可衡量的口径。
//
// 四类数据全部是进程内内存态（重启即清零，不落库）：
//
//   - HTTP 路由级耗时直方图（固定分桶 + p50/p95/p99），按路由模板聚合，不按真实路径；
//   - 慢请求环形缓冲（容量 200，最新在前），带 request id 与本次请求的 SQL 次数/耗时；
//   - SQL 模板级聚合（归一化后按模板计次，超阈值的慢查询单独计数）；
//   - 运行时快照（goroutines / heap / GC / 已登记连接池的 sql.DBStats）。
//
// 基数上限 512：路由与 SQL 模板超出后归并到 "<其他>"，内存上限 < 8 MB。
// NORVELS_METRICS=off 时所有 Observe* 退化为零分配空操作（ConfigureFromEnv 读取）。
//
// 与 api 的接线见 internal/api/observe_routes.go：路由用 Wrap 包装（按 pattern 记模板），
// 中间件只负责在超过慢阈值时补一行 "api: SLOW" 日志。
package observe

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// 基数与容量上限：宁可归并也不要无界增长。
const (
	maxSeries   = 512
	otherKey    = "<其他>"
	slowRingCap = 200
)

// 默认阈值（毫秒）；环境变量 NORVELS_SLOW_MS / NORVELS_SQL_SLOW_MS 可覆盖。
const (
	defaultSlowMs    = 500
	defaultSQLSlowMs = 100
)

// 包级状态：阈值用原子变量（每条请求读一次），开关只由 ConfigureFromEnv 改。
var (
	disabled  bool
	slowNs    atomic.Int64
	sqlSlowNs atomic.Int64
	startedAt = time.Now()
)

func init() {
	slowNs.Store(int64(defaultSlowMs) * int64(time.Millisecond))
	sqlSlowNs.Store(int64(defaultSQLSlowMs) * int64(time.Millisecond))
}

// ConfigureFromEnv 读取环境变量开关；server 装配时调用一次，测试可重复调用。
//
//   - NORVELS_METRICS=off：整体关闭（Observe* 空操作，SnapshotNow 只剩运行时与时长的骨架）；
//   - NORVELS_SLOW_MS：HTTP 慢请求阈值，默认 500；
//   - NORVELS_SQL_SLOW_MS：慢 SQL 阈值，默认 100。
//
// 非法或非正数按「不覆盖」处理：宁可保留默认阈值，也不要因为写错环境变量把观测关掉。
func ConfigureFromEnv() {
	disabled = strings.EqualFold(strings.TrimSpace(os.Getenv("NORVELS_METRICS")), "off")
	if d, ok := envMillis("NORVELS_SLOW_MS"); ok {
		SetSlowThreshold(d)
	}
	if d, ok := envMillis("NORVELS_SQL_SLOW_MS"); ok {
		SetSQLSlowThreshold(d)
	}
}

func envMillis(name string) (time.Duration, bool) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, false
	}
	return time.Duration(n) * time.Millisecond, true
}

// SetSlowThreshold 设置 HTTP 慢请求阈值（超过即进 Slow 环形缓冲并打 SLOW 日志）。
func SetSlowThreshold(d time.Duration) {
	if d <= 0 {
		return
	}
	slowNs.Store(int64(d))
}

// SetSQLSlowThreshold 设置「慢 SQL」阈值（QueryStat.SlowCount 的口径）。
func SetSQLSlowThreshold(d time.Duration) {
	if d <= 0 {
		return
	}
	sqlSlowNs.Store(int64(d))
}

// SlowThreshold 返回当前 HTTP 慢请求阈值（api 中间件判断要不要补 SLOW 日志）。
func SlowThreshold() time.Duration { return time.Duration(slowNs.Load()) }

// SQLSlowThreshold 返回当前慢 SQL 阈值。
func SQLSlowThreshold() time.Duration { return time.Duration(sqlSlowNs.Load()) }

// ResetForTest 清空全部聚合与慢请求缓冲，并把阈值恢复默认；驱动注册与连接池登记保留。
func ResetForTest() {
	resetHTTP()
	resetSQL()
	slowNs.Store(int64(defaultSlowMs) * int64(time.Millisecond))
	sqlSlowNs.Store(int64(defaultSQLSlowMs) * int64(time.Millisecond))
	disabled = false
}

// RouteStat 是一条路由模板的聚合结果。
type RouteStat struct {
	Route        string  `json:"route"` // "GET /api/books/{id}"，模板而非真实路径
	Count        int64   `json:"count"`
	Errors       int64   `json:"errors"`       // status >= 500
	ClientErrors int64   `json:"clientErrors"` // 400 <= status < 500
	AvgMs        float64 `json:"avgMs"`
	MaxMs        float64 `json:"maxMs"`
	P50Ms        float64 `json:"p50Ms"`
	P95Ms        float64 `json:"p95Ms"`
	P99Ms        float64 `json:"p99Ms"`
	Buckets      []int64 `json:"buckets"` // 长度 = len(BucketEdgesMs)+1
}

// SlowRequest 是一条慢请求（At 为 RFC3339，路径含 query 供复现）。
type SlowRequest struct {
	At        string  `json:"at"`
	Route     string  `json:"route"`
	Method    string  `json:"method"`
	Path      string  `json:"path"`
	Status    int     `json:"status"`
	DurMs     float64 `json:"durMs"`
	SQLCount  int     `json:"sqlCount"`
	SQLMs     float64 `json:"sqlMs"`
	RequestID string  `json:"requestId,omitempty"`
}

// QueryStat 是按归一化 SQL 模板聚合的结果。
type QueryStat struct {
	Query     string  `json:"query"`
	Count     int64   `json:"count"`
	Errors    int64   `json:"errors"`
	SlowCount int64   `json:"slowCount"`
	AvgMs     float64 `json:"avgMs"`
	MaxMs     float64 `json:"maxMs"`
	P95Ms     float64 `json:"p95Ms"`
}

// PoolStat 是一个已登记连接池的 sql.DBStats 快照（WaitMs 是累计等待毫秒）。
type PoolStat struct {
	Name      string  `json:"name"`
	Open      int     `json:"open"`
	InUse     int     `json:"inUse"`
	Idle      int     `json:"idle"`
	WaitCount int64   `json:"waitCount"`
	WaitMs    float64 `json:"waitMs"`
}

// RuntimeStat 是进程级运行时快照。
type RuntimeStat struct {
	GoVersion  string     `json:"goVersion"`
	Goroutines int        `json:"goroutines"`
	HeapMB     float64    `json:"heapMB"`
	NumGC      uint32     `json:"numGC"`
	Pools      []PoolStat `json:"pools"`
}

// Snapshot 是 GET /api/metrics 的完整响应体（字段名与前端冻结）。
type Snapshot struct {
	StartedAt string        `json:"startedAt"`
	UptimeSec int64         `json:"uptimeSec"`
	HTTP      []RouteStat   `json:"http"`
	Slow      []SlowRequest `json:"slowRequests"`
	SQL       []QueryStat   `json:"sql"`
	Runtime   RuntimeStat   `json:"runtime"`
}

// SnapshotNow 生成本刻快照：HTTP / SQL 按名称排序，慢请求最新在前，空集给 [] 而非 null。
func SnapshotNow() Snapshot {
	now := time.Now()
	snap := Snapshot{
		StartedAt: startedAt.Format(time.RFC3339),
		UptimeSec: int64(now.Sub(startedAt).Seconds()),
		HTTP:      snapshotRoutes(),
		Slow:      snapshotSlow(),
		SQL:       snapshotSQL(),
		Runtime:   snapshotRuntime(),
	}
	sort.Slice(snap.HTTP, func(i, j int) bool { return snap.HTTP[i].Route < snap.HTTP[j].Route })
	sort.Slice(snap.SQL, func(i, j int) bool {
		if snap.SQL[i].Count != snap.SQL[j].Count {
			return snap.SQL[i].Count > snap.SQL[j].Count
		}
		return snap.SQL[i].Query < snap.SQL[j].Query
	})
	return snap
}

// ObserveHTTP 记录一次请求（3 参版本，供只有耗时的调用方使用）。
//
// api 的路由包装走 ObserveHTTPRequest，能额外带上方法 / 路径 / request id 与本次 SQL 统计；
// 这里没有那些上下文，SlowRequest 的对应字段留空。
func ObserveHTTP(route string, status int, d time.Duration) {
	observeHTTP(nil, route, "", "", "", status, d)
}

// durToMs 把 time.Duration 换成毫秒浮点（保留两位小数，避免 JSON 里出现 12.345678901）。
func durToMs(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return round2(float64(d) / float64(time.Millisecond))
}

// round2 四舍五入到两位小数：读数为人类服务，不需要纳秒级噪声。
func round2(v float64) float64 {
	if v < 0 {
		return 0
	}
	return float64(int64(v*100+0.5)) / 100
}
