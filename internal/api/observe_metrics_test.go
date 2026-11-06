package api

import (
	"net/http"
	"strings"
	"testing"

	"norvels/internal/observe"
)

// metricsRoutes 从快照里挑出路由名（断言用）。
func metricsRoutes(snap observe.Snapshot) []string {
	out := make([]string, 0, len(snap.HTTP))
	for _, stat := range snap.HTTP {
		out = append(out, stat.Route)
	}
	return out
}

func TestMetricsEndpoint(t *testing.T) {
	observe.ResetForTest()
	t.Cleanup(observe.ResetForTest)
	env := newEnv(t)

	if rr := env.get("/api/books?limit=1"); rr.Code != http.StatusOK {
		t.Fatalf("预热请求失败: %d %s", rr.Code, rr.Body.String())
	}
	rr := env.get("/api/metrics")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics = %d, body=%s", rr.Code, rr.Body.String())
	}
	snap := decodeBody[observe.Snapshot](t, rr)

	// 路由按模板聚合：真实路径 /api/books?limit=1 必须落到 "GET /api/books"。
	// （本次 /api/metrics 自己不在里面：快照在请求结束前生成，它只统计「已经结束」的请求。）
	assertRouteRecorded(t, metricsRoutes(snap), "GET /api/books")
	if snap.Slow == nil || snap.SQL == nil {
		t.Fatalf("空集必须是 [] 而不是 null：slow=%v sql=%v", snap.Slow, snap.SQL)
	}
	if len(snap.SQL) == 0 {
		t.Fatal("快照应包含 SQL 模板统计（预热请求已经查过库）")
	}
	assertPoolRegistered(t, snap.Runtime.Pools, "main")
	if snap.Runtime.GoVersion == "" {
		t.Fatal("运行时信息缺失")
	}
}

// assertRouteRecorded 断言某条路由模板出现在快照里。
func assertRouteRecorded(t *testing.T, routes []string, want string) {
	t.Helper()
	if !strings.Contains(strings.Join(routes, ","), want) {
		t.Fatalf("快照缺少路由 %s：%v", want, routes)
	}
}

// assertPoolRegistered 断言某个连接池被登记（main / content）。
func assertPoolRegistered(t *testing.T, pools []observe.PoolStat, want string) {
	t.Helper()
	names := make([]string, 0, len(pools))
	for _, pool := range pools {
		names = append(names, pool.Name)
	}
	if !strings.Contains(strings.Join(names, ","), want) {
		t.Fatalf("连接池 %s 应被登记：%v", want, names)
	}
}

func TestMetricsRequiresAuth(t *testing.T) {
	observe.ResetForTest()
	t.Cleanup(observe.ResetForTest)
	env := newEnvWith(t, Config{AuthToken: "s3cret", DisableAccessLog: true}, testFS())

	if rr := env.get("/api/metrics"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("未带令牌应 401，实际 %d", rr.Code)
	}
	rr := env.do(http.MethodGet, "/api/metrics", nil, map[string]string{"Authorization": "Bearer s3cret"})
	if rr.Code != http.StatusOK {
		t.Fatalf("带令牌应 200，实际 %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMetricsMethodNotAllowed(t *testing.T) {
	observe.ResetForTest()
	t.Cleanup(observe.ResetForTest)
	env := newEnv(t)

	rr := env.do(http.MethodPost, "/api/metrics", nil, nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/metrics 应 405，实际 %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Fatalf("405 应带 Allow: GET，实际 %q", allow)
	}
}
