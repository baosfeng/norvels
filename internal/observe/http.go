package observe

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// routeAgg 是一条路由模板的运行时聚合（只在 httpMu 保护下读写）。
type routeAgg struct {
	route        string
	count        int64
	errors       int64
	clientErrors int64
	sumMs        float64
	maxMs        float64
	hist         histogram
}

// httpState 是 HTTP 侧的全部内存态：路由聚合 + 慢请求环形缓冲。
type httpState struct {
	mu     sync.Mutex
	routes map[string]*routeAgg
	other  *routeAgg
	slow   [slowRingCap]SlowRequest
	n      int64 // 累计写入条数（环形索引用 n % slowRingCap）
}

var httpSt = httpState{routes: map[string]*routeAgg{}}

func resetHTTP() {
	httpSt.mu.Lock()
	httpSt.routes = map[string]*routeAgg{}
	httpSt.other = nil
	httpSt.slow = [slowRingCap]SlowRequest{}
	httpSt.n = 0
	httpSt.mu.Unlock()
}

// lookupRouteLocked 取（必要时建）路由聚合；超出基数上限归并到 otherKey。
func (st *httpState) lookupRouteLocked(route string) *routeAgg {
	if agg, ok := st.routes[route]; ok {
		return agg
	}
	if len(st.routes) >= maxSeries {
		if st.other == nil {
			st.other = &routeAgg{route: otherKey}
		}
		return st.other
	}
	agg := &routeAgg{route: route}
	st.routes[route] = agg
	return agg
}

// observeHTTP 写入一次请求：耗时直方图 + 慢请求环形缓冲。
func observeHTTP(tr *reqTracker, route, method, path, rid string, status int, d time.Duration) {
	if disabled {
		return
	}
	ms := durToMs(d)
	httpSt.mu.Lock()
	agg := httpSt.lookupRouteLocked(route)
	agg.count++
	agg.sumMs += ms
	if ms > agg.maxMs {
		agg.maxMs = ms
	}
	agg.hist.add(ms)
	switch {
	case status >= 500:
		agg.errors++
	case status >= 400:
		agg.clientErrors++
	}
	if d >= SlowThreshold() {
		sqlCount, sqlMs := 0, 0.0
		if tr != nil {
			sqlCount, sqlMs = tr.stats()
		}
		httpSt.pushSlowLocked(SlowRequest{
			At:        time.Now().Format(time.RFC3339),
			Route:     route,
			Method:    method,
			Path:      path,
			Status:    status,
			DurMs:     ms,
			SQLCount:  sqlCount,
			SQLMs:     sqlMs,
			RequestID: rid,
		})
	}
	httpSt.mu.Unlock()
}

// pushSlowLocked 写入环形缓冲（覆盖最旧的一条）。
func (st *httpState) pushSlowLocked(rec SlowRequest) {
	st.slow[st.n%slowRingCap] = rec
	st.n++
}

// snapshotRoutes 复制路由聚合为快照（未排序，排序交给 SnapshotNow）。
func snapshotRoutes() []RouteStat {
	httpSt.mu.Lock()
	out := make([]RouteStat, 0, len(httpSt.routes)+1)
	for _, agg := range httpSt.routes {
		out = append(out, agg.snapshot())
	}
	if httpSt.other != nil {
		out = append(out, httpSt.other.snapshot())
	}
	httpSt.mu.Unlock()
	return out
}

// snapshot 把聚合转成 RouteStat（分位由直方图与 max 共同估算）。
func (a *routeAgg) snapshot() RouteStat {
	stat := RouteStat{
		Route:        a.route,
		Count:        a.count,
		Errors:       a.errors,
		ClientErrors: a.clientErrors,
		MaxMs:        round2(a.maxMs),
		Buckets:      a.hist.snapshot(),
	}
	if a.count > 0 {
		stat.AvgMs = round2(a.sumMs / float64(a.count))
	}
	stat.P50Ms = a.hist.quantile(0.50, a.count, a.maxMs)
	stat.P95Ms = a.hist.quantile(0.95, a.count, a.maxMs)
	stat.P99Ms = a.hist.quantile(0.99, a.count, a.maxMs)
	return stat
}

// snapshotSlow 返回慢请求，最新在前（前端只展示前 N 条）。
func snapshotSlow() []SlowRequest {
	httpSt.mu.Lock()
	defer httpSt.mu.Unlock()
	n := httpSt.n
	if n > slowRingCap {
		n = slowRingCap
	}
	out := make([]SlowRequest, 0, n)
	for k := int64(0); k < n; k++ {
		idx := (httpSt.n - 1 - k) % slowRingCap
		out = append(out, httpSt.slow[idx])
	}
	return out
}

// reqTracker 是一次请求内的 SQL 计时累加器（同一请求可能并发查询，用原子量）。
type reqTracker struct {
	count atomic.Int64
	ns    atomic.Int64
}

func (t *reqTracker) addSQL(d time.Duration) {
	t.count.Add(1)
	t.ns.Add(int64(d))
}

// stats 返回本次请求的 SQL 次数与总毫秒。
func (t *reqTracker) stats() (int, float64) {
	return int(t.count.Load()), durToMs(time.Duration(t.ns.Load()))
}

// reqTrackerKey 是 ctx 中追踪器的键。
type reqTrackerKey struct{}

func trackerFromCtx(ctx context.Context) *reqTracker {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(reqTrackerKey{}).(*reqTracker)
	return t
}

// ObserveHTTPRequest 记录一次已带上下文的请求（api 路由包装的唯一入口）。
func ObserveHTTPRequest(ctx context.Context, route, method, path string, status int, d time.Duration) {
	observeHTTP(trackerFromCtx(ctx), route, method, path, "", status, d)
}

// Wrap 包装一条路由的 handler：按 route（方法 + 模板，如 "GET /api/books/{id}"）记录耗时与状态。
//
// 它在 ctx 上挂一个请求级 SQL 追踪器，随后由包装驱动在每次查询后累加；
// request id 取响应头（requestIDMiddleware 已写好的那个），没开就留空。
func Wrap(route string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if disabled {
			h(w, r)
			return
		}
		start := time.Now()
		cw := &captureWriter{ResponseWriter: w}
		tr := &reqTracker{}
		r = r.WithContext(context.WithValue(r.Context(), reqTrackerKey{}, tr))
		h(cw, r)
		ms := time.Since(start)
		rid := cw.Header().Get("X-Request-Id")
		observeHTTP(tr, route, r.Method, requestPath(r), rid, cw.status(), ms)
	}
}

// requestPath 是「含 query 的真实路径」，慢请求要靠它复现（模板给不出具体 id）。
func requestPath(r *http.Request) string {
	if r.URL == nil {
		return ""
	}
	u := *r.URL
	u.Scheme = ""
	u.Host = ""
	return u.RequestURI()
}

// captureWriter 记录状态码：只包一层，不改变任何响应语义。
type captureWriter struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (w *captureWriter) WriteHeader(code int) {
	if !w.wrote {
		w.code = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.code = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(p)
}

func (w *captureWriter) status() int {
	if !w.wrote {
		return http.StatusOK
	}
	return w.code
}

// Flush 转发给底层 writer（gzip / SSE 之类的流式响应要用）。
func (w *captureWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap 让 http.ResponseController 能拿到底层 writer。
func (w *captureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Hijack 转发给底层 writer（WebSocket / 连接劫持场景；底层不支持时按标准库语义报错）。
func (w *captureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hj.Hijack()
}
