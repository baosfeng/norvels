package store

import (
	"context"
	"testing"
	"time"
)

// 本文件锁死 0025 书级健康计数的**回填侧**（触发器维护与读取口径由 migrate_0025_test.go 守着）：
//
//   - RebuildBookHealthCounts 的结果必须与实时 SQL 逐项相等，且可重入（连跑两次不变）；
//   - 后台回填 StartBookHealthCountsBackfill 真的把状态推到 complete，同一时刻只跑一个，
//     失败也必须释放去重标志（否则后续回填被永久挡住，存量库永远切不回预计算）；
//   - 预计算表缺失 / 有脏值时读取侧降级到实时 SQL，绝不 500、绝不返回错数字。

func TestRebuildBookHealthCountsMatchesLive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	setBookHealthComplete(t, ctx, st, 0) // 生产现场：0025 刚迁移完，历史值待补

	rep, err := st.RebuildBookHealthCounts(ctx)
	requireOK(t, "RebuildBookHealthCounts", err)
	if rep.Elapsed <= 0 {
		t.Errorf("Elapsed = %v，期望正数", rep.Elapsed)
	}
	assertBookHealthCountsMatchLive(t, ctx, st) // 三条口径一份：预计算 == 实时 SQL
	requireEqual(t, "回填后状态 complete", bookHealthComplete(t, ctx, st), 1)

	again, err := st.RebuildBookHealthCounts(ctx)
	requireOK(t, "第二次 RebuildBookHealthCounts", err)
	requireEqual(t, "可重入：缺简介", again.MissingIntro, rep.MissingIntro)
	requireEqual(t, "可重入：缺作者", again.MissingAuthor, rep.MissingAuthor)
	requireEqual(t, "可重入：超薄书", again.ThinBooks, rep.ThinBooks)
	requireEqual(t, "可重入：分桶数", len(again.Buckets), len(rep.Buckets))
	for i := range rep.Buckets {
		requireEqual(t, "可重入：分桶", again.Buckets[i], rep.Buckets[i])
	}
}

// TestStartBookHealthCountsBackfillRunsAndDedupes 后台回填真的跑完，且同一时刻只跑一个。
func TestStartBookHealthCountsBackfillRunsAndDedupes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	setBookHealthComplete(t, ctx, st, 0)
	t.Setenv(EnvDisableBackground, "") // 打开后台任务（TestMain 默认关掉）

	// 已有一个回填在跑：直接返回，不再叠加第二个（两次并发扫 books 只会互相拖慢）。
	st.reconcile.healthCountsRunning.Store(true)
	st.StartBookHealthCountsBackfill()
	requireEqual(t, "去重：不该有第二个回填", bookHealthComplete(t, ctx, st), 0)
	st.reconcile.healthCountsRunning.Store(false)

	st.StartBookHealthCountsBackfill()
	waitBookHealthComplete(t, ctx, st)
	waitHealthCountsIdle(t, st)
	assertBookHealthCountsMatchLive(t, ctx, st)
}

// TestStartBookHealthCountsBackfillReleasesCASOnFailure 回填失败只记一行 WARN，
// 但去重标志必须释放——否则一次失败会把这条回填永久挡住。
func TestStartBookHealthCountsBackfillReleasesCASOnFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setBookHealthComplete(t, ctx, st, 0)
	t.Setenv(EnvDisableBackground, "")
	if err := st.DB().Close(); err != nil { // 主库不可用：回填必然在 BEGIN 就失败
		t.Fatalf("关闭主库: %v", err)
	}
	st.StartBookHealthCountsBackfill()
	waitHealthCountsIdle(t, st)
	requireEqual(t, "失败后去重标志释放", st.reconcile.healthCountsRunning.Load(), false)
}

// TestBookHealthCountsDegradesWhenPrecomputeBroken 预计算表缺失时降级到实时 SQL，数字仍对。
func TestBookHealthCountsDegradesWhenPrecomputeBroken(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)

	// 分桶表缺失（迁移被手改 / 文件损坏）：读取侧按「不可用」处理。
	execOrFail(t, st, "DROP TABLE book_chapter_buckets")
	if _, ok := st.bookHealthCounts(ctx); ok {
		t.Fatal("分桶表缺失时不该返回预计算值")
	}
	assertDegradedReportOK(t, ctx, st)

	// 计数表也缺失：连标量都读不到，同样降级而不是报错。
	execOrFail(t, st, "DROP TABLE book_health_counts")
	if _, ok := st.bookHealthCounts(ctx); ok {
		t.Fatal("计数表缺失时不该返回预计算值")
	}
	assertDegradedReportOK(t, ctx, st)
}

// TestBookHealthCountsDegradesOnCorruptBucketRow 分桶行里出现脏值（整数列塞文本）：
// 读取报错 → 按「不可用」降级，健康报告照常给出三项指标。
func TestBookHealthCountsDegradesOnCorruptBucketRow(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	execOrFail(t, st, "INSERT INTO book_chapter_buckets(bucket, n) VALUES (99, '脏数据')")
	if _, ok := st.bookHealthCounts(ctx); ok {
		t.Fatal("分桶行非法时不该返回预计算值")
	}
	assertDegradedReportOK(t, ctx, st)
}

// TestMissingIntroWherePicksIndexedColumn 探测 intro_blank：看得见它才走索引谓词。
//
// 两条谓词语义相同、只差快慢，所以「探测失败」不会给出错数字，只会慢——但慢的正是
// 0025 要消灭的那 4.8 秒全表扫，因此「探测到列就必须用索引谓词」这一步要锁住。
// 0025 的 intro_blank 是 VIRTUAL 生成列（pragma_table_info 看不到它，见该迁移的注释），
// 这里用一个真实列构造「看得见」的现场，避免用例依赖那个已知的可见性限制。
func TestMissingIntroWherePicksIndexedColumn(t *testing.T) {
	ctx := context.Background()
	// 0025 之前建的库没有这一列：写死新谓词会直接报 no such column，只能退回 TRIM。
	old := newTestStoreTo(t, 24)
	requireEqual(t, "无该列时退回 TRIM 谓词", old.missingIntroWhere(ctx), whereMissingIntro)
	execOrFail(t, old, "ALTER TABLE books ADD COLUMN intro_blank INTEGER")
	requireEqual(t, "有该列时走索引谓词", old.missingIntroWhere(ctx), "intro_blank = 1")
}

// TestColumnExistsSeesVirtualGeneratedColumn 锁住 0025 的虚拟生成列能被探测到。
//
// 回归点：columnExists 曾用 pragma_table_info，它不列出 VIRTUAL 生成列，于是真实库里
// missingIntroWhere 永远退回 TRIM(intro)=” 全表扫（生产 18 万本实测 4.8 秒）。
// 这条用例在迁移到 25 的库上断言「看不见就该红」，改错探测方式会立刻失败。
func TestColumnExistsSeesVirtualGeneratedColumn(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	has, err := st.columnExists(ctx, "books", "intro_blank")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !has {
		t.Fatal("intro_blank 是 0025 的 VIRTUAL 生成列，探测不到说明退回了 pragma_table_info")
	}
	requireEqual(t, "迁移后的库必须走索引谓词", st.missingIntroWhere(ctx), "intro_blank = 1")
}

// assertDegradedReportOK 断言降级路径给出完整报告：三项指标有值、没有告警。
func assertDegradedReportOK(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	rep := st.HealthReport(ctx)
	if len(rep.Warnings) != 0 {
		t.Fatalf("降级路径不该有告警: %v", rep.Warnings)
	}
	if rep.MissingIntro == nil || rep.MissingAuthor == nil || rep.ThinBooks == nil {
		t.Fatalf("降级后指标缺失: missingIntro=%v missingAuthor=%v thin=%v",
			rep.MissingIntro, rep.MissingAuthor, rep.ThinBooks)
	}
}

// bookHealthComplete 读 0025 状态行的 complete 标记。
func bookHealthComplete(t *testing.T, ctx context.Context, st *Store) int {
	t.Helper()
	return int(scalarInt(t, ctx, st, "SELECT complete FROM book_health_state WHERE id = 1"))
}

// setBookHealthComplete 改写状态行（模拟「刚迁移完、历史值待补」的现场）。
func setBookHealthComplete(t *testing.T, ctx context.Context, st *Store, complete int) {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE book_health_state SET complete = ? WHERE id = 1", complete); err != nil {
		t.Fatalf("写状态行: %v", err)
	}
}

// waitBookHealthComplete 等后台回填把状态推到 complete（3 秒等不到即失败）。
func waitBookHealthComplete(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if bookHealthComplete(t, ctx, st) == 1 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内后台回填没有把状态置为 complete")
}

// waitHealthCountsIdle 等到没有回填在跑（消除在途 goroutine 与库清理的竞态）。
func waitHealthCountsIdle(t *testing.T, st *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !st.reconcile.healthCountsRunning.Load() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("3 秒内健康计数回填没有停下来")
}
