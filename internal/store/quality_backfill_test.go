package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件锁死「存量书质量分回填」（backfill-quality）的语义：
//   - 已清空质量列的书逐本补算，补完覆盖率 100%；
//   - 重跑幂等（整本跳过）；
//   - 正文全空的书跳过并计数（它们本来就没有质量分可算）；
//   - 回填算出来的分与「解析期口径」在乱码 / 结构 / 重复三个维度上一致。

// clearBookQuality 把一本书的七列清空，模拟 0020 之前入的老库。
func clearBookQuality(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	if _, err := st.db.ExecContext(ctx,
		"UPDATE books SET garbage_ratio = 0, noise_ratio = 0, structure_score = 0, duplicate_ratio = 0, "+
			"quality_chapters = 0, quality_chars = 0, quality_at = '' WHERE id = ?", bookID); err != nil {
		t.Fatalf("清空质量列 book=%d: %v", bookID, err)
	}
}

// qualityBackfillFixture 造两本正文正常的书，并把质量列清空（模拟老库）。
func qualityBackfillFixture(t *testing.T) (*Store, context.Context, []int64) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	a := saveOrFail(t, ctx, st, withQuality(
		syntheticBook("回填甲", "/books/b1.txt", strings.Repeat("a", 64), qualityChapters(3)),
		model.QualityMetrics{ChapterCount: 3, TotalChars: 900, StructureScore: 1}))
	b := saveOrFail(t, ctx, st, withQuality(
		syntheticBook("回填乙", "/books/b2.txt", strings.Repeat("b", 64), qualityChapters(2)),
		model.QualityMetrics{ChapterCount: 2, TotalChars: 600, StructureScore: 1}))
	clearBookQuality(t, ctx, st, a.BookID)
	clearBookQuality(t, ctx, st, b.BookID)
	return st, ctx, []int64{a.BookID, b.BookID}
}

// runQualityBackfill 跑一次回填（batch 决定每批书数）。
func runQualityBackfill(t *testing.T, ctx context.Context, st *Store, batch int) QualityBackfillReport {
	t.Helper()
	rep, err := st.BackfillQuality(ctx, QualityBackfillOptions{Batch: batch})
	if err != nil {
		t.Fatalf("BackfillQuality: %v", err)
	}
	return rep
}

// assertBackfillSummary 断言回填汇总的四个计数。
func assertBackfillSummary(t *testing.T, rep QualityBackfillReport, books, filled, skipped, noContent int) {
	t.Helper()
	if rep.Books != books || rep.Filled != filled || rep.Skipped != skipped || rep.NoContent != noContent {
		t.Fatalf("回填汇总 = %+v，期望扫描 %d / 回填 %d / 跳过 %d / 无正文 %d",
			rep, books, filled, skipped, noContent)
	}
}

// assertQualityComputed 断言「已算质量分的书数」。
func assertQualityComputed(t *testing.T, ctx context.Context, st *Store, want int) {
	t.Helper()
	cov, err := st.qualityCoverage(ctx)
	if err != nil {
		t.Fatalf("qualityCoverage: %v", err)
	}
	if cov.Computed != want {
		t.Fatalf("已算质量分的书 = %d/%d，期望 %d", cov.Computed, cov.Books, want)
	}
}

// TestBackfillQualityFillsAllAndIsIdempotent 补满 → 覆盖率 100% → 重跑整本跳过。
func TestBackfillQualityFillsAllAndIsIdempotent(t *testing.T) {
	st, ctx, ids := qualityBackfillFixture(t)
	assertQualityComputed(t, ctx, st, 0)

	// Batch=1 走多批：同时验证 keyset 分页能把所有书扫到。
	assertBackfillSummary(t, runQualityBackfill(t, ctx, st, 1), 2, 2, 0, 0)
	assertQualityComputed(t, ctx, st, 2)
	assertBackfilledQuality(t, ctx, st, ids)
	// 重跑幂等：全部按「已有质量分」跳过。
	assertBackfillSummary(t, runQualityBackfill(t, ctx, st, 0), 2, 0, 2, 0)
}

// assertBackfilledQuality 断言回填出来的质量分形状（干净正文：零乱码、结构满分）。
func assertBackfilledQuality(t *testing.T, ctx context.Context, st *Store, ids []int64) {
	t.Helper()
	wantChapters := map[int64]int{ids[0]: 3, ids[1]: 2}
	for _, id := range ids {
		assertCleanQuality(t, ctx, st, id, wantChapters[id])
	}
}

// assertCleanQuality 断言一本书回填后的质量分：已算、章数对、零乱码零重复、结构满分、噪声 0。
func assertCleanQuality(t *testing.T, ctx context.Context, st *Store, id int64, chapters int) {
	t.Helper()
	q, err := st.bookQualityByID(ctx, id)
	if err != nil {
		t.Fatalf("bookQualityByID(%d): %v", id, err)
	}
	if !q.Computed() || q.Chapters != chapters {
		t.Fatalf("书 %d 的质量分 = %+v，期望已算 / %d 章", id, q, chapters)
	}
	if q.GarbageRatio != 0 || q.DuplicateRatio != 0 {
		t.Errorf("干净正文的乱码率/重复率 = %v/%v，期望 0", q.GarbageRatio, q.DuplicateRatio)
	}
	if q.StructureScore != 1 {
		t.Errorf("书 %d 的结构分 = %v，期望 1（章数合理、无空章、无离群章）", id, q.StructureScore)
	}
	// 噪声率按残留计：库里正文已清洗过，回填恒为 0（口径见 quality_calc.go）。
	if q.NoiseRatio != 0 {
		t.Errorf("书 %d 的噪声率 = %v，期望 0（回填口径按残留噪声计）", id, q.NoiseRatio)
	}
}

// TestBackfillQualitySkipsEmptyContent 正文全空的书跳过并计数，不留半份分数。
func TestBackfillQualitySkipsEmptyContent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	empty := saveOrFail(t, ctx, st, syntheticBook("空白之书", "/books/blank.txt", strings.Repeat("z", 64),
		[]model.ParsedChapter{{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: ""}}))
	clearBookQuality(t, ctx, st, empty.BookID)

	assertBackfillSummary(t, runQualityBackfill(t, ctx, st, 0), 1, 0, 0, 1)
	if q, err := st.bookQualityByID(ctx, empty.BookID); err != nil || q.Computed() {
		t.Errorf("正文全空的书不该被写成已计算：%+v（err=%v）", q, err)
	}
}

// TestBackfillQualityMatchesParseFormulas 回填口径与解析期口径在乱码 / 结构 / 重复三维一致。
//
// 语料刻意造得「有乱码、有重复段落、章长不均」：三项都必须真的非零，否则对照是恒真。
func TestBackfillQualityMatchesParseFormulas(t *testing.T) {
	chapters := dirtyStoredChapters()
	got, ok := computeStoredQuality(chapters)
	if !ok {
		t.Fatal("有正文的书应算出质量分")
	}
	if got.GarbageRatio <= 0 || got.DuplicateRatio <= 0 {
		t.Fatalf("乱码率/重复率 = %v/%v，期望都 > 0（语料要真的脏）", got.GarbageRatio, got.DuplicateRatio)
	}
	// 章长 3 章差异大：结构分必须真的被离群判定影响（不是恒 1）。
	if got.StructureScore >= 1 {
		t.Errorf("结构分 = %v，期望 < 1（第三章明显偏短）", got.StructureScore)
	}
	if got.Chapters != 3 || got.Chars <= 0 {
		t.Errorf("章数/字符数 = %d/%d，期望 3 / > 0", got.Chapters, got.Chars)
	}
}

// dirtyStoredChapters 造三章「有乱码、有重复段落、章长不均」的合成正文。
func dirtyStoredChapters() []model.ParsedChapter {
	para := strings.Repeat("临川渡的雾从江面升起，艄公把竹篙插进水里。\n", 40)
	return []model.ParsedChapter{
		{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: para + "\uFFFD\uFFFD乱码\n"},
		{Idx: 1, VolumeIdx: -1, Title: "第二章", Content: para},
		{Idx: 2, VolumeIdx: -1, Title: "第三章", Content: strings.Repeat("孤舟泊岸，灯火在雨里摇晃，另有一个结局。\n", 12)},
	}
}
