package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// 本文件锁死「存量书指纹回填」（norvels backfill-fingerprints 的 store 侧）：
// 3 本书清空指纹 → 回填补齐 → 再跑一遍整本跳过（幂等）；坏章节不拦整批；ctx 可中断。

// backfillBook 保存一本内容唯一的合成书（每本书、每章都不同，避免互相命中）。
func backfillBook(t *testing.T, ctx context.Context, st *Store, n int) int64 {
	t.Helper()
	chapters := syntheticChapters(4)
	for i := range chapters {
		chapters[i].Content = fmt.Sprintf("第%d本书第%d章：梅宁城的风从城楼吹过，雪线压得很低。", n, i)
	}
	book := syntheticBook(fmt.Sprintf("回填之书%d", n),
		fmt.Sprintf("/books/backfill-%d.txt", n), strings.Repeat(fmt.Sprintf("%c", 'a'+n), 64), chapters)
	return saveOrFail(t, ctx, st, book).BookID
}

// clearFingerprints 清空全库指纹（模拟「0019 刚建表、历史书还没回填」的现场）。
func clearFingerprints(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if _, err := st.cdb.ExecContext(ctx, "DELETE FROM chapter_fingerprints"); err != nil {
		t.Fatalf("清空指纹: %v", err)
	}
}

// backfillFixture 建 3 本内容互不相同的合成书（SaveBook 已写好指纹），返回库与书 id。
func backfillFixture(t *testing.T) (*Store, context.Context, []int64) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	ids := []int64{backfillBook(t, ctx, st, 1), backfillBook(t, ctx, st, 2), backfillBook(t, ctx, st, 3)}
	return st, ctx, ids
}

// TestBackfillFingerprintsFills 清空指纹后回填：3 本书 12 条指纹全部补齐。
func TestBackfillFingerprintsFills(t *testing.T) {
	st, ctx, ids := backfillFixture(t)
	clearFingerprints(t, ctx, st)

	rep, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{})
	if err != nil {
		t.Fatalf("BackfillFingerprints: %v", err)
	}
	if rep.Books != 3 || rep.Filled != 3 || rep.Skipped != 0 || rep.NoContent != 0 {
		t.Fatalf("报告 = %+v，期望 3 本全部回填", rep)
	}
	if rep.Fingerprints != 12 || rep.BadChapters != 0 {
		t.Fatalf("写入指纹 %d 行 / 坏章节 %d 章，期望 12 行 / 0 章", rep.Fingerprints, rep.BadChapters)
	}
	for _, id := range ids {
		if got := fingerprintRows(t, ctx, st, id); got != 4 {
			t.Errorf("书 %d 指纹行 = %d，期望 4", id, got)
		}
	}
}

// TestBackfillFingerprintsSkipsExisting 已有指纹的书整本跳过：重跑幂等、不重复插入。
func TestBackfillFingerprintsSkipsExisting(t *testing.T) {
	st, ctx, ids := backfillFixture(t)

	rep, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{})
	if err != nil {
		t.Fatalf("BackfillFingerprints: %v", err)
	}
	if rep.Filled != 0 || rep.Skipped != 3 || rep.Fingerprints != 0 {
		t.Fatalf("报告 = %+v，期望 3 本全部跳过（幂等）", rep)
	}
	if got := fingerprintRows(t, ctx, st, ids[0]); got != 4 {
		t.Errorf("书 %d 指纹行 = %d，期望仍是 4（没被重复插入）", ids[0], got)
	}
}

// TestBackfillFingerprintsReportsProgress 进度回调按批触发，done 单调递增到总书数。
func TestBackfillFingerprintsReportsProgress(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for i := 1; i <= 3; i++ {
		backfillBook(t, ctx, st, i)
	}
	clearFingerprints(t, ctx, st)

	var done []int
	rep, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{
		Batch: 1,
		Progress: func(d, total int, _ time.Duration) {
			if total != 3 {
				t.Errorf("进度 total = %d，期望 3", total)
			}
			done = append(done, d)
		},
	})
	if err != nil {
		t.Fatalf("BackfillFingerprints: %v", err)
	}
	if len(done) != 3 || done[0] != 1 || done[2] != rep.Books {
		t.Errorf("进度序列 = %v，期望每批一次并递增到 %d", done, rep.Books)
	}
}

// TestBackfillFingerprintsBadChapter 坏章节只计数不中断：其余章节照常回填。
func TestBackfillFingerprintsBadChapter(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := backfillBook(t, ctx, st, 1)
	clearFingerprints(t, ctx, st)
	// 把第一章的区间改到块外：chapterAt 必然报错（块本身没坏）。
	if _, err := st.db.ExecContext(ctx,
		"UPDATE chapters SET block_len = block_len + 1000000 WHERE book_id = ? AND idx = 0", id); err != nil {
		t.Fatalf("篡改章节区间: %v", err)
	}

	rep, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{})
	if err != nil {
		t.Fatalf("BackfillFingerprints: %v", err)
	}
	if rep.BadChapters != 1 {
		t.Errorf("坏章节 = %d，期望 1", rep.BadChapters)
	}
	if got := fingerprintRows(t, ctx, st, id); got != 3 {
		t.Errorf("书 %d 指纹行 = %d，期望 3（坏章跳过、其余照写）", id, got)
	}
}

// TestBackfillFingerprintsCanceled ctx 取消时报错返回，已写入的指纹保留（重跑即续）。
func TestBackfillFingerprintsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	st := newTestStore(t)
	for i := 1; i <= 3; i++ {
		backfillBook(t, context.Background(), st, i)
	}
	clearFingerprints(t, context.Background(), st)
	cancel()

	if _, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{}); err == nil {
		t.Fatal("ctx 取消后回填应返回错误（可续跑），实际 nil")
	}
}

// TestBackfillFingerprintsNoTable 迁移没跑时直接说「先 migrate」，而不是假装回填成功。
func TestBackfillFingerprintsNoTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.cdb.ExecContext(ctx, "DROP TABLE chapter_fingerprints"); err != nil {
		t.Fatalf("删指纹表: %v", err)
	}
	_, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{})
	if err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("缺表时应提示先 migrate，实际 err=%v", err)
	}
}

// TestHealthReportFingerprintCoverage 健康报告暴露指纹覆盖率：有缺口时给命令级提示。
func TestHealthReportFingerprintCoverage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := backfillBook(t, ctx, st, 1)
	backfillBook(t, ctx, st, 2)

	rep := st.HealthReport(ctx)
	if rep.Fingerprints == nil || rep.Fingerprints.Missing != 0 || rep.Fingerprints.Books != 2 {
		t.Fatalf("覆盖率 = %+v，期望 2 本书全有指纹", rep.Fingerprints)
	}

	// 只清掉一本书的指纹：缺口 1，并指向回填命令。
	// 走 Store 的删指纹路径（指纹行与覆盖行同事务），裸 SQL 会制造覆盖表漂移。
	st.purgeBookFingerprints(ctx, first)
	rep = st.HealthReport(ctx)
	if rep.Fingerprints == nil || rep.Fingerprints.Missing != 1 || rep.Fingerprints.Fingerprinted != 1 {
		t.Fatalf("覆盖率 = %+v，期望缺口 1", rep.Fingerprints)
	}
	if !strings.Contains(rep.Fingerprints.Hint, "backfill-fingerprints") {
		t.Errorf("缺口提示 = %q，期望指向 backfill-fingerprints", rep.Fingerprints.Hint)
	}
}
