package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// seedBookWithChapters 造一本三章的合成书，返回 (bookID, 首章 id, 第三章 id)。
func seedBookWithChapters(t *testing.T, ctx context.Context, st *Store, title, sha string) (int64, int64, int64) {
	t.Helper()
	res, err := st.SaveBook(ctx, sampleBook(title, "书签作者", "/books/"+title+".txt", sha), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, From: 1, Limit: 10})
	if err != nil || page.Total < 3 {
		t.Fatalf("ListChapters = %+v, err=%v", page, err)
	}
	return res.BookID, page.Items[0].ID, page.Items[len(page.Items)-1].ID
}

// TestAddBookmarkIsIdempotent 锁死「同一位置重复添加只更新 label」的幂等口径。
func TestAddBookmarkIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, chapterID, _ := seedBookWithChapters(t, ctx, st, "幂等之书", strings.Repeat("e", 64))

	first := requireAddBookmark(t, st, bookID, chapterID, 3, "第一次标注", true)
	second := requireAddBookmark(t, st, bookID, chapterID, 3, "改过的标注", false)
	if second.ID != first.ID {
		t.Errorf("幂等更新应复用同一行：first=%d second=%d", first.ID, second.ID)
	}
	requireBookmarkCount(t, st, bookID, 1)
	requireOnlyBookmarkLabel(t, st, bookID, "改过的标注")
}

// TestListBookmarksOrderAndFields 断言倒序与联表补出的章名/章序号。
func TestListBookmarksOrderAndFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, chapterID, lastChapterID := seedBookWithChapters(t, ctx, st, "倒序之书", strings.Repeat("f", 64))

	if _, _, err := st.AddBookmark(ctx, bookID, chapterID, 0, "先加的"); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	if _, _, err := st.AddBookmark(ctx, bookID, lastChapterID, 1, "后加的"); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	items, err := st.ListBookmarks(ctx, bookID)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListBookmarks = %+v, err=%v", items, err)
	}
	if items[0].Label != "后加的" {
		t.Errorf("应按 created_at 倒序：首条 = %q，期望「后加的」", items[0].Label)
	}
	last := items[0]
	if last.ChapterIdx != 2 || last.ChapterTitle != "番外 小记" || last.ParaIndex != 1 {
		t.Errorf("书签字段不完整: %+v", last)
	}
	if last.CreatedAt == "" {
		t.Error("createdAt 为空")
	}
}

// TestAddBookmarkValidation 覆盖入参边界：不属于本书的章节、负段落下标、超长 label。
func TestAddBookmarkValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, chapterID, _ := seedBookWithChapters(t, ctx, st, "校验之书", strings.Repeat("1", 64))
	otherID, otherChapterID, _ := seedBookWithChapters(t, ctx, st, "另一本书", strings.Repeat("2", 64))

	cases := []struct {
		name      string
		bookID    int64
		chapterID int64
		paraIndex int
		label     string
		want      error
	}{
		{"章节不属于该书", bookID, otherChapterID, 0, "", ErrInvalidArgument},
		{"段落下标为负", bookID, chapterID, -1, "", ErrInvalidArgument},
		{"label 超长", bookID, chapterID, 0, strings.Repeat("长", maxBookmarkLabelRunes+1), ErrInvalidArgument},
		{"书不存在", 99999, chapterID, 0, "", ErrNotFound},
		{"章节 id 为 0", bookID, 0, 0, "", ErrInvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := st.AddBookmark(ctx, tc.bookID, tc.chapterID, tc.paraIndex, tc.label); !errors.Is(err, tc.want) {
				t.Errorf("err = %v，期望 %v", err, tc.want)
			}
		})
	}
	if n, err := st.CountBookmarks(ctx, otherID); err != nil || n != 0 {
		t.Errorf("校验失败不应写入另一本书: n=%d err=%v", n, err)
	}
}

// TestDeleteBookmark 删除成功、重复删除 404、非法 id 400。
func TestDeleteBookmark(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, chapterID, _ := seedBookWithChapters(t, ctx, st, "删除书签之书", strings.Repeat("3", 64))
	bm, _, err := st.AddBookmark(ctx, bookID, chapterID, 0, "")
	if err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	if err := st.DeleteBookmark(ctx, bm.ID); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
	if err := st.DeleteBookmark(ctx, bm.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 err = %v，期望 ErrNotFound", err)
	}
	if err := st.DeleteBookmark(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
	if n, err := st.CountBookmarks(ctx, bookID); err != nil || n != 0 {
		t.Errorf("删除后计数 = %d, err=%v", n, err)
	}
}

// TestListBookmarksMissingBook 书不存在的列表请求返回 ErrNotFound（而不是空数组）。
func TestListBookmarksMissingBook(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.ListBookmarks(context.Background(), 424242); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v，期望 ErrNotFound", err)
	}
}

// TestReadStatusPatchAndFilter 锁死阅读状态的白名单、读写与书架筛选。
func TestReadStatusPatchAndFilter(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	wantID, _, _ := seedBookWithChapters(t, ctx, st, "想读之书", strings.Repeat("4", 64))
	readingID, _, _ := seedBookWithChapters(t, ctx, st, "在读之书", strings.Repeat("5", 64))
	doneID, _, _ := seedBookWithChapters(t, ctx, st, "已读之书", strings.Repeat("6", 64))

	setReadStatus(t, st, wantID, ReadStatusWant)
	setReadStatus(t, st, readingID, ReadStatusReading)
	setReadStatus(t, st, doneID, ReadStatusDone)
	if detail, err := st.GetBook(ctx, doneID); err != nil || detail.ReadStatus != ReadStatusDone {
		t.Fatalf("GetBook = %+v, err=%v", detail, err)
	}
	requireBookFiltered(t, st, ReadStatusDone, doneID)
	requireAllBooks(t, st, 3)
	requireInvalidReadStatus(t, st, wantID)
}

// requireAddBookmark 添加一次书签并断言「是否新建」符合预期。
func requireAddBookmark(t *testing.T, st *Store, bookID, chapterID int64, paraIndex int, label string, wantCreated bool) Bookmark {
	t.Helper()
	bm, created, err := st.AddBookmark(context.Background(), bookID, chapterID, paraIndex, label)
	if err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	if created != wantCreated {
		t.Fatalf("AddBookmark created = %v，期望 %v", created, wantCreated)
	}
	return bm
}

// requireBookmarkCount 断言某本书的书签总数。
func requireBookmarkCount(t *testing.T, st *Store, bookID int64, want int) {
	t.Helper()
	n, err := st.CountBookmarks(context.Background(), bookID)
	if err != nil || n != want {
		t.Fatalf("书签总数 = %d, err=%v，期望 %d", n, err, want)
	}
}

// requireOnlyBookmarkLabel 断言全书恰好一条书签且 label 等于 want。
func requireOnlyBookmarkLabel(t *testing.T, st *Store, bookID int64, want string) {
	t.Helper()
	items, err := st.ListBookmarks(context.Background(), bookID)
	if err != nil || len(items) != 1 || items[0].Label != want {
		t.Fatalf("ListBookmarks = %+v, err=%v，期望 1 条且 label=%q", items, err, want)
	}
}

// setReadStatus 设置一本书的阅读状态。
func setReadStatus(t *testing.T, st *Store, bookID int64, status string) {
	t.Helper()
	s := status
	if err := st.UpdateBookMeta(context.Background(), bookID, BookMetaPatch{ReadStatus: &s}); err != nil {
		t.Fatalf("UpdateBookMeta(%d, %q): %v", bookID, status, err)
	}
}

// requireBookFiltered 断言按状态筛选只返回指定那一本。
func requireBookFiltered(t *testing.T, st *Store, status string, wantID int64) {
	t.Helper()
	page, err := st.ListBooks(context.Background(), BookQuery{Page: 1, Size: 10, ReadStatus: status})
	if err != nil || page.Total != 1 || page.Items[0].ID != wantID {
		t.Fatalf("按 %q 筛选 = %+v, err=%v", status, page, err)
	}
}

// requireAllBooks 断言空 readStatus 不过滤。
func requireAllBooks(t *testing.T, st *Store, want int) {
	t.Helper()
	page, err := st.ListBooks(context.Background(), BookQuery{Page: 1, Size: 10})
	if err != nil || page.Total != want {
		t.Fatalf("不带筛选 total = %d, err=%v，期望 %d", page.Total, err, want)
	}
}

// requireInvalidReadStatus 断言非法阅读状态被 store 拒绝。
func requireInvalidReadStatus(t *testing.T, st *Store, bookID int64) {
	t.Helper()
	bad := "abc"
	if err := st.UpdateBookMeta(context.Background(), bookID, BookMetaPatch{ReadStatus: &bad}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法 readStatus err = %v，期望 ErrInvalidArgument", err)
	}
}

// TestProgressAutoMarksReading 锁死唯一自动转移：” → reading，不覆盖 want / done。
func TestProgressAutoMarksReading(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, chapterID, _ := seedBookWithChapters(t, ctx, st, "自动在读之书", strings.Repeat("7", 64))

	if err := st.SaveReadingProgress(ctx, ReadingProgress{BookID: bookID, ChapterID: chapterID, Percent: 0.2}); err != nil {
		t.Fatalf("SaveReadingProgress: %v", err)
	}
	if detail, err := st.GetBook(ctx, bookID); err != nil || detail.ReadStatus != ReadStatusReading {
		t.Fatalf("写进度后 readStatus = %+v, err=%v，期望 reading", detail, err)
	}
	for _, keep := range []string{ReadStatusWant, ReadStatusDone} {
		status := keep
		if err := st.UpdateBookMeta(ctx, bookID, BookMetaPatch{ReadStatus: &status}); err != nil {
			t.Fatalf("UpdateBookMeta: %v", err)
		}
		if err := st.SaveReadingProgress(ctx, ReadingProgress{BookID: bookID, ChapterID: chapterID, Percent: 0.4}); err != nil {
			t.Fatalf("SaveReadingProgress: %v", err)
		}
		if detail, err := st.GetBook(ctx, bookID); err != nil || detail.ReadStatus != keep {
			t.Errorf("用户标记 %q 被进度覆盖成 %q, err=%v", keep, detail.ReadStatus, err)
		}
	}
}
