package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"norvels/internal/model"
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

func TestSearchTotalCapped(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	saveBigBook(t, ctx, st, 1200)

	t.Run("首页截断并提示", func(t *testing.T) { checkCappedFirstPage(t, ctx, st) })
	t.Run("截断不影响深分页", func(t *testing.T) { checkDeepPaging(t, ctx, st) })
	t.Run("少量命中不截断", func(t *testing.T) { checkRareHitNoCap(t, ctx, st) })
}

// checkCappedFirstPage 高频词 total 截断到 searchTotalCap 并写 Notice。
func checkCappedFirstPage(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "临川市", Page: 1, Size: 20})
	if r.Chapters.Total != searchTotalCap {
		t.Errorf("total = %d, want %d", r.Chapters.Total, searchTotalCap)
	}
	if r.Notice != searchTotalCapNotice {
		t.Errorf("notice = %q, want %q", r.Notice, searchTotalCapNotice)
	}
	if len(r.Chapters.Items) != 20 {
		t.Errorf("第一页条数 = %d, want 20", len(r.Chapters.Items))
	}
}

// checkDeepPaging 截断不影响分页数据：第 30 页仍能取满。
func checkDeepPaging(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	deep := mustSearch(t, ctx, st, SearchQuery{Q: "临川市", Page: 30, Size: 20})
	if len(deep.Chapters.Items) != 20 {
		t.Errorf("第 30 页条数 = %d, want 20", len(deep.Chapters.Items))
	}
}

// checkRareHitNoCap 少量命中：不截断、无提示。
func checkRareHitNoCap(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	few := mustSearch(t, ctx, st, SearchQuery{Q: "暗号"})
	if few.Chapters.Total != 1 || few.Notice != "" {
		t.Errorf("稀有词 total=%d notice=%q", few.Chapters.Total, few.Notice)
	}
}

// saveBigBook 写入一本含 n 章的书，每章都含「临川市」，第 1 章额外含「暗号」。
func saveBigBook(t *testing.T, ctx context.Context, st *Store, n int) {
	t.Helper()
	book := &model.ParsedBook{
		Title:      "大书",
		Author:     "作者",
		SourcePath: "/books/big.txt",
		SourceName: "big.txt",
		SHA256:     strings.Repeat("7", 64),
		WordCount:  n * 20,
	}
	for i := 0; i < n; i++ {
		content := fmt.Sprintf("这是第%d章的正文，临川市的故事还在继续。", i+1)
		if i == 0 {
			content += " 这里藏着一个独一无二的暗号。"
		}
		book.Chapters = append(book.Chapters, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: -1,
			Title:     fmt.Sprintf("第%d章", i+1),
			Content:   content,
			WordCount: 20,
		})
	}
	if _, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
}
