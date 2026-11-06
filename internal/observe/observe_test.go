package observe

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBucketIndexBounds(t *testing.T) {
	cases := []struct {
		ms   float64
		want int
	}{
		{0, 0}, {0.4, 0}, {0.5, 1}, {1, 2}, {2.5, 3}, {5000, 13}, {60000, numBuckets - 1}, {1e9, numBuckets - 1},
	}
	for _, c := range cases {
		if got := bucketIndex(c.ms); got != c.want {
			t.Fatalf("bucketIndex(%v) = %d, want %d", c.ms, got, c.want)
		}
	}
	if len(BucketEdgesMs)+1 != numBuckets {
		t.Fatalf("桶数应该等于边界数 + 1：edges=%d buckets=%d", len(BucketEdgesMs), numBuckets)
	}
}

func TestHistogramQuantiles(t *testing.T) {
	var h histogram
	for i := 0; i < 100; i++ {
		h.add(1)
	}
	// 全部样本落在 1ms：分位必须等于实测值（桶内插值由 maxMs 收口）。
	for _, q := range []float64{0.5, 0.95, 0.99} {
		if got := h.quantile(q, 100, 1); got != 1 {
			t.Fatalf("q=%v 得到 %v，期望 1", q, got)
		}
	}

	var mixed histogram
	for i := 1; i <= 100; i++ {
		mixed.add(float64(i))
	}
	p50 := mixed.quantile(0.5, 100, 100)
	p95 := mixed.quantile(0.95, 100, 100)
	p99 := mixed.quantile(0.99, 100, 100)
	if !(p50 <= p95 && p95 <= p99 && p99 <= 100) {
		t.Fatalf("分位必须单调且不超过最大值：p50=%v p95=%v p99=%v", p50, p95, p99)
	}
	if mixed.quantile(0.95, 0, 0) != 0 {
		t.Fatal("没有样本时分位必须是 0")
	}
}

func TestObserveHTTPAggregates(t *testing.T) {
	ResetForTest()
	SetSlowThreshold(time.Millisecond)
	t.Cleanup(ResetForTest)

	recordSampleRequests()
	snap := SnapshotNow()
	ok := mustRoute(t, snap.HTTP, "GET /api/ok")
	if ok.Count != 2 || ok.Errors != 1 || ok.ClientErrors != 0 {
		t.Fatalf("计数不对：count=%d errors=%d clientErrors=%d", ok.Count, ok.Errors, ok.ClientErrors)
	}
	if len(snap.HTTP) != 2 {
		t.Fatalf("GET 与 POST 是两条路由，实际 %d 条：%+v", len(snap.HTTP), snap.HTTP)
	}
	if len(ok.Buckets) != numBuckets {
		t.Fatalf("桶数应为 %d，实际 %d", numBuckets, len(ok.Buckets))
	}
	if ok.MaxMs < 3 {
		t.Fatalf("max 应记录最大样本，实际 %v", ok.MaxMs)
	}
	if !(ok.P50Ms <= ok.P95Ms && ok.P95Ms <= ok.P99Ms && ok.P99Ms <= ok.MaxMs) {
		t.Fatalf("分位必须单调且不超过 max：%+v", ok)
	}
}

func TestSlowRequestThreshold(t *testing.T) {
	ResetForTest()
	SetSlowThreshold(time.Millisecond)
	t.Cleanup(ResetForTest)

	recordSampleRequests()
	snap := SnapshotNow()
	// 只有 3ms / 10ms 两条超过 1ms 阈值；0.5ms 那条不进慢请求。
	if len(snap.Slow) != 2 {
		t.Fatalf("慢请求应有 2 条，实际 %d：%+v", len(snap.Slow), snap.Slow)
	}
	if snap.Slow[0].Route != "POST /api/ok" || snap.Slow[0].DurMs != 10 {
		t.Fatalf("慢请求应为最新在前，实际首条 %+v", snap.Slow[0])
	}
}

// recordSampleRequests 写三条样本：0.5ms（不慢）/ 3ms（慢，500）/ 10ms（慢，404）。
func recordSampleRequests() {
	ObserveHTTP("GET /api/ok", http.StatusOK, 500*time.Microsecond)
	ObserveHTTP("GET /api/ok", http.StatusInternalServerError, 3*time.Millisecond)
	ObserveHTTP("POST /api/ok", http.StatusNotFound, 10*time.Millisecond)
}

// mustRoute 从快照里取指定路由的聚合，缺失时直接失败。
func mustRoute(t *testing.T, stats []RouteStat, name string) *RouteStat {
	t.Helper()
	for i := range stats {
		if stats[i].Route == name {
			return &stats[i]
		}
	}
	t.Fatalf("快照缺少路由 %s：%+v", name, stats)
	return nil
}

func TestSlowRingKeepsNewest200(t *testing.T) {
	ResetForTest()
	SetSlowThreshold(time.Nanosecond)
	t.Cleanup(ResetForTest)
	for i := 0; i < slowRingCap+30; i++ {
		ObserveHTTP("GET /api/ring", http.StatusOK, time.Duration(i+1)*time.Millisecond)
	}
	snap := SnapshotNow()
	if len(snap.Slow) != slowRingCap {
		t.Fatalf("环形缓冲应恒为 %d 条，实际 %d", slowRingCap, len(snap.Slow))
	}
	if snap.Slow[0].DurMs <= snap.Slow[len(snap.Slow)-1].DurMs {
		t.Fatal("慢请求应按时间倒序（最新在前）")
	}
}

func TestRouteCardinalityCap(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	for i := 0; i < maxSeries+20; i++ {
		ObserveHTTP("GET /api/x/"+string(rune('a'+i%26))+string(rune('a'+i/26)), 200, time.Millisecond)
	}
	snap := SnapshotNow()
	if len(snap.HTTP) > maxSeries+1 {
		t.Fatalf("路由基数超过上限：%d", len(snap.HTTP))
	}
	found := false
	for _, stat := range snap.HTTP {
		if stat.Route == otherKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("超过基数上限后应出现 %q 归并项", otherKey)
	}
}

func TestSQLNormalization(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SELECT * FROM books WHERE id = 12 AND title = 'x'", "SELECT * FROM books WHERE id = ? AND title = ?"},
		{"SELECT * FROM books WHERE id IN (1, 2, 3)", "SELECT * FROM books WHERE id IN (?)"},
		{"SELECT   *\nFROM\tbooks", "SELECT * FROM books"},
		{"SELECT * FROM books2 WHERE v = 1", "SELECT * FROM books2 WHERE v = ?"},
		{"SELECT * FROM t WHERE a = 'It''s'", "SELECT * FROM t WHERE a = ?"},
		{"SELECT * FROM t WHERE score >= 1.5e3", "SELECT * FROM t WHERE score >= ?"},
		// 未闭合引号按到结尾处理，不能把整条语句吞掉。
		{"SELECT 'oops", "SELECT ?"},
	}
	for _, c := range cases {
		if got := normalizeSQLSlow(c.in); got != c.want {
			t.Fatalf("normalizeSQLSlow(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	long := "SELECT * FROM t WHERE " + strings.Repeat("a = 1 AND ", 100) + "z = 2"
	if got := normalizeSQLSlow(long); len(got) > maxQueryBytes+len("…") {
		t.Fatalf("超长 SQL 应截断到 %d 字节内，实际 %d", maxQueryBytes, len(got))
	}
}

func TestSQLAggregationMergesTemplates(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	SetSQLSlowThreshold(10 * time.Millisecond)
	ObserveSQL("SELECT * FROM books WHERE id = 1", time.Millisecond, nil)
	ObserveSQL("SELECT * FROM books WHERE id = 2", 20*time.Millisecond, nil)
	ObserveSQL("SELECT * FROM books WHERE id = 3", time.Millisecond, context.Canceled)

	snap := SnapshotNow()
	if len(snap.SQL) != 1 {
		t.Fatalf("三条实参应归一化成同一个模板，实际 %d 条：%+v", len(snap.SQL), snap.SQL)
	}
	q := snap.SQL[0]
	if q.Query != "SELECT * FROM books WHERE id = ?" {
		t.Fatalf("模板归一化不对：%q", q.Query)
	}
	if q.Count != 3 || q.Errors != 1 || q.SlowCount != 1 {
		t.Fatalf("SQL 计数不对：%+v", q)
	}
	if q.MaxMs < 20 || q.P95Ms > 20 {
		t.Fatalf("SQL 分位/最大值不对：%+v", q)
	}
}

func TestSQLCardinalityCap(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	for i := 0; i < maxSeries+10; i++ {
		ObserveSQL("SELECT * FROM t"+string(rune('a'+i%26))+string(rune('a'+i/26))+" WHERE id = ?", time.Millisecond, nil)
	}
	snap := SnapshotNow()
	if len(snap.SQL) > maxSeries+1 {
		t.Fatalf("SQL 模板基数超过上限：%d", len(snap.SQL))
	}
}

func TestWrapRecordsRouteAndSQL(t *testing.T) {
	ResetForTest()
	SetSlowThreshold(time.Nanosecond) // 每条请求都进慢请求缓冲，便于断言 SQL 关联
	t.Cleanup(ResetForTest)

	db := newObservedDB(t)
	h := Wrap("GET /api/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		var n int
		if err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM t").Scan(&n); err != nil {
			t.Errorf("查询失败: %v", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/books/7?x=1", nil)
	h(httptest.NewRecorder(), req)

	snap := SnapshotNow()
	stat := mustRoute(t, snap.HTTP, "GET /api/books/{id}")
	if stat.Errors != 1 {
		t.Fatalf("500 应计入 errors：%+v", stat)
	}
	if len(snap.Slow) != 1 {
		t.Fatalf("应有 1 条慢请求，实际 %d", len(snap.Slow))
	}
	assertSlowCarriesRequestDetail(t, snap.Slow[0])
	assertSelectTemplate(t, snap.SQL)
}

// newObservedDB 打开一个内存库并建一张只有一行的表（走包装驱动）。
func newObservedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open(DriverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("CREATE TABLE t(id INTEGER)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	return db
}

// assertSlowCarriesRequestDetail 校验慢请求记录了可复现的信息与本次 SQL 统计。
func assertSlowCarriesRequestDetail(t *testing.T, slow SlowRequest) {
	t.Helper()
	if slow.Method != http.MethodGet || !strings.Contains(slow.Path, "?x=1") {
		t.Fatalf("慢请求应带真实路径（含 query）：%+v", slow)
	}
	if slow.SQLCount != 1 || slow.SQLMs < 0 {
		t.Fatalf("慢请求应关联本次 SQL 次数，实际 count=%d ms=%v", slow.SQLCount, slow.SQLMs)
	}
}

// assertSelectTemplate 校验 SQL 按模板记录（建表语句也会被记录，只找 SELECT 那条）。
func assertSelectTemplate(t *testing.T, stats []QueryStat) {
	t.Helper()
	for i := range stats {
		if stats[i].Query == "SELECT COUNT(*) FROM t" {
			return
		}
	}
	t.Fatalf("SQL 应记录 SELECT 模板：%+v", stats)
}

func TestMetricsOffIsNoOp(t *testing.T) {
	ResetForTest()
	t.Setenv("NORVELS_METRICS", "off")
	ConfigureFromEnv()
	t.Cleanup(func() {
		t.Setenv("NORVELS_METRICS", "")
		ResetForTest()
	})

	ObserveHTTP("GET /api/books", 200, time.Second)
	ObserveSQL("SELECT 1", time.Second, nil)
	snap := SnapshotNow()
	if len(snap.HTTP) != 0 || len(snap.SQL) != 0 || len(snap.Slow) != 0 {
		t.Fatalf("关闭后不应记录任何指标：http=%d sql=%d slow=%d", len(snap.HTTP), len(snap.SQL), len(snap.Slow))
	}
}

func TestConfigureFromEnvThresholds(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	t.Setenv("NORVELS_SLOW_MS", "1500")
	t.Setenv("NORVELS_SQL_SLOW_MS", "250")
	ConfigureFromEnv()
	if SlowThreshold() != 1500*time.Millisecond || SQLSlowThreshold() != 250*time.Millisecond {
		t.Fatalf("阈值未从环境变量生效：%v / %v", SlowThreshold(), SQLSlowThreshold())
	}
	// 非法值不覆盖默认阈值。
	t.Setenv("NORVELS_SLOW_MS", "abc")
	SetSlowThreshold(900 * time.Millisecond)
	ConfigureFromEnv()
	if SlowThreshold() != 900*time.Millisecond {
		t.Fatalf("非法环境变量不应覆盖已有阈值，实际 %v", SlowThreshold())
	}
}

func TestRegisterPoolAndRuntime(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	db, err := sql.Open(DriverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close() //nolint:errcheck // 测试结束关闭
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	RegisterPool(db, "unit-test")
	RegisterPool(nil, "ignored") // nil 应被忽略

	snap := SnapshotNow()
	if snap.Runtime.GoVersion == "" || snap.Runtime.Goroutines == 0 {
		t.Fatalf("运行时信息缺失：%+v", snap.Runtime)
	}
	found := false
	for _, pool := range snap.Runtime.Pools {
		if pool.Name == "unit-test" {
			found = true
			if pool.Open < 1 {
				t.Fatalf("连接池 open 计数异常：%+v", pool)
			}
		}
		if pool.Name == "ignored" {
			t.Fatal("nil 连接不应被登记")
		}
	}
	if !found {
		t.Fatalf("登记的连接池没有出现在快照里：%+v", snap.Runtime.Pools)
	}
}

func TestSnapshotJSONShape(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	raw, err := json.Marshal(SnapshotNow())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	for _, key := range []string{`"startedAt"`, `"uptimeSec"`, `"http":[]`, `"slowRequests":[]`, `"sql":[]`, `"runtime"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("快照 JSON 缺字段 %s：%s", key, body)
		}
	}
}

func TestDriverForwardsContextCancellation(t *testing.T) {
	db, err := sql.Open(DriverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close() //nolint:errcheck // 测试结束关闭
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t(id INTEGER)"); err == nil {
		t.Fatal("已取消的 ctx 应该让查询失败（Queryer/Execer 未被转发成 Prepare 路径）")
	}
}
