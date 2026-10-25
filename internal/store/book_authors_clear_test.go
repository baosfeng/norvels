package store

import (
	"context"
	"testing"
)

// TestClearBookAuthors 清空作者：关联行删除、author_id 置 0、旧作者统计归零。
func TestClearBookAuthors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b := sampleBook("清作者之书", "原作者", "/books/clear-authors.txt", strings64("a"))
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	requireAuthorBookCount(t, ctx, st, "原作者", 1)

	empty := []string{}
	if err := st.UpdateBookMeta(ctx, res.BookID, BookMetaPatch{Authors: &empty}); err != nil {
		t.Fatalf("清空作者: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if len(detail.Authors) != 0 {
		t.Errorf("清空后 authors = %v，期望空数组", detail.Authors)
	}
	if detail.AuthorName != "" {
		t.Errorf("清空后 authorName = %q，期望空串", detail.AuthorName)
	}
	var authorID int64
	if err := st.DB().QueryRowContext(ctx, "SELECT author_id FROM books WHERE id = ?", res.BookID).Scan(&authorID); err != nil {
		t.Fatalf("读 author_id: %v", err)
	}
	if authorID != 0 {
		t.Errorf("清空后 books.author_id = %d，期望 0", authorID)
	}
	var rows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM books_authors WHERE book_id = ?", res.BookID).Scan(&rows); err != nil {
		t.Fatalf("读关联行: %v", err)
	}
	if rows != 0 {
		t.Errorf("清空后 books_authors 仍有 %d 行", rows)
	}
	requireAuthorBookCount(t, ctx, st, "原作者", 0)
}

// requireAuthorBookCount 断言某作者的聚合作品数。
func requireAuthorBookCount(t *testing.T, ctx context.Context, st *Store, name string, want int) {
	t.Helper()
	var count int
	if err := st.DB().QueryRowContext(ctx, "SELECT book_count FROM authors WHERE name = ?", name).Scan(&count); err != nil {
		t.Fatalf("读作者 %q 统计: %v", name, err)
	}
	if count != want {
		t.Errorf("作者 %q book_count = %d，期望 %d", name, count, want)
	}
}

// strings64 造一个长度 64 的合成哈希（每本书唯一）。
func strings64(s string) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}
