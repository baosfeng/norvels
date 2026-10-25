package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// seedChapterBook 写一本只有指定章节的书，返回 bookID 与按目录顺序的章节 id。
func seedChapterBook(t *testing.T, ctx context.Context, st *Store, chapters []model.ParsedChapter) (int64, []int64) {
	t.Helper()
	b := &model.ParsedBook{
		Title:      "章节编辑样本",
		Author:     "样本作者",
		SourcePath: "/books/chapter-edit.txt",
		SourceName: "chapter-edit.txt",
		Encoding:   "utf-8",
		FileSize:   2048,
		SHA256:     strings.Repeat("c", 64),
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "第一卷"}},
		Chapters:   chapters,
	}
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, From: 1, Limit: 100})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	ids := make([]int64, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return res.BookID, ids
}

// chaptersOf 读回目录（标题 + 字数），供断言顺序与重排结果。
func chaptersOf(t *testing.T, ctx context.Context, st *Store, bookID int64) []ChapterBrief {
	t.Helper()
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: bookID, From: 1, Limit: 100})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	return page.Items
}

func bookByID(t *testing.T, ctx context.Context, st *Store, bookID int64) *BookDetail {
	t.Helper()
	detail, err := st.GetBook(ctx, bookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	return detail
}

// TestUpdateChapterTitle 锁死改标题的三条口径：成功落库、书 updated_at 跟着动、非法标题被拒。
func TestUpdateChapterTitle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "旧标题", Content: "第一段正文。", WordCount: 6},
	})
	// 先把 updated_at 压到一个过去的时间：否则「导入 + 改标题」可能落在同一毫秒，
	// 断言「时间变了」就会假失败。
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", "2020-01-01T00:00:00.000Z", bookID); err != nil {
		t.Fatalf("重置 updated_at: %v", err)
	}
	before := bookByID(t, ctx, st, bookID).UpdatedAt

	if err := st.UpdateChapterTitle(ctx, bookID, ids[0], "  新标题  "); err != nil {
		t.Fatalf("UpdateChapterTitle: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	if got[0].Title != "新标题" {
		t.Errorf("标题 = %q，期望去空白后的「新标题」", got[0].Title)
	}
	if after := bookByID(t, ctx, st, bookID).UpdatedAt; after == before {
		t.Errorf("书的 updated_at 没有刷新：%q", after)
	}

	if err := st.UpdateChapterTitle(ctx, bookID, ids[0], "   "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空标题 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.UpdateChapterTitle(ctx, bookID, ids[0], strings.Repeat("长", 201)); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("超长标题 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.UpdateChapterTitle(ctx, bookID, 99999, "标题"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的章 err = %v，期望 ErrNotFound", err)
	}
	if err := st.UpdateChapterTitle(ctx, 99999, ids[0], "标题"); !errors.Is(err, ErrNotFound) {
		t.Errorf("错书的章 err = %v，期望 ErrNotFound", err)
	}
}

// TestDeleteChapterShiftsIdx 删中间那章：序号补齐、剩下章节的正文不乱。
func TestDeleteChapterShiftsIdx(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "第一章", Content: "一章正文。", WordCount: 5},
		{Idx: 1, VolumeIdx: 0, Title: "第二章", Content: "二章正文。", WordCount: 5},
		{Idx: 2, VolumeIdx: 0, Title: "第三章", Content: "三章正文。", WordCount: 5},
	})

	if err := st.DeleteChapter(ctx, bookID, ids[1]); err != nil {
		t.Fatalf("DeleteChapter: %v", err)
	}
	got := chaptersOf(t, ctx, st, bookID)
	requireTitles(t, got, []string{"第一章", "第三章"})
	requireIdxSequence(t, got)
	requireChapterText(t, ctx, st, bookID, ids[0], "一章正文。")
	requireChapterText(t, ctx, st, bookID, ids[2], "三章正文。")

	if err := st.DeleteChapter(ctx, bookID, ids[1]); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 err = %v，期望 ErrNotFound", err)
	}
}

// TestDeleteChapterAdjustsCounts 删一章：书级计数与库级字数一起减。
func TestDeleteChapterAdjustsCounts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "第一章", Content: "一章正文。", WordCount: 5},
		{Idx: 1, VolumeIdx: 0, Title: "第二章", Content: "二章正文。", WordCount: 7},
	})
	before := bookByID(t, ctx, st, bookID)
	metricsBefore := mustMetrics(t, ctx, st)

	if err := st.DeleteChapter(ctx, bookID, ids[1]); err != nil {
		t.Fatalf("DeleteChapter: %v", err)
	}
	detail := bookByID(t, ctx, st, bookID)
	requireInt(t, "chapter_count", detail.ChapterCount, before.ChapterCount-1)
	requireInt(t, "word_count", detail.WordCount, before.WordCount-7)
	after := mustMetrics(t, ctx, st)
	requireInt(t, "metrics 章节字数", int(after.ChapterWordSum), int(metricsBefore.ChapterWordSum-7))
}

// TestDeleteLastChapterReapsBlock 删掉整本书唯一一章：块随之回收，正文体积降下来。
func TestDeleteLastChapterReapsBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, ids := seedChapterBook(t, ctx, st, []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "唯一一章", Content: strings.Repeat("正文字符。", 200), WordCount: 1000},
	})
	before := mustMetrics(t, ctx, st)
	if n := blockCount(t, ctx, st, bookID); n != 1 {
		t.Fatalf("初始块数 = %d，期望 1", n)
	}

	if err := st.DeleteChapter(ctx, bookID, ids[0]); err != nil {
		t.Fatalf("DeleteChapter: %v", err)
	}
	if n := blockCount(t, ctx, st, bookID); n != 0 {
		t.Errorf("删光章节后块数 = %d，期望 0", n)
	}
	after := mustMetrics(t, ctx, st)
	if after.TextBytes >= before.TextBytes {
		t.Errorf("正文体积没有降：%d → %d", before.TextBytes, after.TextBytes)
	}
	if after.ChapterWordSum >= before.ChapterWordSum {
		t.Errorf("章节字数没有降：%d → %d", before.ChapterWordSum, after.ChapterWordSum)
	}
}

// blockCount 数这本书还剩几个正文块。
func blockCount(t *testing.T, ctx context.Context, st *Store, bookID int64) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&n); err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	return n
}

// mustMetrics 读库级聚合值（拆/删之后的 metrics 断言都走它）。
func mustMetrics(t *testing.T, ctx context.Context, st *Store) metricsSnapshot {
	t.Helper()
	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	return snap
}
