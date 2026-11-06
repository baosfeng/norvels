package store

import (
	"context"
	"testing"
)

// 本文件锁死覆盖表 fingerprint_books 的**唯一不变量**：
//
//	一本书在覆盖表里  ⟺  它在正文库里至少有一条指纹行
//
// 增量维护（写指纹路径 replaceFingerprints / purgeBookFingerprints）与全量重算
// （RepairFingerprintCoverage）必须给出同一个集合——这与 0022 的预计算表是同一条口径
// （见 TestImportTrendCacheMatchesLive）。覆盖表是用来替掉那条 22 秒 COUNT(DISTINCT) 的，
// 一旦它和指纹行漂移，健康报告就会给出一个「看着正常但错」的覆盖率。

// TestFingerprintCoverageMatchesFullRecount 增量维护结果 == 全量重算结果。
func TestFingerprintCoverageMatchesFullRecount(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	idA := backfillBook(t, ctx, st, 1)
	idB := backfillBook(t, ctx, st, 2)

	// 1) 正常写路径：两本书都有指纹，覆盖表也两行。
	assertCoverageMatchesFull(t, ctx, st)

	// 2) 一本书的指纹被整本重写为空（章节全空的情形）：覆盖行必须跟着消失。
	if err := st.replaceFingerprints(ctx, idA, nil); err != nil {
		t.Fatalf("replaceFingerprints 空指纹: %v", err)
	}
	assertCoverageMatchesFull(t, ctx, st)

	// 3) 删书路径（purgeBookFingerprints）：指纹行与覆盖行同事务清掉。
	st.purgeBookFingerprints(ctx, idB)
	assertCoverageMatchesFull(t, ctx, st)

	// 4) 回填路径把指纹写回来：覆盖表跟着补上（仍是 1 本）。
	if _, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{}); err != nil {
		t.Fatalf("BackfillFingerprints: %v", err)
	}
	assertCoverageMatchesFull(t, ctx, st)
}

// TestRepairFingerprintCoverageFixesDrift 绕过 Store 的裸 SQL 弄脏覆盖表后，重算能修回来（可重入）。
//
// 漂移只可能来自裸 SQL（手工删指纹 / 旧实现残留），两条方向都要修：
// 有指纹没覆盖行 → 补；有覆盖行没指纹 → 清。
func TestRepairFingerprintCoverageFixesDrift(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	idA := backfillBook(t, ctx, st, 1)
	backfillBook(t, ctx, st, 2)

	// 方向一：裸删除一本书的指纹行 → 覆盖行变成陈旧（有覆盖、没指纹）。
	if _, err := st.cdb.ExecContext(ctx,
		"DELETE FROM chapter_fingerprints WHERE book_id = ?", idA); err != nil {
		t.Fatalf("裸删指纹: %v", err)
	}
	assertCoverageDrifted(t, ctx, st)

	// 方向二：直接插一行「有指纹、没覆盖行」的书（模拟 0023 之前写入的指纹）。
	if _, err := st.cdb.ExecContext(ctx,
		"INSERT OR REPLACE INTO chapter_fingerprints(hash, book_id, bytes) VALUES (?, ?, ?)",
		int64(424242), int64(999), int64(64)); err != nil {
		t.Fatalf("插入孤立指纹: %v", err)
	}

	rep, err := st.RepairFingerprintCoverage(ctx)
	if err != nil {
		t.Fatalf("RepairFingerprintCoverage: %v", err)
	}
	if rep.Pruned != 1 {
		t.Errorf("清理陈旧覆盖 = %d，期望 1", rep.Pruned)
	}
	if rep.Added != 1 {
		t.Errorf("补进覆盖表 = %d，期望 1", rep.Added)
	}
	assertCoverageMatchesFull(t, ctx, st)

	// 再跑一次：幂等，什么都不用修。
	again, err := st.RepairFingerprintCoverage(ctx)
	if err != nil {
		t.Fatalf("第二次 RepairFingerprintCoverage: %v", err)
	}
	if again.Added != 0 || again.Pruned != 0 {
		t.Errorf("重算不幂等：Added=%d Pruned=%d", again.Added, again.Pruned)
	}
}

// TestFingerprintCoverageEmptyBookNotCovered 章节正文全空的书写不出指纹，不能算「已覆盖」。
//
// 这类书本来就判不出重复，留在缺口里是对的；把它算成已覆盖会让 Hint 消失，
// 运维再也看不到「这本书判重比不到」。
func TestFingerprintCoverageEmptyBookNotCovered(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := saveOrFail(t, ctx, st, syntheticBook("空白之书", "/books/blank.txt",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		syntheticChapters(0))).BookID
	if err := st.replaceFingerprints(ctx, id, nil); err != nil {
		t.Fatalf("replaceFingerprints 空指纹: %v", err)
	}
	cover, err := st.fingerprintCoverageCount(ctx)
	if err != nil {
		t.Fatalf("fingerprintCoverageCount: %v", err)
	}
	if cover != 0 {
		t.Errorf("覆盖表 = %d 行，期望 0（这本书写不出指纹）", cover)
	}
}

// assertCoverageMatchesFull 断言覆盖表行数 == 全表重算出的「有指纹的书数」。
func assertCoverageMatchesFull(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	cover, err := st.fingerprintCoverageCount(ctx)
	if err != nil {
		t.Fatalf("fingerprintCoverageCount: %v", err)
	}
	full, err := st.fingerprintedBookCountLegacy(ctx)
	if err != nil {
		t.Fatalf("fingerprintedBookCountLegacy: %v", err)
	}
	if cover != full {
		t.Fatalf("覆盖表 %d 本，全量重算 %d 本（增量维护与全量口径漂移了）", cover, full)
	}
}

// assertCoverageDrifted 断言覆盖表与指纹行确实不一致（前提校验：漂移得能被发现）。
func assertCoverageDrifted(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	cover, err := st.fingerprintCoverageCount(ctx)
	if err != nil {
		t.Fatalf("fingerprintCoverageCount: %v", err)
	}
	full, err := st.fingerprintedBookCountLegacy(ctx)
	if err != nil {
		t.Fatalf("fingerprintedBookCountLegacy: %v", err)
	}
	if cover == full {
		t.Fatalf("裸删指纹后覆盖表 %d 仍等于全量 %d，用例前提不成立", cover, full)
	}
}
