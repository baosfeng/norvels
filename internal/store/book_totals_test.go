package store

import (
	"context"
	"testing"
	"time"
)

// 0026 的口径：书数 / 书目字数 / 已算质量分书数由 SQL 触发器维护，
// 读取侧优先读预计算、不可用时实时 SQL，两条路径必须恒等；
// 迁移只建结构（可重入），回填幂等，降级路径绝不返回错数字。

// TestMigrate0026TriggersAndState 空库迁移后：三个维护触发器在位，状态行 complete=1（无需回填）。
func TestMigrate0026TriggersAndState(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	var triggers int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'books_totals_%'").Scan(&triggers); err != nil {
		t.Fatalf("数触发器: %v", err)
	}
	requireEqual(t, "维护触发器个数", triggers, 3)
	var complete int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT complete FROM book_totals_state WHERE id = 1").Scan(&complete); err != nil {
		t.Fatalf("读状态行: %v", err)
	}
	requireEqual(t, "空库状态 complete", complete, 1)
}

// TestMigrate0026SkipsLegacySchema 老库（没有 books 表）上步骤不报错（只是什么都不做）。
func TestMigrate0026SkipsLegacySchema(t *testing.T) {
	ctx := context.Background()
	st := openBareOnly(t)
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // 只读探测
	if err := createBookTotalsStep(ctx, tx); err != nil {
		t.Fatalf("无 books 表时步骤应直接返回：%v", err)
	}
}

// TestBookTotalsMatchesLiveSQL 触发器维护值与实时 SQL 恒等（插入 / 改四列 / 删除）。
func TestBookTotalsMatchesLiveSQL(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTotalsBooks(t, ctx, st)
	assertBookTotalsMatchLive(t, ctx, st)

	for _, stmt := range []string{
		"UPDATE books SET word_count = word_count + 1000 WHERE id = 1",                                 // 只改字数
		"UPDATE books SET superseded_by = 2 WHERE id = 2",                                              // 变成被替换的旧版
		"UPDATE books SET superseded_by = 0 WHERE id = 2",                                              // 又变回当前版本
		"UPDATE books SET quality_chapters = 0, quality_at = '' WHERE id = 3",                          // 退化为「未计算」
		"UPDATE books SET quality_chapters = 12, quality_at = '2026-01-01T00:00:00.000Z' WHERE id = 4", // 补上质量分
		"DELETE FROM books WHERE id = 5",                                                               // 删书
	} {
		if _, err := st.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("执行 %q: %v", stmt, err)
		}
		assertBookTotalsMatchLive(t, ctx, st)
	}
}

// TestBookTotalsRebuildMatchesLive 全量回填与实时值恒等（回填是「运维怀疑漂了」时的兜底）。
func TestBookTotalsRebuildMatchesLive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTotalsBooks(t, ctx, st)
	// 模拟「触发器之前就有数据的老库」：把预计算值踩坏，回填必须自愈。
	if _, err := st.DB().ExecContext(ctx, "UPDATE book_totals SET n = 999"); err != nil {
		t.Fatalf("踩坏预计算值: %v", err)
	}
	rep, err := st.RebuildBookTotals(ctx)
	requireOK(t, "RebuildBookTotals", err)
	requireEqual(t, "回填书数", rep.BooksAll, 5)
	assertBookTotalsMatchLive(t, ctx, st)
}

// TestBookTotalsDegradesWithoutState 状态未完成时：读侧回退实时 SQL，stats / 健康报告仍然正确。
func TestBookTotalsDegradesWithoutState(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTotalsBooks(t, ctx, st)
	if _, ok := st.bookTotals(ctx); !ok {
		t.Fatal("空库回填完成后应可用")
	}
	if _, err := st.DB().ExecContext(ctx, "UPDATE book_totals_state SET complete = 0"); err != nil {
		t.Fatalf("标记未完成: %v", err)
	}
	if _, ok := st.bookTotals(ctx); ok {
		t.Fatal("状态未完成时不该返回预计算值")
	}
	live, err := st.liveBookTotals(ctx)
	requireOK(t, "liveBookTotals", err)

	stats, err := st.Stats(ctx)
	requireOK(t, "Stats 降级", err)
	requireEqual(t, "降级 stats.books", stats.Books, live.BooksCurrent)
	requireEqual(t, "降级 stats.words", stats.Words, live.WordSum)

	rep := st.HealthReport(ctx)
	if rep.Quality == nil {
		t.Fatalf("降级后质量分覆盖率缺失: %v", rep.Warnings)
	}
	requireEqual(t, "降级 quality.books", rep.Quality.Books, live.BooksAll)
	requireEqual(t, "降级 quality.computed", rep.Quality.Computed, live.QualityComputed)
	requireEqual(t, "降级不产生告警", len(rep.Warnings), 0)
}

// seedTotalsBooks 造 5 本覆盖各种判据的书，并把质量分 / 替换标记写成不同取值。
func seedTotalsBooks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for i := 1; i <= 5; i++ {
		b := syntheticBook("总量之书"+string(rune('0'+i)), "/path/to/txt/totals-"+string(rune('0'+i))+".txt",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"+string(rune('0'+i)),
			syntheticChapters(2))
		if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
			t.Fatalf("SaveBook: %v", err)
		}
	}
	for _, stmt := range []string{
		"UPDATE books SET quality_chapters = 3, quality_at = '2026-01-01T00:00:00.000Z' WHERE id <= 2",
		"UPDATE books SET quality_chapters = 0, quality_at = '' WHERE id = 3",
		"UPDATE books SET superseded_by = 1 WHERE id = 5",
	} {
		if _, err := st.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("执行 %q: %v", stmt, err)
		}
	}
}

// assertBookTotalsMatchLive 断言四个预计算值与实时 SQL 逐项相等。
func assertBookTotalsMatchLive(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	got, ok := st.bookTotals(ctx)
	if !ok {
		t.Fatal("预计算值不可用（状态应为 complete）")
	}
	want, err := st.liveBookTotals(ctx)
	requireOK(t, "liveBookTotals", err)
	requireEqual(t, "books_all", got.BooksAll, want.BooksAll)
	requireEqual(t, "books_current", got.BooksCurrent, want.BooksCurrent)
	requireEqual(t, "word_sum", got.WordSum, want.WordSum)
	requireEqual(t, "quality_computed", got.QualityComputed, want.QualityComputed)
}

// TestStartBookTotalsBackfillRunsAndDedupes 后台回填真的跑完，且同一时刻只跑一个。
func TestStartBookTotalsBackfillRunsAndDedupes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTotalsBooks(t, ctx, st)
	setTotalsComplete(t, ctx, st, 0)
	t.Setenv(EnvDisableBackground, "") // 打开后台任务（TestMain 默认关掉）

	// 已有一个回填在跑：直接返回，不再叠加第二个（两次并发扫 books 只会互相拖慢）。
	st.reconcile.totalsRunning.Store(true)
	st.StartBookTotalsBackfill()
	requireEqual(t, "去重：不该有第二个回填", totalsComplete(t, ctx, st), 0)
	st.reconcile.totalsRunning.Store(false)

	st.StartBookTotalsBackfill()
	waitTotalsComplete(t, ctx, st)
	waitTotalsIdle(t, st)
	assertBookTotalsMatchLive(t, ctx, st)
}

// TestStartBookTotalsBackfillReleasesCASOnFailure 回填失败只记一行 WARN，但去重标志必须释放——
// 否则一次失败会把这条回填永久挡住，存量库永远切不回预计算。
func TestStartBookTotalsBackfillReleasesCASOnFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setTotalsComplete(t, ctx, st, 0)
	t.Setenv(EnvDisableBackground, "")
	if err := st.DB().Close(); err != nil { // 主库不可用：回填必然在 BEGIN 就失败
		t.Fatalf("关闭主库: %v", err)
	}
	st.StartBookTotalsBackfill()
	waitTotalsIdle(t, st)
	requireEqual(t, "失败后去重标志释放", st.reconcile.totalsRunning.Load(), false)
}

// TestBookTotalsDegradesWhenTableMissing 预计算表被删（迁移被手改 / 文件损坏）：
// 读取侧按「不可用」降级到实时 SQL，stats 与健康报告的数字仍然正确。
func TestBookTotalsDegradesWhenTableMissing(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTotalsBooks(t, ctx, st)
	live, err := st.liveBookTotals(ctx)
	requireOK(t, "liveBookTotals", err)

	execOrFail(t, st, "DROP TABLE book_totals")
	if _, ok := st.bookTotals(ctx); ok {
		t.Fatal("表缺失时不该返回预计算值")
	}
	stats, err := st.Stats(ctx)
	requireOK(t, "Stats 降级", err)
	requireEqual(t, "表缺失时 stats.books", stats.Books, live.BooksCurrent)
	requireEqual(t, "表缺失时 stats.words", stats.Words, live.WordSum)
	rep := st.HealthReport(ctx)
	if rep.Quality == nil {
		t.Fatalf("表缺失时质量分覆盖率缺失: %v", rep.Warnings)
	}
	requireEqual(t, "表缺失时 quality.books", rep.Quality.Books, live.BooksAll)
}

// TestRebuildBookCountsTxSkipsMissingTables refresh-metrics 的兜底：0025 / 0026 的表不存在的
// 老库上直接跳过，不报错（否则运维命令会在「还没升级」的库上失败）。
func TestRebuildBookCountsTxSkipsMissingTables(t *testing.T) {
	ctx := context.Background()
	st := openBareOnly(t)
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // 只读探测
	requireOK(t, "表不存在时应跳过", rebuildBookCountsTx(ctx, tx))
}

// TestRebuildBookTotalsFailsWithoutState 状态行被删（0026 没应用）：回填后仍不可读，必须报错，
// 不能假装成功——否则后台日志会说「回填完成」而读取侧一直在扫全表。
func TestRebuildBookTotalsFailsWithoutState(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTotalsBooks(t, ctx, st)
	execOrFail(t, st, "DELETE FROM book_totals_state")
	_, err := st.RebuildBookTotals(ctx)
	if err == nil {
		t.Fatal("状态行缺失时回填应报错（而不是静默成功）")
	}
}

// totalsComplete 读回填状态行。
func totalsComplete(t *testing.T, ctx context.Context, st *Store) int {
	t.Helper()
	return int(scalarInt(t, ctx, st, "SELECT complete FROM book_totals_state WHERE id = 1"))
}

// setTotalsComplete 改写状态行（模拟「刚迁移完、历史值待补」的现场）。
func setTotalsComplete(t *testing.T, ctx context.Context, st *Store, complete int) {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE book_totals_state SET complete = ? WHERE id = 1", complete); err != nil {
		t.Fatalf("写状态行: %v", err)
	}
}

// waitTotalsComplete 等后台回填把状态推到 complete（3 秒等不到即失败）。
func waitTotalsComplete(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if totalsComplete(t, ctx, st) == 1 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内后台回填没有把状态置为 complete")
}

// waitTotalsIdle 等到没有回填在跑（消除在途 goroutine 与库清理的竞态）。
func waitTotalsIdle(t *testing.T, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !st.reconcile.totalsRunning.Load() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内库级总量回填没有停下来")
}
