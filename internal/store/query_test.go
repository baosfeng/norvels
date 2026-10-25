package store

import (
	"context"
	"strings"
	"testing"
)

func TestListBooksFilterAndPaging(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedThreeBooks(t, ctx, st)

	t.Run("分页不重叠", func(t *testing.T) { checkPagingNoOverlap(t, ctx, st) })
	t.Run("按作者名模糊搜", func(t *testing.T) { checkAuthorFilter(t, ctx, st) })
	t.Run("标签过滤", func(t *testing.T) { checkTagFilter(t, ctx, st) })
}

// checkPagingNoOverlap 第 1/2 页条数正确且两页 id 不重复。
func checkPagingNoOverlap(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	p1, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 2, Sort: "title"})
	if err != nil || p1.Total != 3 || len(p1.Items) != 2 {
		t.Fatalf("page1 = %+v, err=%v", p1, err)
	}
	p2, err := st.ListBooks(ctx, BookQuery{Page: 2, Size: 2, Sort: "title"})
	if err != nil || len(p2.Items) != 1 {
		t.Fatalf("page2 = %+v, err=%v", p2, err)
	}
	if p1.Items[0].ID == p2.Items[0].ID {
		t.Errorf("分页重复: %d", p1.Items[0].ID)
	}
}

// checkAuthorFilter 书名/作者模糊匹配应只命中该作者的书。
func checkAuthorFilter(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	q, err := st.ListBooks(ctx, BookQuery{Q: "作者甲", Page: 1, Size: 10})
	if err != nil || q.Total != 1 {
		t.Fatalf("author filter = %+v, err=%v", q, err)
	}
}

// checkTagFilter 标签过滤只返回打标书，ListTags 的聚合计数同步。
func checkTagFilter(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	bookID := seedTaggedBook(t, ctx, st)
	tagged, err := st.ListBooks(ctx, BookQuery{Tag: "武侠", Page: 1, Size: 10})
	if err != nil || tagged.Total != 1 || tagged.Items[0].ID != bookID {
		t.Fatalf("tag filter = %+v, err=%v", tagged, err)
	}
	tags, err := st.ListTags(ctx)
	if err != nil || len(tags) != 1 || tags[0].BookCount != 1 {
		t.Fatalf("ListTags = %+v, err=%v", tags, err)
	}
}

// seedThreeBooks 写入三本不同作者的书（书名/路径/哈希互不相同）。
func seedThreeBooks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	authorNames := []string{"作者甲", "作者乙", "作者丙"}
	for i := 0; i < 3; i++ {
		b := sampleBook("书"+string(rune('A'+i)), authorNames[i], "/books/"+string(rune('a'+i))+".txt",
			strings.Repeat(string(rune('a'+i)), 64))
		if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
}

// seedTaggedBook 给「作者甲」的书打上「武侠」标签，返回书籍 id。
func seedTaggedBook(t *testing.T, ctx context.Context, st *Store) int64 {
	t.Helper()
	q, err := st.ListBooks(ctx, BookQuery{Q: "作者甲", Page: 1, Size: 10})
	if err != nil || q.Total != 1 {
		t.Fatalf("find book: %+v, err=%v", q, err)
	}
	bookID := q.Items[0].ID

	var tagID int64
	if err := st.DB().QueryRowContext(ctx, "INSERT INTO tags(name) VALUES ('武侠') RETURNING id").Scan(&tagID); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "INSERT INTO book_tags(book_id, tag_id) VALUES (?, ?)", bookID, tagID); err != nil {
		t.Fatalf("insert book_tag: %v", err)
	}
	return bookID
}
