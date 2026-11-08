package store

import (
	"context"
	"strings"
	"testing"
)

// authorIDByName 按作者名查 authors.id（测试助手）。
func authorIDByName(t *testing.T, st *Store, ctx context.Context, name string) int64 {
	t.Helper()
	var id int64
	if err := st.DB().QueryRowContext(ctx, "SELECT id FROM authors WHERE name = ?", name).Scan(&id); err != nil {
		t.Fatalf("查 author id %q: %v", name, err)
	}
	return id
}

// TestUpdateBookMetaKeepsAuthorAgg 修改书籍作者时，author_agg 的旧/新作者
// book_count / word_count 必须同步更新（0022 聚合不变量）。
func TestUpdateBookMetaKeepsAuthorAgg(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	movedID := seedMetaBook(t, st, ctx, "第一本书", "作者甲", "/path/to/agg1.txt", strings.Repeat("a", 64))
	seedMetaBook(t, st, ctx, "第二本书", "作者乙", "/path/to/agg2.txt", strings.Repeat("b", 64))

	oldID := authorIDByName(t, st, ctx, "作者甲")
	newID := authorIDByName(t, st, ctx, "作者乙")

	before := readAuthorAggRows(t, ctx, st)
	if got := before[oldID]; got != [2]int{1, 30} {
		t.Fatalf("before 作者甲 agg = %v, want [1 30]", got)
	}
	if got := before[newID]; got != [2]int{1, 30} {
		t.Fatalf("before 作者乙 agg = %v, want [1 30]", got)
	}

	if err := st.UpdateBookMeta(ctx, movedID, BookMetaPatch{Author: ptr("作者乙")}); err != nil {
		t.Fatalf("UpdateBookMeta: %v", err)
	}

	after := readAuthorAggRows(t, ctx, st)
	// 旧作者回到 0 本 / 0 字；新作者累加到 2 本 / 60 字。
	if got := after[oldID]; got != [2]int{0, 0} {
		t.Fatalf("after 作者甲 agg = %v, want [0 0]", got)
	}
	if got := after[newID]; got != [2]int{2, 60} {
		t.Fatalf("after 作者乙 agg = %v, want [2 60]", got)
	}
	// 本书作者也应落成新作者。
	detail, err := st.GetBook(ctx, movedID)
	if err != nil || detail.AuthorName != "作者乙" {
		t.Fatalf("detail = %+v, err=%v", detail, err)
	}
}
