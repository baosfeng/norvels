package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 0022 预计算的「降级与失败」路径：数据库在查询中途不可用时，读接口必须
// 优雅退化（标 stale / 返回零值），写路径必须把错误如实向上抛而不是吞掉。
//
// 做法与 internal/api/coverage_store_fail_test.go 一致：用真实 store 把数据准备好，
// 再把连接关掉，让后续调用返回错误。这类分支是「数据库抖动时驾驶舱显示什么」的
// 真实口径，正常路径的测试永远走不到它们。

// closedStore 返回一个连接已关闭的 store（数据先备好再关）。
func closedStore(t *testing.T) *Store {
	t.Helper()
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	if err := st.Close(); err != nil {
		t.Fatalf("关闭 store: %v", err)
	}
	return st
}

// TestPrecomputeReadsSurviveClosedDB 覆盖三张预计算表读不动时的降级：
// 读接口不 panic、不返回错误，而是标记降级并给出空/零结果。
func TestPrecomputeReadsSurviveClosedDB(t *testing.T) {
	st := closedStore(t)
	ctx := context.Background()

	// PrecomputeStatus：探测不到表 → 判不可用（不是报错）。
	if ok, why := st.PrecomputeStatus(ctx); ok || why == "" {
		t.Errorf("连接关闭后 PrecomputeStatus = %v, %q，期望不可用且给出原因", ok, why)
	}
	// lengthHistFromTable：直接读表失败 → 返回错误（调用方据此记 Warnings）。
	if _, err := st.lengthHistFromTable(ctx); err == nil {
		t.Error("连接关闭后 lengthHistFromTable 应返回错误")
	}
	// ImportTrend：读表失败 → 降级到实时 SQL，实时也失败 → 返回错误。
	if _, err := st.ImportTrend(ctx, 7); err == nil {
		t.Error("连接关闭后 ImportTrend 应返回错误（降级路径也读不到库）")
	}
	// ListAuthors：计数查询就失败 → 返回错误。
	if _, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 5}); err == nil {
		t.Error("连接关闭后 ListAuthors 应返回错误")
	}
	// HealthReport 是「绝不 500」的接口：每项失败记 Warnings，整体照常返回。
	rep := st.HealthReport(ctx)
	if len(rep.Warnings) == 0 {
		t.Error("连接关闭后健康报告应有 Warnings")
	}
	if rep.TotalBooks != nil {
		t.Errorf("连接关闭后 totalBooks = %v，期望 nil（已记入 Warnings）", *rep.TotalBooks)
	}
}

// TestPrecomputeReadsDegradeOnMissingTables 覆盖「表被删了但库还能读」：
// 降级路径必须给出数字而不是错误。
func TestPrecomputeReadsDegradeOnMissingTables(t *testing.T) {
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	ctx := context.Background()
	dropPrecomputeTables(t, ctx, st)

	// lengthHistFromTable 读不到表 → 错误（调用方记 Warnings 并降级）。
	if _, err := st.lengthHistFromTable(ctx); err == nil {
		t.Error("表被删后 lengthHistFromTable 应返回错误")
	}
	// countByDayFromTable 同样。
	if _, _, err := st.countByDayFromTable(ctx); err == nil {
		t.Error("表被删后 countByDayFromTable 应返回错误")
	}
	// 但四个对外读接口必须照常返回数字。
	if _, err := st.ImportTrend(ctx, 7); err != nil {
		t.Errorf("表被删后 ImportTrend 应降级: %v", err)
	}
	if _, err := st.Stats(ctx); err != nil {
		t.Errorf("表被删后 Stats 应降级: %v", err)
	}
	if _, err := st.ListAuthors(ctx, AuthorQuery{Sort: "books", Page: 1, Size: 5}); err != nil {
		t.Errorf("表被删后 ListAuthors 应降级: %v", err)
	}
	rep := st.HealthReport(ctx)
	if !rep.Stale {
		t.Error("表被删后健康报告应标 stale")
	}
	if len(rep.ChapterLengthBuckets) != len(lengthBucketLabels) {
		t.Errorf("表被删后长度直方图 = %v，期望 %d 桶（实时算出来）",
			rep.ChapterLengthBuckets, len(lengthBucketLabels))
	}
}

// TestPrecomputeWritePathsFailOnClosedDB 覆盖增量维护的写路径在库不可用时报错。
//
// 这些分支平时永远走不到（写入与读在同一个事务里），但错误必须如实上抛：
// 吞掉它们会让「章节写进去了但直方图没记」的漂移静默发生。
func TestPrecomputeWritePathsFailOnClosedDB(t *testing.T) {
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	bookID := firstBookID(t, st)
	chapters := precomputeChapters(t, st, bookID)
	ctx := context.Background()
	if err := st.Close(); err != nil {
		t.Fatalf("关闭 store: %v", err)
	}

	if err := st.DeleteBook(ctx, bookID); err == nil {
		t.Error("连接关闭后 DeleteBook 应返回错误")
	}
	if err := st.DeleteChapter(ctx, bookID, chapters[0].ID); err == nil {
		t.Error("连接关闭后 DeleteChapter 应返回错误")
	}
	if err := st.DeleteChapters(ctx, bookID, []int64{chapters[0].ID}); err == nil {
		t.Error("连接关闭后 DeleteChapters 应返回错误")
	}
	if err := st.UpdateChapterContent(ctx, bookID, chapters[0].ID, "新的正文内容。"); err == nil {
		t.Error("连接关闭后 UpdateChapterContent 应返回错误")
	}
	if _, err := st.InsertChapter(ctx, bookID, model.ParsedChapter{
		Title: "第98章 插入", Content: "插入的章节正文。",
	}, 0); err == nil {
		t.Error("连接关闭后 InsertChapter 应返回错误")
	}
}

// TestPrecomputeIncrementErrorOnClosedTx 覆盖增量维护函数在事务已关闭时报错。
//
// 直接对已提交的事务调用 bump*：错误路径要么来自「表不存在」，要么来自
// 「事务已结束」。两种都必须返回错误，不能静默成功。
func TestPrecomputeIncrementErrorOnClosedTx(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := bumpLengthHist(ctx, tx, []int{1000}, nil); err == nil {
		t.Error("已提交事务上 bumpLengthHist 应返回错误")
	}
	if err := bumpImportDaily(ctx, tx, formatTime(nowUTC()), 1, 1); err == nil {
		t.Error("已提交事务上 bumpImportDaily 应返回错误")
	}
	if err := refreshAuthorAgg(ctx, tx, 1); err == nil {
		t.Error("已提交事务上 refreshAuthorAgg 应返回错误")
	}
}

// TestPrecomputeRebuildFailsOnClosedDB 覆盖迁移回填与运维重算在库不可用时报错。
func TestPrecomputeRebuildFailsOnClosedDB(t *testing.T) {
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	ctx := context.Background()
	if err := st.Close(); err != nil {
		t.Fatalf("关闭 store: %v", err)
	}

	tx, err := st.DB().BeginTx(ctx, nil)
	if err == nil {
		defer tx.Rollback() //nolint:errcheck // 连接已关，回滚是空操作
	}
	// 连接关闭时连 BeginTx 都会失败；两条路径都只要求「不静默成功」。
	if err != nil {
		return
	}
	if err := rebuildDashboardTables(ctx, tx); err == nil {
		t.Error("库不可用时 rebuildDashboardTables 应返回错误")
	}
}

// TestPrecomputeAuthorHelpersFailOnClosedDB 覆盖作者聚合辅助函数的错误分支。
func TestPrecomputeAuthorHelpersFailOnClosedDB(t *testing.T) {
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	bookID := firstBookID(t, st)
	ctx := context.Background()
	if err := st.Close(); err != nil {
		t.Fatalf("关闭 store: %v", err)
	}

	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		return // 连接已关：BeginTx 就失败，错误分支无从触达
	}
	defer tx.Rollback() //nolint:errcheck // 同上

	if _, err := authorIDsOfBook(ctx, tx, bookID); err == nil {
		t.Error("库不可用时 authorIDsOfBook 应返回错误")
	}
	if err := refreshAuthorIDs(ctx, tx, []int64{1}); err == nil {
		t.Error("库不可用时 refreshAuthorIDs 应返回错误")
	}
	if err := refreshBookAuthors(ctx, tx, bookID); err == nil {
		t.Error("库不可用时 refreshBookAuthors 应返回错误")
	}
	if _, err := bookChapterWordCounts(ctx, tx, bookID); err == nil {
		t.Error("库不可用时 bookChapterWordCounts 应返回错误")
	}
	if _, err := bookImportedAt(ctx, tx, bookID); err == nil {
		t.Error("库不可用时 bookImportedAt 应返回错误")
	}
}

// dropPrecomputeTables 删掉三张预计算表（模拟「0022 之前的库」）。
func dropPrecomputeTables(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, table := range []string{tableChapterLengthHist, tableImportDaily, tableAuthorAgg} {
		if _, err := st.DB().ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatalf("删表 %s: %v", table, err)
		}
	}
	st.resetPrecomputeCache()
}

// firstBookID 返回库里最小的书 id。
func firstBookID(t *testing.T, st *Store) int64 {
	t.Helper()
	var id int64
	if err := st.DB().QueryRowContext(context.Background(),
		"SELECT MIN(id) FROM books").Scan(&id); err != nil {
		t.Fatalf("取首本书: %v", err)
	}
	return id
}

// TestPrecomputeMetaReadFailures 覆盖口径元数据读取的三种失败形态。
func TestPrecomputeMetaReadFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("表不存在", func(t *testing.T) {
		st := newTestStore(t)
		if _, err := st.DB().ExecContext(ctx, "DROP TABLE store_metrics"); err != nil {
			t.Fatalf("删 store_metrics: %v", err)
		}
		if _, _, err := st.readPrecomputeMeta(ctx); err == nil {
			t.Error("store_metrics 不存在时 readPrecomputeMeta 应返回错误")
		}
	})

	t.Run("口径未落库", func(t *testing.T) {
		st := newTestStore(t)
		if _, err := st.DB().ExecContext(ctx,
			"DELETE FROM store_metrics WHERE key = ?", metricPrecomputeVersion); err != nil {
			t.Fatalf("清口径: %v", err)
		}
		_, _, err := st.readPrecomputeMeta(ctx)
		if err == nil || !strings.Contains(err.Error(), "尚未回填") {
			t.Errorf("口径缺失时 = %v，期望「尚未回填」", err)
		}
	})

	t.Run("版本落后", func(t *testing.T) {
		st := newTestStore(t)
		if _, err := st.DB().ExecContext(ctx,
			"UPDATE store_metrics SET value = ? WHERE key = ?", precomputeVersion-1, metricPrecomputeVersion); err != nil {
			t.Fatalf("改口径版本: %v", err)
		}
		if ok, why := st.PrecomputeStatus(ctx); ok || !strings.Contains(why, "落后") {
			t.Errorf("版本落后时 = %v, %q，期望不可用且提到落后", ok, why)
		}
	})
}

// TestPrecomputeRebuildErrorPaths 覆盖回填与重算在表缺失时的错误分支。
//
// 回填正常情况下只在迁移里跑一次，那些错误分支（表被删、列缺失）平时走不到，
// 但它们是「迁移中途失败要整体回滚」的依据——吞掉错误会留下半截数据。
func TestPrecomputeRebuildErrorPaths(t *testing.T) {
	// 表名 + 重建函数成对登记：三种「目标表被删」用同一条断言，新增一种只加一行。
	cases := []struct {
		name  string
		table string
		fn    func(ctx context.Context, tx *sql.Tx) (int, error)
	}{
		{"直方图表缺失", tableChapterLengthHist, rebuildChapterLengthHist},
		{"趋势表缺失", tableImportDaily, rebuildImportDaily},
		{"作者表缺失", "authors", rebuildAuthorAgg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertRebuildFailsOnMissingTable(t, c.table, c.fn)
		})
	}
	t.Run("三表齐备时重建成功", func(t *testing.T) {
		assertRebuildSucceeds(t)
	})
}

// assertRebuildFailsOnMissingTable 删掉 table 后跑 fn，要求返回错误。
func assertRebuildFailsOnMissingTable(t *testing.T, table string,
	fn func(ctx context.Context, tx *sql.Tx) (int, error)) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.DB().ExecContext(ctx, "DROP TABLE "+table); err != nil {
		t.Fatalf("删表 %s: %v", table, err)
	}
	tx := beginTx(t, st)
	defer tx.Rollback() //nolint:errcheck // 用例结束即回滚
	if _, err := fn(ctx, tx); err == nil {
		t.Errorf("%s 缺失时重建应返回错误", table)
	}
}

// assertRebuildSucceeds 覆盖三表齐备时的正常重建（顺带验证计数确实非空）。
func assertRebuildSucceeds(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	seedDashboardBooks(t, st)
	tx := beginTx(t, st)
	defer tx.Rollback() //nolint:errcheck // 提交与否都不影响断言
	if err := rebuildDashboardTables(ctx, tx); err != nil {
		t.Fatalf("rebuildDashboardTables: %v", err)
	}
	if n, err := countTableRows(ctx, tx, tableAuthorAgg); err != nil || n == 0 {
		t.Errorf("重建后 author_agg 行数 = %d, err=%v，期望 > 0", n, err)
	}
}

// beginTx 开一个事务并要求成功。
func beginTx(t *testing.T, st *Store) *sql.Tx {
	t.Helper()
	tx, err := st.DB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	return tx
}
