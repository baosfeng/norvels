package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"norvels/internal/model"
)

// 本文件是「版本择优不显著增加判重开销」的性能证据（设计 §5 的性能约束：增量 < 50 ms）。
//
// 判定叠加的成本只有一次按主键的 SELECT（读旧书七列）+ 三个比较函数；
// 新书的质量分来自解析结果（ParsedBook.Quality），判定路径不重扫正文。
// 这里用一本 40 MB / 2000 章的书把两件事量出来：纯判重与「判重 + 版本择优」的耗时差。

// bigVersionChapters 造 40 MB / 2000 章的合成正文（与 TestCheckContentDuplicate40MB 同口径）。
func bigVersionChapters() []model.ParsedChapter {
	para := "梅宁城的风从城楼吹过，少年按住剑柄，远处的雪线压得很低。\n"
	const (
		chapterCount = 2000
		chapterBytes = 21 << 10 // 单章 21 KB x 2000 章 >= 40 MiB
	)
	body := strings.Repeat(para, chapterBytes/len(para)+1)
	chapters := make([]model.ParsedChapter, 0, chapterCount)
	for i := 0; i < chapterCount; i++ {
		chapters = append(chapters, model.ParsedChapter{
			Idx: i, VolumeIdx: -1, Title: fmt.Sprintf("第%d章", i+1),
			Content: fmt.Sprintf("第%d章\n%s", i, body),
		})
	}
	return chapters
}

// TestCheckBookDuplicateOverhead 版本择优的增量开销 < 50 ms，总耗时仍 < 200 ms。
func TestCheckBookDuplicateOverhead(t *testing.T) {
	if testing.Short() {
		t.Skip("性能用例要造 40 MB 合成语料")
	}
	if testing.CoverMode() != "" {
		t.Skip("覆盖率插桩下不计时")
	}
	ctx := context.Background()
	st := newTestStore(t)
	chapters := bigVersionChapters()
	total := corpusBytes(t, chapters)
	// 旧版本：同一批章节 + 一份已落库的质量分。
	saved := saveOrFail(t, ctx, st, withQuality(
		syntheticBook("大书", "/books/big.txt", strings.Repeat("g", 64), chapters),
		model.QualityMetrics{ChapterCount: len(chapters), TotalChars: total / 3, NoiseRatio: 0.2, StructureScore: 0.6}))
	fresh := withQuality(syntheticBook("大书新版", "/books/big2.txt", strings.Repeat("h", 64), chapters),
		model.QualityMetrics{ChapterCount: len(chapters), TotalChars: total / 3, StructureScore: 0.9})

	pure := timeDedupe(t, func() { mustDuplicate(t, st, ctx, chapters) })
	full := timeDedupe(t, func() { assertUpgradeOnBigBook(t, st, ctx, fresh, saved.BookID) })
	delta := full - pure
	t.Logf("纯判重 %s / 判重+版本择优 %s / 增量 %s", pure, full, delta)
	if limit := 50 * time.Millisecond; delta > limit {
		t.Errorf("版本择优增量 %s，超过 %s 的性能约束", delta, limit)
	}
	if limit := 200 * time.Millisecond; full > limit {
		t.Errorf("判定耗时 %s，超过 %s 的性能契约", full, limit)
	}
}

// corpusBytes 返回语料总字节，并断言它达到 40 MB 的判定口径。
func corpusBytes(t *testing.T, chapters []model.ParsedChapter) int {
	t.Helper()
	total := 0
	for i := range chapters {
		total += len(chapters[i].Content)
	}
	if total < 40<<20 {
		t.Fatalf("语料只有 %d 字节，达不到 40 MB 的口径", total)
	}
	return total
}

// assertUpgradeOnBigBook 在大书上跑一次「判重 + 版本择优」，断言结论是升级候选。
func assertUpgradeOnBigBook(t *testing.T, st *Store, ctx context.Context, fresh *model.ParsedBook, oldID int64) {
	t.Helper()
	rep, err := st.CheckBookDuplicate(ctx, fresh, DefaultDedupeThreshold)
	if err != nil {
		t.Fatalf("CheckBookDuplicate: %v", err)
	}
	if rep.DuplicateBookID != oldID || rep.Version == nil || !rep.Version.Upgrade {
		t.Fatalf("报告 = %+v，期望判成升级候选", rep)
	}
}

// timeDedupe 量一次判定的耗时。
func timeDedupe(t *testing.T, fn func()) time.Duration {
	t.Helper()
	start := time.Now()
	fn()
	return time.Since(start)
}

// mustDuplicate 跑一次纯判重（失败即中止）。
func mustDuplicate(t *testing.T, st *Store, ctx context.Context, chapters []model.ParsedChapter) {
	t.Helper()
	if _, err := st.CheckContentDuplicate(ctx, chapters); err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
}
