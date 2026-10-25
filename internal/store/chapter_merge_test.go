package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestMergeChaptersReusesContiguousSpan 拆完再合并：同一块的连续区间直接拉长 span，
// 不重压，因此块数、正文体积、库级聚合值都不动。
func TestMergeChaptersReusesContiguousSpan(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-contiguous",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "长章", Content: longChapter, WordCount: 20},
		})
	if _, err := st.SplitChapter(ctx, bookID, ids[0], []int{1}); err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
	split := chapterIDsOf(t, ctx, st, bookID)
	if len(split) != 2 {
		t.Fatalf("拆分后章数 = %d，期望 2", len(split))
	}
	before := mustMetrics(t, ctx, st)

	if err := st.MergeChapters(ctx, bookID, split); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if len(got) != 1 {
		t.Fatalf("合并后章数 = %d，期望 1", len(got))
	}
	// 首章 id 必须留住（阅读进度与书签按它指过来）
	if got[0].ID != split[0] {
		t.Errorf("首章 id = %d，期望 %d", got[0].ID, split[0])
	}
	requireString(t, "合并后标题", got[0].Title, "长章（1/2）")
	requireChapterText(t, ctx, st, bookID, split[0], longChapter)
	// 字数守恒：countWords 口径每段 4 字，共 16
	requireInt(t, "合并后字数", got[0].WordCount, 16)
	after := mustMetrics(t, ctx, st)
	requireInt(t, "正文体积（不该重压）", int(after.TextBytes), int(before.TextBytes))
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1", n)
	}
}

// TestMergeChaptersAcrossBlocksWritesNewBlock 两章分属不同块：只能新建一块装拼接后的正文。
func TestMergeChaptersAcrossBlocksWritesNewBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 每章约 660 KB：超过 1 MB 块目标的一半，两章必然各占一块
	first := strings.Repeat("甲段正文。", 44000) + "甲尾"
	second := strings.Repeat("乙段正文。", 44000) + "乙尾"
	bookID, ids := seedStructBook(t, ctx, st, "merge-blocks",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "前章", Content: first, WordCount: 100},
			{Idx: 1, VolumeIdx: 0, Title: "后章", Content: second, WordCount: 200},
		})
	if n := blockCount(t, ctx, st, bookID); n != 2 {
		t.Fatalf("初始块数 = %d，期望 2", n)
	}

	if err := st.MergeChapters(ctx, bookID, []int64{ids[1], ids[0]}); err != nil { // 故意给反序
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if len(got) != 1 || got[0].ID != ids[0] {
		t.Fatalf("合并结果 = %+v，期望只剩首章 %d", got, ids[0])
	}
	requireString(t, "合并后标题", got[0].Title, "前章")
	// 拼接顺序由 idx 决定，不跟参数顺序走
	requireChapterText(t, ctx, st, bookID, ids[0], first+second)
	requireInt(t, "合并后字数", got[0].WordCount, 300)
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（旧块无人引用应被回收）", n)
	}
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, 1)
	requireInt(t, "word_count", detail.WordCount, 300)
}

// TestMergeChaptersLeavesNeighboursIntact 合并中间两章：同一块仍被前后章引用，
// 块不许回收，前后章的正文一个字节都不许动。
func TestMergeChaptersLeavesNeighboursIntact(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-middle",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	before := mustMetrics(t, ctx, st)

	if err := st.MergeChapters(ctx, bookID, []int64{ids[1], ids[2]}); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"章甲", "章乙", "章丁", "章戊"})
	requireIdxSequence(t, got)
	requireChapterText(t, ctx, st, bookID, ids[0], "章甲正文。")
	requireChapterText(t, ctx, st, bookID, ids[1], "章乙正文。章丙正文。")
	requireChapterText(t, ctx, st, bookID, ids[3], "章丁正文。")
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Errorf("块数 = %d，期望 1（块仍被前后章引用，不许回收）", n)
	}
	after := mustMetrics(t, ctx, st)
	requireInt(t, "正文体积（不该重压）", int(after.TextBytes), int(before.TextBytes))
}

// TestMergeChaptersKeepsCountsAndUpdatedAt 三处计数同事务维护，books.updated_at 刷新。
func TestMergeChaptersKeepsCountsAndUpdatedAt(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedStructBook(t, ctx, st, "merge-counts",
		[]model.ParsedVolume{{Idx: 0, Title: "第一卷"}}, fiveChapters())
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", "2020-01-01T00:00:00.000Z", bookID); err != nil {
		t.Fatalf("重置 updated_at: %v", err)
	}
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)

	if err := st.MergeChapters(ctx, bookID, []int64{ids[0], ids[1], ids[2]}); err != nil {
		t.Fatalf("MergeChapters: %v", err)
	}
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, before.ChapterCount-2)
	// 字数守恒：三章 12 字并成 12 字，书级总字数与库级字数之和都不该变
	requireInt(t, "word_count", detail.WordCount, before.WordCount)
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum))
	if detail.UpdatedAt == before.UpdatedAt {
		t.Errorf("books.updated_at 没有刷新：%q", detail.UpdatedAt)
	}
	if got := chaptersOf(t, ctx, st, bookID); got[0].WordCount != 12 {
		t.Errorf("合并后首章字数 = %d，期望 12（各章之和）", got[0].WordCount)
	}
}

// TestMergeChaptersRejectsBadSelection 跨卷 / 不连续 / 单章 / 空 / 重复 / 跨书 一律拒绝。
func TestMergeChaptersRejectsBadSelection(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	vols := []model.ParsedVolume{{Idx: 0, Title: "第一卷"}, {Idx: 1, Title: "第二卷"}}
	bookID, ids := seedStructBook(t, ctx, st, "merge-reject", vols, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "甲", Content: "甲正文。", WordCount: 4},
		{Idx: 1, VolumeIdx: 0, Title: "乙", Content: "乙正文。", WordCount: 4},
		{Idx: 2, VolumeIdx: 0, Title: "丙", Content: "丙正文。", WordCount: 4},
		{Idx: 3, VolumeIdx: 1, Title: "丁", Content: "丁正文。", WordCount: 4},
	})
	_, otherIDs := seedStructBook(t, ctx, st, "merge-reject-other", vols, fiveChapters())

	cases := []struct {
		name string
		cids []int64
		want error
		why  string
	}{
		{"空列表", nil, ErrInvalidArgument, ""},
		{"重复 id", []int64{ids[0], ids[0]}, ErrInvalidArgument, ""},
		{"只有一章", []int64{ids[0]}, ErrNotMergeable, "少于 2 章"},
		{"跨卷", []int64{ids[2], ids[3]}, ErrNotMergeable, "跨卷"},
		{"不连续", []int64{ids[0], ids[2]}, ErrNotMergeable, "不连续"},
		{"别的书的章", []int64{ids[0], otherIDs[1]}, ErrNotFound, ""},
	}
	for _, tc := range cases {
		err := st.MergeChapters(ctx, bookID, tc.cids)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s：err = %v，期望 %v", tc.name, err, tc.want)
			continue
		}
		if tc.why != "" && !strings.Contains(err.Error(), tc.why) {
			t.Errorf("%s：错误信息缺少原因 %q：%v", tc.name, tc.why, err)
		}
	}
	if err := st.MergeChapters(ctx, 0, []int64{ids[0], ids[1]}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("书 id 非法 err = %v，期望 ErrInvalidArgument", err)
	}
	if got := chaptersOf(t, ctx, st, bookID); len(got) != 4 {
		t.Errorf("被拒的合并不该改目录，实际 %d 章", len(got))
	}
}
