package store

import (
	"context"
	"testing"
)

// 分页 total 的口径：与「带 authors LEFT JOIN 的 COUNT」逐字相同。
//
// countBooks 一律不 JOIN authors（那个 JOIN 只给每行多做一次主键查找，total 不读作者名），
// 这组测试守住的正是「去掉 JOIN 后 total 不变」——改坏了会直接表现为分页总数漂移。

// joinedTotal 用带 LEFT JOIN 的 SQL 直接数一遍，作为对照口径。
func joinedTotal(t *testing.T, st *Store, where string, args ...any) int {
	t.Helper()
	q := "SELECT COUNT(*) FROM books b LEFT JOIN authors a ON a.id = b.author_id WHERE " + where
	var n int
	if err := st.DB().QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("对照计数: %v", err)
	}
	return n
}

// seedCountBooks 插 3 本书：两本归第一个作者，一本无作者。
func seedCountBooks(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.DB().ExecContext(ctx, "INSERT INTO authors(name) VALUES ('甲作者')"); err != nil {
		t.Fatalf("插入作者: %v", err)
	}
	var authorID int64
	if err := st.DB().QueryRowContext(ctx, "SELECT MAX(id) FROM authors").Scan(&authorID); err != nil {
		t.Fatalf("取作者 id: %v", err)
	}
	for i, row := range []struct {
		title  string
		author int64
	}{{"甲之一", authorID}, {"甲之二", authorID}, {"无名之书", 0}} {
		if _, err := st.DB().ExecContext(ctx,
			"INSERT INTO books(title, author_id, source_path, file_sha256) VALUES (?, ?, ?, ?)",
			row.title, row.author, "/path/to/txt/"+string(rune('a'+i))+".txt", "sha-"+string(rune('a'+i))); err != nil {
			t.Fatalf("插入 %s: %v", row.title, err)
		}
	}
}

// TestListBooksTotalEqualsJoinedCount 无关键词时 total 与带 JOIN 的计数一致。
func TestListBooksTotalEqualsJoinedCount(t *testing.T) {
	st := newTestStore(t)
	seedCountBooks(t, st)
	page, err := st.ListBooks(context.Background(), BookQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	want := joinedTotal(t, st, "b.superseded_by = 0")
	if page.Total != want {
		t.Errorf("total = %d，带 JOIN 的对照口径 = %d", page.Total, want)
	}
	if page.Total != 3 {
		t.Errorf("total = %d，期望 3", page.Total)
	}
}

// TestListBooksTotalWithKeyword 有关键词时只按书名匹配，total 与 JOIN 口径一致；
// 作者名不再参与 WHERE——同名作者的书不会因为作者命中而被数进来。
func TestListBooksTotalWithKeyword(t *testing.T) {
	st := newTestStore(t)
	seedCountBooks(t, st)
	page, err := st.ListBooks(context.Background(), BookQuery{Q: "甲", Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks(q=甲): %v", err)
	}
	want := joinedTotal(t, st, "b.superseded_by = 0 AND b.title LIKE ?", "%甲%")
	if page.Total != want {
		t.Errorf("total = %d，带 JOIN 的对照口径 = %d", page.Total, want)
	}
	if page.Total != 2 {
		t.Errorf("total = %d，期望 2（书名带「甲」的两本，不含无作者的第三本）", page.Total)
	}
}

// TestListBooksTotalIgnoresAuthorName 关键词只出现在作者名里时一本书都不算命中。
func TestListBooksTotalIgnoresAuthorName(t *testing.T) {
	st := newTestStore(t)
	seedCountBooks(t, st)
	page, err := st.ListBooks(context.Background(), BookQuery{Q: "作者", Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks(q=作者): %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("按作者名匹配到 %d 本（%v），期望 0", page.Total, page.Items)
	}
}
