package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"norvels/internal/model"
)

// 本文件锁死「质量分落库」的口径（设计 §5 / 迁移 0020）：
//   - SaveBook 把解析期质量分写进 books 的七列；
//   - 没有质量分的书（EPUB / 合集拆出的子书）写「未计算」而不是 0 分；
//   - 覆盖导入会重写质量分；
//   - 覆盖率统计与 BookQuality.Computed() 同口径。

// qualityChapters 造 n 章合成正文：每章内容互不相同、章长约 1600 字。
//
// 这两个条件是有意的：章长落在「合理区间」里（回填出来的结构分才是 1），段落两两不同
// （回填出来的内部重复率才是 0）。语料若全章同文，回填质量分天然是「重复率 0.67、
// 结构分 0.17」——那不是 bug，是语料造的。
func qualityChapters(n int) []model.ParsedChapter {
	out := make([]model.ParsedChapter, 0, n)
	for i := 0; i < n; i++ {
		var b strings.Builder
		for p := 0; p < 40; p++ {
			fmt.Fprintf(&b, "第%d章第%d段：临川渡的雾从江面升起，艄公把竹篙插进水里，岸边的灯一盏一盏亮起来。\n", i, p)
		}
		out = append(out, model.ParsedChapter{
			Idx: i, VolumeIdx: -1, Title: "第" + string(rune('1'+i)) + "章", Content: b.String(),
		})
	}
	return out
}

// withQuality 给一本书挂上解析期质量分（章数必须与章节数一致）。
func withQuality(b *model.ParsedBook, q model.QualityMetrics) *model.ParsedBook {
	b.Quality = q
	return b
}

// TestSaveBookWritesQuality SaveBook 把质量分写进七列，读回与写入一致。
func TestSaveBookWritesQuality(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := qualityChapters(3)
	q := model.QualityMetrics{
		ChapterCount: 3, TotalChars: 900, GarbageRatio: 0.0125, NoiseRatio: 0.25,
		StructureScore: 0.9, DuplicateRatio: 0.03,
	}
	book := withQuality(syntheticBook("质量之书", "/books/q.txt", strings.Repeat("q", 64), chapters), q)
	res := saveOrFail(t, ctx, st, book)

	got, err := st.bookQualityByID(ctx, res.BookID)
	if err != nil {
		t.Fatalf("bookQualityByID: %v", err)
	}
	if !got.Computed() {
		t.Fatalf("刚写入的质量分应为已计算：%+v", got)
	}
	if got.Chapters != 3 || got.Chars != 900 {
		t.Errorf("章数/字符数 = %d/%d，期望 3/900", got.Chapters, got.Chars)
	}
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"乱码率", got.GarbageRatio, q.GarbageRatio},
		{"噪声率", got.NoiseRatio, q.NoiseRatio},
		{"结构分", got.StructureScore, q.StructureScore},
		{"重复率", got.DuplicateRatio, q.DuplicateRatio},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestSaveBookMarksUncomputedQuality 零值质量分写「未计算」：七列零值 + quality_at 空串。
func TestSaveBookMarksUncomputedQuality(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := syntheticBook("EPUB 之书", "/books/e.epub", strings.Repeat("e", 64), qualityChapters(2))
	res := saveOrFail(t, ctx, st, book)

	got, err := st.bookQualityByID(ctx, res.BookID)
	if err != nil {
		t.Fatalf("bookQualityByID: %v", err)
	}
	if got.Computed() {
		t.Fatalf("零值质量分不该算「已计算」：%+v", got)
	}
	if got.Chapters != 0 || got.Chars != 0 || got.GarbageRatio != 0 {
		t.Errorf("未计算的书七列应全零：%+v", got)
	}
	var at string
	if err := st.db.QueryRowContext(ctx,
		"SELECT quality_at FROM books WHERE id = ?", res.BookID).Scan(&at); err != nil {
		t.Fatalf("读 quality_at: %v", err)
	}
	if at != "" {
		t.Errorf("quality_at = %q，期望空串（未计算）", at)
	}
}

// TestSaveBookOverwriteRewritesQuality 覆盖导入必须重写质量分（旧分数不能留在新版本上）。
func TestSaveBookOverwriteRewritesQuality(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	path := "/books/over.txt"
	sha := strings.Repeat("o", 64)
	old := withQuality(syntheticBook("覆盖之书", path, sha, qualityChapters(2)),
		model.QualityMetrics{ChapterCount: 2, TotalChars: 600, NoiseRatio: 0.4, StructureScore: 0.5})
	res := saveOrFail(t, ctx, st, old)

	fresh := withQuality(syntheticBook("覆盖之书", path, sha, qualityChapters(4)),
		model.QualityMetrics{ChapterCount: 4, TotalChars: 1200, NoiseRatio: 0.1, StructureScore: 0.95})
	if _, err := st.SaveBook(ctx, fresh, SaveOptions{Force: true}); err != nil {
		t.Fatalf("覆盖保存: %v", err)
	}
	got, err := st.bookQualityByID(ctx, res.BookID)
	if err != nil {
		t.Fatalf("bookQualityByID: %v", err)
	}
	if got.Chapters != 4 || got.Chars != 1200 || got.NoiseRatio != 0.1 {
		t.Errorf("覆盖后质量分 = %+v，期望章数 4 / 字符 1200 / 噪声 0.1", got)
	}
}

// TestQualityCoverage 覆盖率统计：已算 / 未算两档，与 Computed() 同口径。
func TestQualityCoverage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	computed := withQuality(syntheticBook("已算之书", "/books/c1.txt", strings.Repeat("c", 64), qualityChapters(2)),
		model.QualityMetrics{ChapterCount: 2, TotalChars: 600, StructureScore: 1})
	saveOrFail(t, ctx, st, computed)
	saveOrFail(t, ctx, st, syntheticBook("未算之书", "/books/c2.txt", strings.Repeat("d", 64), qualityChapters(2)))

	cov, err := st.qualityCoverage(ctx)
	if err != nil {
		t.Fatalf("qualityCoverage: %v", err)
	}
	if cov.Books != 2 || cov.Computed != 1 || cov.Missing != 1 {
		t.Errorf("覆盖率 = %+v，期望 2 本 / 已算 1 / 缺 1", cov)
	}
}

// TestQualityFromMetrics 解析期质量分 → 落库形状的翻译口径。
func TestQualityFromMetrics(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	if got := qualityFromMetrics(model.QualityMetrics{}, at); got.Computed() {
		t.Errorf("零值质量分应翻译成「未计算」：%+v", got)
	}
	m := model.QualityMetrics{ChapterCount: 5, TotalChars: 1500, GarbageRatio: 0.02, NoiseRatio: 0.3, StructureScore: 0.8}
	got := qualityFromMetrics(m, at)
	if !got.Computed() || got.Chapters != 5 || got.Chars != 1500 || !got.ComputedAt.Equal(at) {
		t.Fatalf("翻译结果 = %+v，期望 5 章 / 1500 字符 / 时间 %v", got, at)
	}
	args := got.qualityArgs()
	if len(args) != len(bookQualityColumns) {
		t.Fatalf("写入参数 = %d 个，期望 %d 个（与七列一致）", len(args), len(bookQualityColumns))
	}
	if zeroArgs := (BookQuality{}).qualityArgs(); zeroArgs[6] != "" {
		t.Errorf("未计算的 quality_at = %v，期望空串", zeroArgs[6])
	}
}
