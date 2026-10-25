package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestDeleteBookCascades 锁死删除口径：一本书连同全部从属数据在一个事务里消失，
// 作者聚合列随之重算，且删第二次是 404 而不是静默成功。
func TestDeleteBookCascades(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedDeletableBook(t, ctx, st)

	if err := st.DeleteBook(ctx, bookID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	requireBookRowsGone(t, ctx, st, bookID)
	requireAuthorStatsReset(t, ctx, st)
	requireProgressCleared(t, ctx, st)

	if err := st.DeleteBook(ctx, bookID); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 err = %v，期望 ErrNotFound", err)
	}
	if err := st.DeleteBook(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
}

// TestDeleteBookKeepsOtherBooks 删一本不能波及另一本（同一作者的书尤其要保住作者行）。
func TestDeleteBookKeepsOtherBooks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	keep := sampleBook("留下来的书", "同一作者", "/books/keep.txt", strings.Repeat("a", 64))
	keepRes, err := st.SaveBook(ctx, keep, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook keep: %v", err)
	}
	drop := sampleBook("要删的书", "同一作者", "/books/drop.txt", strings.Repeat("b", 64))
	dropRes, err := st.SaveBook(ctx, drop, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook drop: %v", err)
	}
	if err := st.DeleteBook(ctx, dropRes.BookID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}

	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 10})
	if err != nil || authors.Total != 1 {
		t.Fatalf("ListAuthors = %+v, err=%v", authors, err)
	}
	if authors.Items[0].BookCount != 1 {
		t.Errorf("留下的书应让作者作品数保持 1: %+v", authors.Items[0])
	}
	books, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 10})
	if err != nil || books.Total != 1 || books.Items[0].ID != keepRes.BookID {
		t.Errorf("列表应只剩 keep: %+v, err=%v", books, err)
	}
}

// requireBookRowsGone 断言 books 本体与全部子表里都不再有这本书的行。
func requireBookRowsGone(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	for _, table := range append([]string{"books"}, bookCascadeTables...) {
		if n := bookRowCount(t, ctx, st, table, bookID); n != 0 {
			t.Errorf("%s 仍残留 %d 行（book_id=%d）", table, n, bookID)
		}
	}
	if _, err := st.GetBook(ctx, bookID); !errors.Is(err, ErrNotFound) {
		t.Errorf("删后 GetBook err = %v，期望 ErrNotFound", err)
	}
}

// requireAuthorStatsReset 断言作者行保留、聚合列归零。
func requireAuthorStatsReset(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	if authors.Total != 1 {
		t.Fatalf("作者应保留 1 位，实际 %d", authors.Total)
	}
	if got := authors.Items[0]; got.BookCount != 0 || got.WordCount != 0 {
		t.Errorf("作者聚合未刷新: %+v", got)
	}
}

// requireProgressCleared 断言阅读进度被一并清除。
func requireProgressCleared(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	items, err := st.ListReadingProgress(ctx, 10)
	if err != nil {
		t.Fatalf("ListReadingProgress: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("阅读进度未清除: %+v", items)
	}
}

// seedDeletableBook 造一本「带封面 / 标签 / 三章 / 阅读进度」的书，返回 book id。
func seedDeletableBook(t *testing.T, ctx context.Context, st *Store) int64 {
	t.Helper()
	book := sampleBook("待删的书", "作者甲", "/books/deletable.txt", strings.Repeat("c", 64))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if err := st.SaveCover(ctx, res.BookID, "image/png", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	tags := []string{"玄幻"}
	if err := st.UpdateBookMeta(ctx, res.BookID, BookMetaPatch{Tags: &tags}); err != nil {
		t.Fatalf("UpdateBookMeta tags: %v", err)
	}
	var chapterID int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT id FROM chapters WHERE book_id = ? ORDER BY idx LIMIT 1", res.BookID).Scan(&chapterID); err != nil {
		t.Fatalf("读首章 id: %v", err)
	}
	if err := st.SaveReadingProgress(ctx,
		ReadingProgress{BookID: res.BookID, ChapterID: chapterID, Percent: 0.5}); err != nil {
		t.Fatalf("SaveReadingProgress: %v", err)
	}
	// 自检：造出来的样本确实每张子表都有行，否则上面的「清零」断言是空转。
	for _, table := range bookCascadeTables {
		if n := bookRowCount(t, ctx, st, table, res.BookID); n == 0 && table != "chapter_blocks" {
			t.Fatalf("%s 没有行，用例样本不成立", table)
		}
	}
	return res.BookID
}

// bookRowCount 数某张子表里属于某本书的行数。
func bookRowCount(t *testing.T, ctx context.Context, st *Store, table string, bookID int64) int {
	t.Helper()
	where := "book_id = ?"
	if table == "books" {
		where = "id = ?"
	}
	var n int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE "+where, bookID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
