package store

import (
	"context"
	"testing"
	"time"
)

// 本文件锁死覆盖表**后台回填启动路径**（StartFingerprintBackfill）的契约：
//
//   - 启动即返回，回填自己跑完、把断点状态置 complete（健康报告据此停止标 stale）；
//   - 同一时刻只允许一个回填在跑（atomic CAS），第二次调用不叠加 goroutine；
//   - 回填失败只记一行 WARN，去重标志必须释放，否则后续启动永远补不上。
//
// 增量维护与全量重算的口径恒等由 fingerprint_books_test.go 守着，这里只管启动路径。

func TestStartFingerprintBackfillRepairsCoverage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	dirty := backfillBook(t, ctx, st, 1)
	backfillBook(t, ctx, st, 2)
	t.Setenv(EnvDisableBackground, "") // 打开后台任务（TestMain 默认关掉）

	// 生产升级后的真实现场：状态行未完成 + 覆盖表少一行（历史指纹还没搬完）。
	requireOK(t, "setCoverageState", st.setCoverageState(ctx, false, 0))
	execCdbOrFail(t, st, "DELETE FROM "+fingerprintBooksTable+" WHERE book_id = ?", dirty)
	assertCoverageDrifted(t, ctx, st)

	// 已有一个回填在跑：直接返回，不再叠加第二个（两次并发只会互相拖慢）。
	st.reconcile.coverageRunning.Store(true)
	st.StartFingerprintBackfill(ctx)
	assertCoverageDrifted(t, ctx, st)
	st.reconcile.coverageRunning.Store(false)

	st.StartFingerprintBackfill(ctx)
	waitCoverageBackfillIdle(t, st)
	assertCoverageMatchesFull(t, ctx, st)
	complete, _, err := st.coverageState(ctx)
	requireOK(t, "coverageState", err)
	requireEqual(t, "回填完成后状态 complete", complete, true)
}

// TestStartFingerprintBackfillReleasesCASOnFailure 正文库不可用时回填失败，
// 去重标志仍要释放（中断只记一行 WARN，下次启动接着补）。
func TestStartFingerprintBackfillReleasesCASOnFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	t.Setenv(EnvDisableBackground, "")
	requireOK(t, "关闭正文库", st.cdb.Close())
	st.StartFingerprintBackfill(ctx)
	waitCoverageBackfillIdle(t, st)
	requireEqual(t, "失败后去重标志释放", st.CoverageBackfillRunning(), false)
}

// execCdbOrFail 在正文库上执行一条测试用 SQL。
func execCdbOrFail(t *testing.T, st *Store, sqlText string, args ...any) {
	t.Helper()
	if _, err := st.cdb.ExecContext(context.Background(), sqlText, args...); err != nil {
		t.Fatalf("正文库执行 SQL 失败: %v\n%s", err, sqlText)
	}
}

// waitCoverageBackfillIdle 等到没有回填在跑（消除在途 goroutine 与库清理的竞态）。
func waitCoverageBackfillIdle(t *testing.T, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !st.CoverageBackfillRunning() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内覆盖表回填没有停下来")
}
