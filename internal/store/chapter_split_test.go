package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// longChapter 是一章 4 段的正文，够用来切两刀。
const longChapter = "第一段落。\n第二段落。\n第三段落。\n第四段落。"

// TestSplitChapterByParagraph 一章按段落切成两章：段数、标题、正文都要对。
func TestSplitChapterByParagraph(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "长章", Content: longChapter, WordCount: 20},
	})

	pieces, err := st.SplitChapter(ctx, bookID, ids[0], []int{1})
	if err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
	requireInt(t, "返回章数", pieces, 2)

	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"长章（1/2）", "长章（2/2）"})
	requireIdxSequence(t, got)
	requireChapterText(t, ctx, st, bookID, got[0].ID, "第一段落。\n第二段落。\n")
	requireChapterText(t, ctx, st, bookID, got[1].ID, "第三段落。\n第四段落。")
}

// TestSplitChapterKeepsTextAndWords 拆完拼回来等于原正文；两章字数之和按 countWords 口径。
func TestSplitChapterKeepsTextAndWords(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "长章", Content: longChapter, WordCount: 20},
	})
	before := bookByID(t, ctx, st, bookID)

	pieces, err := st.SplitChapter(ctx, bookID, ids[0], []int{1, 2})
	if err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
	requireInt(t, "返回章数", pieces, 3)

	got := chaptersOf(t, ctx, st, bookID)
	joined := ""
	words := 0
	for _, chapter := range got {
		joined += mustChapter(t, ctx, st, bookID, chapter.ID).Content
		words += chapter.WordCount
	}
	requireString(t, "拼回来的正文", joined, longChapter)
	// countWords 口径：CJK 逐字计数、标点与空白不算 → 每段 4 字，共 16
	requireInt(t, "三章字数之和", words, 16)
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, 3)
	requireInt(t, "word_count", detail.WordCount, before.WordCount-20+16)
}

// TestSplitChapterShiftsFollowingChapters 拆中间那章：后面的章整体后移，且不受影响。
func TestSplitChapterShiftsFollowingChapters(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "开头", Content: "开头正文。", WordCount: 5},
		{Idx: 1, VolumeIdx: 0, Title: "中间", Content: longChapter, WordCount: 20},
		{Idx: 2, VolumeIdx: 0, Title: "结尾", Content: "结尾正文。", WordCount: 5},
	})

	if _, err := st.SplitChapter(ctx, bookID, ids[1], []int{1}); err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if len(got) != 4 {
		t.Fatalf("应变成 4 章，实际 %d", len(got))
	}
	wantTitles := []string{"开头", "中间（1/2）", "中间（2/2）", "结尾"}
	for i, want := range wantTitles {
		if got[i].Title != want || got[i].Idx != i {
			t.Errorf("第 %d 章 = %q(idx=%d)，期望 %q(idx=%d)", i, got[i].Title, got[i].Idx, want, i)
		}
	}
	// 首尾两章的 id 与正文必须原样保住（读进度、书签都按 chapter id 指过来）
	if got[0].ID != ids[0] || got[3].ID != ids[2] {
		t.Errorf("首尾章 id 变了：%d / %d，期望 %d / %d", got[0].ID, got[3].ID, ids[0], ids[2])
	}
	if tail := mustChapter(t, ctx, st, bookID, ids[2]); tail.Content != "结尾正文。" {
		t.Errorf("结尾章正文 = %q", tail.Content)
	}
	if detail := bookByID(t, ctx, st, bookID); detail.ChapterCount != 4 {
		t.Errorf("chapter_count = %d，期望 4", detail.ChapterCount)
	}
}

// TestSplitChapterKeepsVolume 拆出来的章留在原卷里（否则目录会凭空多出一卷「未分卷」）。
func TestSplitChapterKeepsVolume(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "卷内章", Content: longChapter, WordCount: 20},
	})
	if _, err := st.SplitChapter(ctx, bookID, ids[0], []int{0, 2}); err != nil {
		t.Fatalf("SplitChapter: %v", err)
	}
	detail := bookByID(t, ctx, st, bookID)
	if len(detail.Volumes) != 1 {
		t.Fatalf("卷数 = %d，期望 1", len(detail.Volumes))
	}
	if n := len(detail.Volumes[0].Chapters); n != 3 {
		t.Errorf("卷内章数 = %d，期望 3", n)
	}
}

// TestSplitChapterRejectsBadCuts 拒绝：没给点、点乱序、点越界、只一段、切出空章。
func TestSplitChapterRejectsBadCuts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "长章", Content: longChapter, WordCount: 20},
		{Idx: 1, VolumeIdx: 0, Title: "单段章", Content: "只有一段没有换行。", WordCount: 8},
		{Idx: 2, VolumeIdx: 0, Title: "空行章", Content: "前段。\n\n\n后段。", WordCount: 6},
	})
	cases := []struct {
		name string
		chap int64
		cuts []int
	}{
		{"没有拆分点", ids[0], nil},
		{"乱序", ids[0], []int{2, 1}},
		{"重复", ids[0], []int{1, 1}},
		{"越界", ids[0], []int{3}},
		{"负数", ids[0], []int{-1}},
		{"只有一个段落", ids[1], []int{0}},
		{"切出空章", ids[2], []int{0, 1}},
	}
	for _, tc := range cases {
		if _, err := st.SplitChapter(ctx, bookID, tc.chap, tc.cuts); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s：err = %v，期望 ErrInvalidArgument", tc.name, err)
		}
	}
	if _, err := st.SplitChapter(ctx, bookID, 99999, []int{0}); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的章 err = %v，期望 ErrNotFound", err)
	}
	// 失败不能留下半截改动
	if got := chaptersOf(t, ctx, st, bookID); len(got) != 3 {
		t.Errorf("被拒的拆分不该改目录，实际 %d 章", len(got))
	}
}

// TestSplitChapterTooManyPieces 一次最多 100 章：101 段拆点被拒。
func TestSplitChapterTooManyPieces(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "很多段", Content: strings.Repeat("一段。\n", 120), WordCount: 240},
	})
	cuts := make([]int, 0, 100)
	for i := 0; i < 100; i++ {
		cuts = append(cuts, i)
	}
	if _, err := st.SplitChapter(ctx, bookID, ids[0], cuts); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("101 章 err = %v，期望 ErrInvalidArgument", err)
	}
}

// mustChapter 读一章正文（不存在或不属于本书直接 fail）。
func mustChapter(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64) *ChapterContent {
	t.Helper()
	ch, err := st.GetChapter(ctx, bookID, chapterID)
	if err != nil || ch == nil {
		t.Fatalf("GetChapter(%d, %d): %v", bookID, chapterID, err)
	}
	return ch
}

// requireInt 断言两个整数相等。
func requireInt(t *testing.T, what string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %d，期望 %d", what, got, want)
	}
}

// requireString 断言两个字符串相等。
func requireString(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %q，期望 %q", what, got, want)
	}
}

// requireTitles 断言目录标题顺序完全一致。
func requireTitles(t *testing.T, got []ChapterBrief, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("章数 = %d，期望 %d（%v）", len(got), len(want), want)
	}
	for i, title := range want {
		if got[i].Title != title {
			t.Fatalf("第 %d 章 = %q，期望 %q", i, got[i].Title, title)
		}
	}
}

// requireIdxSequence 断言章节序号从 0 连续排到 n-1。
func requireIdxSequence(t *testing.T, got []ChapterBrief) {
	t.Helper()
	for i, chapter := range got {
		if chapter.Idx != i {
			t.Fatalf("第 %d 章的 idx = %d，期望 %d", i, chapter.Idx, i)
		}
	}
}

// requireChapterText 断言某一章的正文完全一致。
func requireChapterText(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64, want string) {
	t.Helper()
	got := mustChapter(t, ctx, st, bookID, chapterID).Content
	if got != want {
		t.Fatalf("第 %d 章正文 = %q，期望 %q", chapterID, got, want)
	}
}
