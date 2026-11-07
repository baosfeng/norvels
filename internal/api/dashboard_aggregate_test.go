package api

import (
	"net/http"
	"testing"
	"time"

	"norvels/internal/store"
)

// GET /api/dashboard 的契约：一次返回驾驶舱全部数字块，块形状与各自端点一致，
// 短 TTL 缓存命中时不重复取数，参数越界一律 400。

// TestDashboardAggregatesEveryBlock 七块齐全、数组块是 [] 而非 null、缓存与 TTL 字段合理。
func TestDashboardAggregatesEveryBlock(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/dashboard")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	d := decodeBody[dashboardPayload](t, rr)
	if d.Cached {
		t.Error("首次请求不该命中缓存")
	}
	if d.TTLSeconds <= 0 {
		t.Errorf("ttlSeconds = %v，期望默认 5 秒量级", d.TTLSeconds)
	}
	for name, ok := range map[string]bool{
		"stats": d.Stats != nil, "health": d.Health != nil,
		"readingStats": d.ReadingStats != nil, "topAuthors": d.TopAuthors != nil,
		"importTrend": d.ImportTrend != nil, "readingProgress": d.ReadingProgress != nil,
		"importJobs": d.ImportJobs != nil,
	} {
		if !ok {
			t.Errorf("数据块 %s 缺失（数组块也必须是 []，不能是 null）", name)
		}
	}
	if len(d.Warnings) != 0 {
		t.Fatalf("空库聚合不该有告警: %v", d.Warnings)
	}
}

// TestDashboardMatchesStandaloneEndpoints 聚合不是另一套算法：各块与独立端点同口径。
func TestDashboardMatchesStandaloneEndpoints(t *testing.T) {
	e := newEnv(t)
	d := decodeBody[dashboardPayload](t, e.get("/api/dashboard"))
	stats := decodeBody[store.Stats](t, e.get("/api/stats"))
	if d.Stats.Books != stats.Books || d.Stats.Words != stats.Words {
		t.Errorf("聚合 stats 与 /api/stats 不一致: %+v vs %+v", d.Stats, stats)
	}
	health := decodeBody[store.HealthReport](t, e.get("/api/health/report"))
	if d.Health.TotalBooks == nil || health.TotalBooks == nil || *d.Health.TotalBooks != *health.TotalBooks {
		t.Errorf("聚合 health 与 /api/health/report 不一致")
	}
	trend := e.get("/api/stats/import-trend?days=14")
	if len(d.ImportTrend) != 14 || trend.Code != http.StatusOK {
		t.Errorf("聚合 importTrend 天数 = %d，期望 14（单独端点 %d）", len(d.ImportTrend), trend.Code)
	}
}

// TestDashboardCacheWithinTTL TTL 内复用同一份结果（cached=true），过期后重新计算。
func TestDashboardCacheWithinTTL(t *testing.T) {
	e := newEnvWith(t, Config{DisableAccessLog: true, DashboardTTL: 40 * time.Millisecond}, testFS())
	if first := decodeBody[dashboardPayload](t, e.get("/api/dashboard")); first.Cached {
		t.Fatal("首次请求不该命中缓存")
	}
	second := decodeBody[dashboardPayload](t, e.get("/api/dashboard"))
	if !second.Cached {
		t.Fatal("TTL 内应命中缓存（cached=true）")
	}
	if second.AgeSeconds <= 0 {
		t.Errorf("缓存命中的 ageSeconds = %v，应为正数", second.AgeSeconds)
	}
	time.Sleep(80 * time.Millisecond)
	if third := decodeBody[dashboardPayload](t, e.get("/api/dashboard")); third.Cached {
		t.Fatal("TTL 过期后应重新计算（cached=false）")
	}
}

// TestDashboardCacheKeyIsolatedByUser 缓存键必须含账号：readingStats / readingProgress
// 按 user_id 隔离，只按参数缓存会让 5 秒 TTL 内第二个账号拿到第一个账号的个人阅读数据。
func TestDashboardCacheKeyIsolatedByUser(t *testing.T) {
	p := dashboardParams{Days: 14, ReadingDays: 90, TZ: 0, TopAuthors: 10, Progress: 50, Jobs: 20}
	if p.key(1) == p.key(2) {
		t.Fatalf("不同账号的缓存键相同：%s", p.key(1))
	}
	if p.key(1) != p.key(1) {
		t.Fatal("同一账号同一参数的缓存键不稳定")
	}
}

// TestDashboardRejectsBadParams 参数越界一律 400（不静默夹紧）。
func TestDashboardRejectsBadParams(t *testing.T) {
	e := newEnv(t)
	for _, target := range []string{
		"/api/dashboard?days=0",
		"/api/dashboard?days=367",
		"/api/dashboard?readingDays=0",
		"/api/dashboard?topAuthors=51",
		"/api/dashboard?progress=0",
		"/api/dashboard?jobs=101",
		"/api/dashboard?tz=9999",
	} {
		requireError(t, e.get(target), http.StatusBadRequest, codeInvalidParam)
	}
}

// TestDashboardStatsFailureIsError stats 是首屏核心块：取不到则整体失败（不返回半份 200）。
func TestDashboardStatsFailureIsError(t *testing.T) {
	e := newEnv(t)
	if err := e.st.Close(); err != nil {
		t.Fatalf("关闭库: %v", err)
	}
	if rr := e.get("/api/dashboard"); rr.Code < http.StatusInternalServerError {
		t.Fatalf("库不可用时状态码 = %d，期望 5xx", rr.Code)
	}
}
