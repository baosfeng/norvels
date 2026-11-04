package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// 0022 预计算的三条不变量，每条都有测试守着：
//  1. **增量 == 全量**：导入/删改路径的增量维护结果，必须与「清空后重算」逐项相等；
//  2. **缓存 == 实时**：读接口走预计算表与走原实时 SQL 必须给出同一组数字；
//  3. **缺失不 500**：缓存不可用时接口降级并标 stale，不返回错误。

// ---------- 缓存 == 实时 ----------

// TestImportTrendCacheMatchesLive 校验预计算路径与实时路径给出同一张趋势表。
//
// 两条路径的归并口径（本地日、脏时间跳过）必须逐字一致：一边 parseTime → Local()，
// 另一边读表里的本地日 key，任何一边偏一天都会让驾驶舱少一天数据。
func TestImportTrendCacheMatchesLive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	cached := requireTrend(t, st, 14)
	live := liveImportTrend(t, ctx, st, 14)
	if len(cached) != len(live) {
		t.Fatalf("天数 = %d，实时 %d", len(cached), len(live))
	}
	for i := range cached {
		if cached[i] != live[i] {
			t.Errorf("第 %d 天：缓存 %+v，实时 %+v", i, cached[i], live[i])
		}
	}
}

// liveImportTrend 用**原实现**（books 全表按 imported_at 归日 + chapters JOIN）
// 算一遍趋势，作为对照基准。
func liveImportTrend(t *testing.T, ctx context.Context, st *Store, days int) []ImportTrendPoint {
	t.Helper()
	start := trendStartDay(days)
	out := make([]ImportTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		out = append(out, ImportTrendPoint{Date: start.AddDate(0, 0, i).Format(dateLayout)})
	}
	books, chapters, err := st.trendCounts(ctx, start)
	if err != nil {
		t.Fatalf("trendCounts: %v", err)
	}
	fillTrend(out, books, chapters)

	// 章节口径另用「JOIN 到 chapters 逐行计数」验一遍：这是本次消灭掉的语句，
	// 但它的结果才是「章节按书归日」的定义，缓存路径必须与它相等而不是自说自话。
	legacy := countChaptersByDayLegacy(t, ctx, st, start)
	for i := range out {
		if out[i].Chapters != legacy[out[i].Date] {
			t.Errorf("%s 章节数 = %d，章节 JOIN 逐行口径 %d",
				out[i].Date, out[i].Chapters, legacy[out[i].Date])
		}
	}
	return out
}

// countChaptersByDayLegacy 是 0022 之前那条语句的等价实现（JOIN chapters 逐行归日）。
func countChaptersByDayLegacy(t *testing.T, ctx context.Context, st *Store, start time.Time) map[string]int {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT b.imported_at FROM chapters c JOIN books b ON b.id = c.book_id WHERE b.imported_at >= ?",
		start)
	if err != nil {
		t.Fatalf("legacy trend: %v", err)
	}
	defer rows.Close()
	out := make(map[string]int, 16)
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("legacy scan: %v", err)
		}
		if !raw.Valid || raw.String == "" {
			continue
		}
		t, err := parseTime(raw.String)
		if err != nil {
			continue
		}
		out[t.Local().Format(dateLayout)]++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("legacy rows: %v", err)
	}
	return out
}

// TestListAuthorsAggMatchesLive 校验物化聚合与实时聚合给出同一页作者。
//
// 三种排序都比：作者的作品数与字数一旦漂了，驾驶舱的「字数 Top 10」会悄悄换成
// 另一批作者，而这种错很难被肉眼发现。
func TestListAuthorsAggMatchesLive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	for _, sortKey := range []string{"books", "words", "name"} {
		cached, err := st.listAuthorsFromAgg(ctx, sortKey, "", nil, 50, 0)
		if err != nil {
			t.Fatalf("listAuthorsFromAgg(%s): %v", sortKey, err)
		}
		live, err := st.listAuthorsLive(ctx, sortKey, "", nil, 50, 0)
		if err != nil {
			t.Fatalf("listAuthorsLive(%s): %v", sortKey, err)
		}
		if len(cached) != len(live) {
			t.Fatalf("%s：条数 = %d，实时 %d", sortKey, len(cached), len(live))
		}
		for i := range cached {
			if cached[i] != live[i] {
				t.Errorf("%s 第 %d 条：物化 %+v，实时 %+v", sortKey, i, cached[i], live[i])
			}
		}
	}
}

// TestLengthHistMatchesLive 校验预计算直方图与实时区间 COUNT 一致。
func TestLengthHistMatchesLive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	cached, err := st.lengthHistFromTable(ctx)
	if err != nil {
		t.Fatalf("lengthHistFromTable: %v", err)
	}
	live, err := bucketCountsOn(ctx, st.DB(), "word_count", "chapters", lengthBucketEdges)
	if err != nil {
		t.Fatalf("bucketCountsOn: %v", err)
	}
	if !equalInts(cached, live) {
		t.Errorf("长度直方图：预计算 %v，实时 %v", cached, live)
	}
}

// TestStatsChaptersMatchesCount 校验 /api/stats 的章节数来自缓存且等于实时 COUNT。
func TestStatsChaptersMatchesCount(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	exact, err := st.scalarInt64(ctx, chapterCountSQL)
	if err != nil {
		t.Fatalf("精确章节数: %v", err)
	}
	if int64(stats.Chapters) != exact {
		t.Errorf("Stats.Chapters = %d，精确 %d", stats.Chapters, exact)
	}
	if stats.CompositionEstimated {
		t.Error("缓存有效时不应标 estimated")
	}
}

// TestStatsChapterCountCacheMiss 覆盖「章节数缓存缺失」：退回实时 COUNT 并标 estimated。
func TestStatsChapterCountCacheMiss(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	exact, err := st.scalarInt64(ctx, chapterCountSQL)
	if err != nil {
		t.Fatalf("精确章节数: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM store_metrics WHERE key = ?", metricChapterCount); err != nil {
		t.Fatalf("清章节数缓存: %v", err)
	}
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("缓存缺失时 Stats 不应报错: %v", err)
	}
	if int64(stats.Chapters) != exact {
		t.Errorf("章节数 = %d，精确 %d", stats.Chapters, exact)
	}
	if !stats.CompositionEstimated {
		t.Error("章节数走实时 COUNT 时应标 CompositionEstimated")
	}
}

// ---------- 缓存缺失时的降级 ----------

// TestPrecomputeMissingFallsBack 覆盖「预计算表被删/未回填」时读接口的表现：
// 数字仍然正确，标记 stale，绝不返回错误。
func TestPrecomputeMissingFallsBack(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	want := captureDashboardViews(t, ctx, st)
	clearPrecomputeContent(t, ctx, st)

	got := captureDashboardViews(t, ctx, st)
	assertTrendEqual(t, "降级后", want.trend, got.trend)
	assertStatsEqual(t, "降级后", want.stats, got.stats)
	assertAuthorPageEqual(t, "降级后", want.authors, got.authors)
	if got.stats.CompositionEstimated {
		// 章节数不进预计算表（它走 store_metrics），所以这里是**仍然走缓存**：
		// 预计算表缺失不影响 /api/stats 的章节数，仍是精确值、也不标 estimated。
		// 「章节数缓存缺失」是另一条独立场景，见 TestStatsChapterCountCacheMiss。
		t.Error("预计算表缺失不应影响章节数缓存（它走 store_metrics）")
	}
	if !got.health.Stale {
		t.Error("降级后健康报告应标 stale")
	}
	if n := len(got.health.ChapterLengthBuckets); n != len(lengthBucketLabels) {
		t.Errorf("降级后长度直方图 = %v", got.health.ChapterLengthBuckets)
	}
	assertBucketsEqual(t, "降级后长度直方图", want.health.ChapterLengthBuckets, got.health.ChapterLengthBuckets)
}

// dashboardViews 是四个驾驶舱读接口的一次快照（用于「降级前后必须一致」的比对）。
type dashboardViews struct {
	trend   []ImportTrendPoint
	stats   Stats
	authors Page[AuthorSummary]
	health  HealthReport
}

// captureDashboardViews 依次取四个接口（任一失败即 Fatal）。
func captureDashboardViews(t *testing.T, ctx context.Context, st *Store) dashboardViews {
	t.Helper()
	trend := requireTrend(t, st, 7)
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	authors, err := st.ListAuthors(ctx, AuthorQuery{Sort: "words", Page: 1, Size: 5})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	return dashboardViews{
		trend:   trend,
		stats:   stats,
		authors: authors,
		health:  st.HealthReport(ctx),
	}
}

// clearPrecomputeContent 模拟「三张预计算表的内容没算，只剩空壳」：
// 表都在但值为空，且口径版本缺失——这正是「迁移跑完但回填没跑完 / 回填失败」的形态。
func clearPrecomputeContent(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, table := range []string{tableChapterLengthHist, tableImportDaily, tableAuthorAgg} {
		if _, err := st.DB().ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("清 %s: %v", table, err)
		}
	}
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM store_metrics WHERE key = ?", metricPrecomputeVersion); err != nil {
		t.Fatalf("清口径版本: %v", err)
	}
	st.resetPrecomputeCache()
}

// assertTrendEqual 逐日比对趋势。
func assertTrendEqual(t *testing.T, label string, want, got []ImportTrendPoint) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s趋势天数 = %d，期望 %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s第 %d 天 = %+v，期望 %+v", label, i, got[i], want[i])
		}
	}
}

// assertStatsEqual 比对 /api/stats 的计数口径（体积构成另算，见各用例）。
func assertStatsEqual(t *testing.T, label string, want, got Stats) {
	t.Helper()
	if got.Chapters != want.Chapters {
		t.Errorf("%s章节数 = %d，期望 %d", label, got.Chapters, want.Chapters)
	}
	if got.Books != want.Books || got.Authors != want.Authors || got.Words != want.Words {
		t.Errorf("%s统计 = %+v，期望 books/authors/words = %d/%d/%d",
			label, got, want.Books, want.Authors, want.Words)
	}
}

// assertAuthorPageEqual 逐条比对作者页。
func assertAuthorPageEqual(t *testing.T, label string, want, got Page[AuthorSummary]) {
	t.Helper()
	if got.Total != want.Total {
		t.Errorf("%s作者总数 = %d，期望 %d", label, got.Total, want.Total)
	}
	if len(got.Items) != len(want.Items) {
		t.Fatalf("%s作者条数 = %d，期望 %d", label, len(got.Items), len(want.Items))
	}
	for i := range want.Items {
		if got.Items[i] != want.Items[i] {
			t.Errorf("%s第 %d 位作者 = %+v，期望 %+v", label, i, got.Items[i], want.Items[i])
		}
	}
}

// assertBucketsEqual 逐桶比对直方图。
func assertBucketsEqual(t *testing.T, label string, want, got []Bucket) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s桶数 = %d，期望 %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s第 %d 桶 = %+v，期望 %+v", label, i, got[i], want[i])
		}
	}
}

// TestPrecomputeMissingTablesNoError 覆盖「三张表根本不存在」（0022 之前的库）。
//
// 这时读接口必须照常返回 200：迁移还没跑完就打开驾驶舱是真实存在的场景
// （容器启动后迁移与端口监听之间的窗口，或运维回滚到旧镜像读新库之外的形态）。
func TestPrecomputeMissingTablesNoError(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	for _, table := range []string{tableChapterLengthHist, tableImportDaily, tableAuthorAgg} {
		if _, err := st.DB().ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatalf("删表 %s: %v", table, err)
		}
	}
	st.resetPrecomputeCache()

	if _, err := st.ImportTrend(ctx, 7); err != nil {
		t.Errorf("表不存在时 ImportTrend 应降级而非报错: %v", err)
	}
	if _, err := st.Stats(ctx); err != nil {
		t.Errorf("表不存在时 Stats 应降级而非报错: %v", err)
	}
	if _, err := st.ListAuthors(ctx, AuthorQuery{Sort: "words", Page: 1, Size: 5}); err != nil {
		t.Errorf("表不存在时 ListAuthors 应降级而非报错: %v", err)
	}
	r := st.HealthReport(ctx)
	if len(r.Warnings) != 0 {
		t.Errorf("健康报告告警 = %v，期望空（降级不算失败）", r.Warnings)
	}
	if len(r.ChapterLengthBuckets) != len(lengthBucketLabels) {
		t.Errorf("长度直方图 = %v，期望 %d 桶", r.ChapterLengthBuckets, len(lengthBucketLabels))
	}
}

// TestPrecomputeZoneChangeInvalidates 覆盖「时区变了」：import_daily 的 key 是本地日，
// 偏移一变旧 key 就落到别的天，必须判过期而不是照读。
func TestPrecomputeZoneChangeInvalidates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)

	if ok, _ := st.PrecomputeStatus(ctx); !ok {
		t.Fatal("初始状态应就绪")
	}
	// 把记录下来的偏移改成一个当前不可能的值（当前偏移 ± 60 分钟）。
	if _, err := st.DB().ExecContext(ctx, "UPDATE store_metrics SET value = ? WHERE key = ?",
		int64(currentZoneOffset())+60, metricPrecomputeZone); err != nil {
		t.Fatalf("改时区偏移: %v", err)
	}
	st.resetPrecomputeCache()
	ok, why := st.PrecomputeStatus(ctx)
	if ok {
		t.Fatal("时区偏移变了仍判就绪")
	}
	if !strings.Contains(why, "时区") {
		t.Errorf("原因 = %q，期望提到时区", why)
	}
	// 读接口降级后仍返回正确数字。
	if _, err := st.ImportTrend(ctx, 7); err != nil {
		t.Errorf("时区失效后 ImportTrend 应降级: %v", err)
	}
}
