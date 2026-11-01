package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件锁死「版本择优」在 store 侧的判定链（设计 §5）：CheckBookDuplicate 在章级判重之上
// 比一次质量分，命中率没到阈值时不做对比。判定规则本身在 versionpick_test.go。
//
// 语料一律合成（虚构书名 + 造出来的正文与数值），不碰真实库与真实书名。

// versionBook 造一本「带质量分」的书（供版本择优用例复用）。
func versionBook(title, path, sha string, chapters []model.ParsedChapter, q model.QualityMetrics) *model.ParsedBook {
	return withQuality(syntheticBook(title, path, sha, chapters), q)
}

// assertDuplicateOf 断言报告判成「书 bookID 的重复」。
func assertDuplicateOf(t *testing.T, rep DedupeReport, bookID int64) {
	t.Helper()
	if rep.Level(DefaultDedupeThreshold) != DedupeLevelDuplicate || rep.DuplicateBookID != bookID {
		t.Fatalf("报告 = %+v，期望判成书 %d 的重复", rep, bookID)
	}
	if rep.Version == nil {
		t.Fatal("命中率到阈值时必须做版本对比")
	}
}

// assertUpgradePick 断言版本对比是「升级候选：三项更优、零项更差」。
func assertUpgradePick(t *testing.T, pick *VersionPick) {
	t.Helper()
	if !pick.Upgrade || len(pick.Better) != 3 || len(pick.Worse) != 0 {
		t.Fatalf("版本对比 = %+v，期望升级候选且三项更优、零项更差", pick)
	}
	if !pick.OldComputed || !pick.NewComputed {
		t.Error("两侧都有质量分，OldComputed/NewComputed 应为真")
	}
}

// assertOldQualityKept 断言旧书的噪声率没被改写（判定只给结论，不自动替换）。
func assertOldQualityKept(t *testing.T, ctx context.Context, st *Store, bookID int64, want float64) {
	t.Helper()
	got, err := st.bookQualityByID(ctx, bookID)
	if err != nil || got.NoiseRatio != want {
		t.Errorf("判定后旧书质量分 = %+v（err=%v），期望噪声率仍是 %v", got, err, want)
	}
}

// TestCheckBookDuplicateUpgrade 命中率到阈值 + 新版本三项显著更优 → 升级候选（不自动替换）。
func TestCheckBookDuplicateUpgrade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(10)
	old := versionBook("旧版之书", "/books/v1.txt", strings.Repeat("v", 64), chapters,
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, NoiseRatio: 0.4, GarbageRatio: 0.02, StructureScore: 0.5})
	saved := saveOrFail(t, ctx, st, old)

	// 只有排版差异（指纹相同、SHA 不同）+ 质量分全面更好。
	fresh := versionBook("新版之书", "/books/v2.txt", strings.Repeat("w", 64), noisyChapters(chapters),
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, NoiseRatio: 0.004, GarbageRatio: 0.001, StructureScore: 0.9})
	rep, err := st.CheckBookDuplicate(ctx, fresh, DefaultDedupeThreshold)
	if err != nil {
		t.Fatalf("CheckBookDuplicate: %v", err)
	}
	assertDuplicateOf(t, rep, saved.BookID)
	assertUpgradePick(t, rep.Version)
	assertOldQualityKept(t, ctx, st, saved.BookID, 0.4)
}

// TestCheckBookDuplicateNewWorse 新版本更脏（噪声升、乱码升）→ 判重复，不是升级候选。
func TestCheckBookDuplicateNewWorse(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(10)
	old := versionBook("干净之书", "/books/w1.txt", strings.Repeat("p", 64), chapters,
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, StructureScore: 0.9})
	saveOrFail(t, ctx, st, old)

	dirty := versionBook("更脏之书", "/books/w2.txt", strings.Repeat("q", 64), noisyChapters(chapters),
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, NoiseRatio: 0.3, GarbageRatio: 0.05, StructureScore: 0.9})
	rep, err := st.CheckBookDuplicate(ctx, dirty, DefaultDedupeThreshold)
	if err != nil {
		t.Fatalf("CheckBookDuplicate: %v", err)
	}
	if rep.Version == nil || rep.Version.Upgrade {
		t.Fatalf("版本对比 = %+v，期望判重复（新版本更差）", rep.Version)
	}
	if len(rep.Version.Worse) != 2 {
		t.Errorf("更差维度 = %d 项，期望 2 项（噪声 + 乱码）", len(rep.Version.Worse))
	}
}

// TestCheckBookDuplicateOldWithoutQuality 旧版本没有质量分 → 保守路径（判重复 + 说明原因）。
func TestCheckBookDuplicateOldWithoutQuality(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(10)
	saveOrFail(t, ctx, st, syntheticBook("无分之书", "/books/nq.txt", strings.Repeat("n", 64), chapters))

	fresh := versionBook("新版之书", "/books/nq2.txt", strings.Repeat("m", 64), noisyChapters(chapters),
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, StructureScore: 1})
	rep, err := st.CheckBookDuplicate(ctx, fresh, DefaultDedupeThreshold)
	if err != nil {
		t.Fatalf("CheckBookDuplicate: %v", err)
	}
	if rep.Version == nil || rep.Version.Upgrade {
		t.Fatalf("版本对比 = %+v，期望走保守路径", rep.Version)
	}
	if rep.Version.Note != VersionNoteOldUnknown {
		t.Errorf("保守路径说明 = %q，期望 %q", rep.Version.Note, VersionNoteOldUnknown)
	}
}

// TestCheckBookDuplicateBelowThreshold 命中率没到阈值时不做版本对比（不是同一本书，没有可比对象）。
func TestCheckBookDuplicateBelowThreshold(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(10)
	saveOrFail(t, ctx, st, versionBook("原书", "/books/t1.txt", strings.Repeat("t", 64), chapters,
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, StructureScore: 0.5}))

	changed := syntheticChapters(10)
	changed[0].Content = "这一章被彻底改写：风雪、孤城、另一个结局。"
	fresh := versionBook("改版", "/books/t2.txt", strings.Repeat("u", 64), changed,
		model.QualityMetrics{ChapterCount: 10, TotalChars: 900, StructureScore: 0.9})
	rep, err := st.CheckBookDuplicate(ctx, fresh, DefaultDedupeThreshold)
	if err != nil {
		t.Fatalf("CheckBookDuplicate: %v", err)
	}
	if rep.Level(DefaultDedupeThreshold) != DedupeLevelRelated {
		t.Fatalf("分档 = %q，期望 %q", rep.Level(DefaultDedupeThreshold), DedupeLevelRelated)
	}
	if rep.Version != nil {
		t.Errorf("命中率没到阈值不该做版本对比：%+v", rep.Version)
	}
}
