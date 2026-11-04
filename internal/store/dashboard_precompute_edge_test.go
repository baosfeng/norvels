package store

import (
	"context"
	"strings"
	"testing"
)

// 0022 预计算的三条不变量，每条都有测试守着：
//  1. **增量 == 全量**：导入/删改路径的增量维护结果，必须与「清空后重算」逐项相等；
//  2. **缓存 == 实时**：读接口走预计算表与走原实时 SQL 必须给出同一组数字；
//  3. **缺失不 500**：缓存不可用时接口降级并标 stale，不返回错误。

// ---------- 边界 ----------

// TestBumpLengthHistBuckets 覆盖直方图增量维护的桶边界（含加/减两侧）。
func TestBumpLengthHistBuckets(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 直接清空，让断言只看本次增量。
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM "+tableChapterLengthHist); err != nil {
		t.Fatalf("清直方图: %v", err)
	}
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交与否都不影响断言
	// 边界值：0 / 999 / 1000 / 3000 / 5000 / 10000 / 极大。
	if err := bumpLengthHist(ctx, tx, []int{0, 999, 1000, 3000, 5000, 10000, 1 << 30}, nil); err != nil {
		t.Fatalf("bumpLengthHist: %v", err)
	}
	// 再从「1万+」桶里减掉一个。
	if err := bumpLengthHist(ctx, tx, nil, []int{20000}); err != nil {
		t.Fatalf("bumpLengthHist 减: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	// <1千: 0,999 | 1千-3千: 1000 | 3千-5千: 3000 | 5千-1万: 5000 | 1万+: 10000,1<<30 再加 −20000
	want := []int{2, 1, 1, 1, 1}
	got, err := st.lengthHistFromTable(ctx)
	if err != nil {
		t.Fatalf("lengthHistFromTable: %v", err)
	}
	if !equalInts(got, want) {
		t.Errorf("直方图 = %v，期望 %v", got, want)
	}
}

// TestLengthHistIgnoresOutOfRangeBuckets 覆盖脏数据：越界桶号与负计数不得污染直方图。
func TestLengthHistIgnoresOutOfRangeBuckets(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM "+tableChapterLengthHist); err != nil {
		t.Fatalf("清直方图: %v", err)
	}
	for _, row := range []struct {
		bucket int
		count  int
	}{{0, 3}, {-1, 99}, {99, 42}, {1, -5}, {2, 7}} {
		if _, err := st.DB().ExecContext(ctx,
			"INSERT INTO "+tableChapterLengthHist+"(bucket, count) VALUES (?, ?)", row.bucket, row.count); err != nil {
			t.Fatalf("插入脏桶: %v", err)
		}
	}
	got, err := st.lengthHistFromTable(ctx)
	if err != nil {
		t.Fatalf("lengthHistFromTable: %v", err)
	}
	if want := []int{3, 0, 7, 0, 0}; !equalInts(got, want) {
		t.Errorf("直方图 = %v，期望 %v（越界桶与负计数都被忽略）", got, want)
	}
}

// TestImportTrendSkipsBadTimestamps 覆盖脏时间：解析不了的行不进趋势表，
// 两条路径（写侧 localDayKey / 读侧降级 countByDayFromBooks）都要跳过它。
func TestImportTrendSkipsBadTimestamps(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := savePrecomputeBook(t, st, samplePrecomputeBook("脏时间样本", "作者丁", "/books/bad.txt", "bad"))
	if _, err := st.DB().ExecContext(ctx, "UPDATE books SET imported_at = ? WHERE id = ?",
		"不是时间", bookID); err != nil {
		t.Fatalf("写脏时间: %v", err)
	}
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	points := requireTrend(t, st, 3)
	for _, p := range points {
		if p.Books != 0 {
			t.Errorf("%s 书数 = %d，脏时间应被跳过", p.Date, p.Books)
		}
	}
	// 降级路径同样跳过。
	live := liveImportTrend(t, ctx, st, 3)
	for i := range points {
		if points[i] != live[i] {
			t.Errorf("第 %d 天：缓存 %+v，实时 %+v", i, points[i], live[i])
		}
	}
}

// TestPrecomputeEmptyLibrary 覆盖空库：三张表可以是空的，接口返回 0 不是错误。
func TestPrecomputeEmptyLibrary(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if ok, why := st.PrecomputeStatus(ctx); !ok {
		t.Fatalf("空库也应就绪（回填过就是有效状态）: %s", why)
	}
	points := requireTrend(t, st, 3)
	for _, p := range points {
		if p.Books != 0 || p.Chapters != 0 {
			t.Errorf("%s = %+v，空库应为 0", p.Date, p)
		}
	}
	if _, err := st.Stats(ctx); err != nil {
		t.Errorf("空库 Stats: %v", err)
	}
	if page, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 5}); err != nil || len(page.Items) != 0 {
		t.Errorf("空库 ListAuthors = %v, %v", page.Items, err)
	}
}

// TestPrecomputeStatusReportsMissingTable 覆盖状态探测的三种不可用形态。
func TestPrecomputeStatusReportsMissingTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.DB().ExecContext(ctx, "DROP TABLE "+tableAuthorAgg); err != nil {
		t.Fatalf("删表: %v", err)
	}
	st.resetPrecomputeCache()
	ok, why := st.PrecomputeStatus(ctx)
	if ok {
		t.Fatal("表被删后仍判就绪")
	}
	if !strings.Contains(why, tableAuthorAgg) {
		t.Errorf("原因 = %q，期望点名缺失的表", why)
	}
}

// TestRefreshMetricsRecomputesPrecompute 校验 refresh-metrics（运维入口）会把三张表重算。
func TestRefreshMetricsRecomputesPrecompute(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	before := readPrecomputeSnapshot(t, ctx, st)

	// 人为破坏三张表，再让 RefreshMetrics 修回来。
	for _, table := range []string{tableChapterLengthHist, tableImportDaily, tableAuthorAgg} {
		if _, err := st.DB().ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("清 %s: %v", table, err)
		}
	}
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	after := readPrecomputeSnapshot(t, ctx, st)
	assertSnapshotsEqual(t, "破坏后重算", before, after)
}
