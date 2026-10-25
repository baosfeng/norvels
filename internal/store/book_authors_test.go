package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// bookAuthorRows 返回某本书的「作者名 + position」（按 position 升序）。
func bookAuthorRows(t *testing.T, st *Store, ctx context.Context, bookID int64) ([]string, []int) {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT a.name, ba.position FROM books_authors ba JOIN authors a ON a.id = ba.author_id "+
			"WHERE ba.book_id = ? ORDER BY ba.position", bookID)
	if err != nil {
		t.Fatalf("查 books_authors: %v", err)
	}
	defer rows.Close()

	var names []string
	var positions []int
	for rows.Next() {
		var (
			name string
			pos  int
		)
		if err := rows.Scan(&name, &pos); err != nil {
			t.Fatalf("扫描 books_authors: %v", err)
		}
		names = append(names, name)
		positions = append(positions, pos)
	}
	return names, positions
}

// primaryAuthorName 返回 books.author_id 对应的作者名。
func primaryAuthorName(t *testing.T, st *Store, ctx context.Context, bookID int64) string {
	t.Helper()
	var name string
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COALESCE(a.name, '') FROM books b LEFT JOIN authors a ON a.id = b.author_id WHERE b.id = ?",
		bookID).Scan(&name); err != nil {
		t.Fatalf("查主作者: %v", err)
	}
	return name
}

// TestSaveBookWritesMultipleAuthors 多作者写入：position 保序，books.author_id = position 0。
func TestSaveBookWritesMultipleAuthors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("多作者示例书", "示例作者", "/path/to/multi.epub", strings.Repeat("m", 64))
	b.Authors = []string{"作者乙", "作者甲", "作者丙"}
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	names, positions := bookAuthorRows(t, st, ctx, res.BookID)
	if !reflect.DeepEqual(names, []string{"作者乙", "作者甲", "作者丙"}) {
		t.Fatalf("books_authors = %v", names)
	}
	if !reflect.DeepEqual(positions, []int{0, 1, 2}) {
		t.Fatalf("position = %v", positions)
	}
	if got := primaryAuthorName(t, st, ctx, res.BookID); got != "作者乙" {
		t.Fatalf("books.author_id 作者 = %q，期望 position 0 的「作者乙」", got)
	}

	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !reflect.DeepEqual(detail.Authors, []string{"作者乙", "作者甲", "作者丙"}) {
		t.Fatalf("GetBook.Authors = %v", detail.Authors)
	}
}

// TestSaveBookAuthorsNormalized 作者名去空白、丢空名、保序去重。
func TestSaveBookAuthorsNormalized(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("规范化示例书", "示例作者", "/path/to/norm.epub", strings.Repeat("n", 64))
	b.Authors = []string{" 作者甲 ", "", "作者甲", "作者乙"}
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	names, _ := bookAuthorRows(t, st, ctx, res.BookID)
	if !reflect.DeepEqual(names, []string{"作者甲", "作者乙"}) {
		t.Fatalf("books_authors = %v，期望去重去空后两项", names)
	}
}

// TestSaveBookAuthorsFallbackForLegacy 存量书（无 books_authors 行）按主作者兜底。
func TestSaveBookAuthorsFallbackForLegacy(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	res, err := st.SaveBook(ctx, sampleBook("存量示例书", "旧作者", "/path/to/legacy.txt", strings.Repeat("o", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	// 模拟 0009 之前入库的书：关联行为空。
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM books_authors WHERE book_id = ?", res.BookID); err != nil {
		t.Fatalf("清理关联行: %v", err)
	}

	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !reflect.DeepEqual(detail.Authors, []string{"旧作者"}) {
		t.Fatalf("兜底作者 = %v，期望 [旧作者]", detail.Authors)
	}
}

// TestSaveBookAuthorsFallbackUnknownAuthor 没有作者的书兜底成「佚名」，不出空数组。
func TestSaveBookAuthorsFallbackUnknownAuthor(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("无名示例书", "", "/path/to/nobody2.txt", strings.Repeat("p", 64))
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !reflect.DeepEqual(detail.Authors, []string{unknownAuthor}) {
		t.Fatalf("兜底作者 = %v，期望 [%s]", detail.Authors, unknownAuthor)
	}
}

// TestSaveBookForceReplacesAuthors 覆盖导入要全量换作者，不留孤儿关联行。
func TestSaveBookForceReplacesAuthors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("覆盖示例书", "作者甲", "/path/to/force.epub", strings.Repeat("q", 64))
	b.Authors = []string{"作者甲", "作者乙"}
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	repl := sampleBook("覆盖示例书", "作者丙", "/path/to/force.epub", strings.Repeat("r", 64))
	repl.Authors = []string{"作者丙"}
	if _, err := st.SaveBook(ctx, repl, SaveOptions{Force: true}); err != nil {
		t.Fatalf("覆盖导入: %v", err)
	}
	names, _ := bookAuthorRows(t, st, ctx, res.BookID)
	if !reflect.DeepEqual(names, []string{"作者丙"}) {
		t.Fatalf("覆盖后 books_authors = %v，期望只剩 [作者丙]", names)
	}
	if got := primaryAuthorName(t, st, ctx, res.BookID); got != "作者丙" {
		t.Fatalf("覆盖后主作者 = %q", got)
	}
}

// TestFindByTitleAuthor 三级匹配的第三级：按书名 + 主作者找书。
func TestFindByTitleAuthor(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	res, err := st.SaveBook(ctx, sampleBook("查找示例书", "查找示例作者", "/path/to/find.txt", strings.Repeat("s", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	id, ok, err := st.FindByTitleAuthor(ctx, "查找示例书", "查找示例作者")
	if err != nil || !ok || id != res.BookID {
		t.Fatalf("FindByTitleAuthor = (%d, %v, %v)，期望 (%d, true, nil)", id, ok, err, res.BookID)
	}
	if _, ok, err := st.FindByTitleAuthor(ctx, "没有这本书", "查找示例作者"); err != nil || ok {
		t.Fatalf("未命中应为 (false, nil)，实际 ok=%v err=%v", ok, err)
	}
	if _, _, err := st.FindByTitleAuthor(ctx, "  ", "查找示例作者"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空书名的错误 = %v，期望 ErrInvalidArgument", err)
	}
}

// TestAuthorAggregatesIncludeCoAuthors 副作者（books_authors 里非首位）也要进
// 作者页、作者筛选与作品列表：否则详情页看得到「作者甲」，作者页却查不到他的书。
func TestAuthorAggregatesIncludeCoAuthors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	solo := sampleBook("独立作品", "作者甲", "/path/to/agg-a.txt", strings.Repeat("a", 64))
	if _, err := st.SaveBook(ctx, solo, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook solo: %v", err)
	}
	co := sampleBook("合著作品", "作者乙", "/path/to/agg-b.txt", strings.Repeat("b", 64))
	co.Authors = []string{"作者乙", "作者甲"}
	if _, err := st.SaveBook(ctx, co, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook co: %v", err)
	}

	page, err := st.ListAuthors(ctx, AuthorQuery{Sort: "books"})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	byName := make(map[string]AuthorSummary, len(page.Items))
	for _, a := range page.Items {
		byName[a.Name] = a
	}
	if byName["作者甲"].BookCount != 2 {
		t.Fatalf("作者甲 book_count = %d，期望 2（独立 + 合著）", byName["作者甲"].BookCount)
	}
	if byName["作者乙"].BookCount != 1 {
		t.Fatalf("作者乙 book_count = %d，期望 1", byName["作者乙"].BookCount)
	}

	detail, err := st.GetAuthor(ctx, byName["作者甲"].ID)
	if err != nil {
		t.Fatalf("GetAuthor: %v", err)
	}
	if len(detail.Books) != 2 {
		t.Fatalf("作者甲作品数 = %d，期望 2", len(detail.Books))
	}

	filtered, err := st.ListBooks(ctx, BookQuery{AuthorID: byName["作者甲"].ID})
	if err != nil {
		t.Fatalf("ListBooks by author: %v", err)
	}
	if filtered.Total != 2 {
		t.Fatalf("按作者甲筛选 = %d 本，期望 2", filtered.Total)
	}
}
