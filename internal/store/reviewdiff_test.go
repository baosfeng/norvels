package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 本文件锁死「审核输入构造」（reviewdiff.go）的口径：按章 idx 对齐、空白不计差异、
// 缺章不进片段、片段与比对章数都有上限（等量抽样、确定性）。

// TestSampleReviewDiff 按章 idx 对齐、只挑正文不同的章、带上下文。
func TestSampleReviewDiff(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)

	diff, err := st.SampleReviewDiff(ctx, oldID, newID)
	if err != nil {
		t.Fatalf("SampleReviewDiff: %v", err)
	}
	if diff.OldChapters != 3 || diff.NewChapters != 3 || diff.Compared != 3 {
		t.Errorf("对齐结果 = %+v，期望 3 章全对齐", diff)
	}
	if diff.Differing != 1 || len(diff.Segments) != 1 {
		t.Fatalf("差异 = %d 章 / %d 段，期望 1 / 1（只有第 2 章不同）", diff.Differing, len(diff.Segments))
	}
	assertDiffSegment(t, diff.Segments[0])
}

// assertDiffSegment 断言片段形状：章号、两版正文、共有上下文。
func assertDiffSegment(t *testing.T, seg ReviewSegment) {
	t.Helper()
	if seg.Idx != 1 {
		t.Errorf("差异章 idx = %d，期望 1", seg.Idx)
	}
	if !strings.Contains(seg.Old, "梅宁城") || !strings.Contains(seg.New, "临川渡") {
		t.Errorf("片段 = 旧 %q / 新 %q，期望各自保留差异正文", seg.Old, seg.New)
	}
	// 上下文取自两版共有的文字：差异前的残句 + 差异后的整句。
	if !strings.Contains(seg.Context, "他站在城楼上") || !strings.Contains(seg.Context, "与风雪。") {
		t.Errorf("上下文 = %q，期望包含差异前后的共有文字", seg.Context)
	}
}

// TestSampleReviewDiffIdenticalAndMisaligned 排版差异不算差异；缺章不进片段。
func TestSampleReviewDiffIdenticalAndMisaligned(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID := mustSaveBook(t, ctx, st, sampleBook("甲", "示例作者", "/books/a.txt", strings.Repeat("c", 64)))

	// 只改排版（多插空行）：不该产生差异片段。
	same := sampleBook("甲", "示例作者", "/books/a2.txt", strings.Repeat("d", 64))
	same.Chapters[0].Content = "华夏国临川市，午后的秋日，微风拂过城楼。\n\n"
	sameID := mustSaveBook(t, ctx, st, same)
	diff, err := st.SampleReviewDiff(ctx, oldID, sameID)
	if err != nil {
		t.Fatalf("SampleReviewDiff(排版差异): %v", err)
	}
	if diff.Differing != 0 || len(diff.Segments) != 0 {
		t.Errorf("排版差异 = %d 章，期望 0（空白不计差异）", diff.Differing)
	}

	// 少一章：只在两边都存在的 idx 上比对。
	short := sampleBook("甲", "示例作者", "/books/a3.txt", strings.Repeat("e", 64))
	short.Chapters = short.Chapters[:2]
	shortID := mustSaveBook(t, ctx, st, short)
	diff, err = st.SampleReviewDiff(ctx, oldID, shortID)
	if err != nil {
		t.Fatalf("SampleReviewDiff(缺章): %v", err)
	}
	if diff.OldChapters != 3 || diff.NewChapters != 2 || diff.Compared != 2 {
		t.Errorf("缺章对齐 = %+v，期望 3/2 章、比对 2 章", diff)
	}
	if diff.Differing != 0 {
		t.Errorf("缺章 = %d 章差异，期望 0（缺章走完整性维度，不进片段）", diff.Differing)
	}
	if _, err := st.SampleReviewDiff(ctx, 0, oldID); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("非法 id err = %v，期望 ErrInvalidArgument", err)
	}
}

// TestSampleSegmentsCap 片段与比对章数都有上限（等距抽样、确定性）。
func TestSampleSegmentsCap(t *testing.T) {
	diffs := make([]chapterDiff, 0, 120)
	for i := 0; i < 120; i++ {
		diffs = append(diffs, chapterDiff{idx: i, old: "旧版正文" + strings.Repeat("甲", i%3), new: "新版正文" + strings.Repeat("乙", i%3)})
	}
	segs := sampleSegments(diffs, maxReviewSegments)
	if len(segs) != maxReviewSegments {
		t.Fatalf("片段数 = %d，期望 %d", len(segs), maxReviewSegments)
	}
	if segs[0].Idx != 0 || segs[len(segs)-1].Idx < 100 {
		t.Errorf("抽样范围 = %d..%d，期望覆盖全书（等距）", segs[0].Idx, segs[len(segs)-1].Idx)
	}
	pairs := make([]chapterPair, 0, 900)
	for i := 0; i < 900; i++ {
		pairs = append(pairs, chapterPair{idx: i})
	}
	if got := limitChapterPairs(pairs, maxReviewScanChapters); len(got) != maxReviewScanChapters {
		t.Errorf("比对章数 = %d，期望压到 %d", len(got), maxReviewScanChapters)
	}
	if got := sampleSegments(nil, maxReviewSegments); len(got) != 0 {
		t.Errorf("空差异 = %d 段，期望 0", len(got))
	}
}
