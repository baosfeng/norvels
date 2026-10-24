package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// searchFixture 建库并写入两本书：b1 含「临川市」，b2 含「秋蝉」与一段 script 标签。
func searchFixture(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)

	b1 := sampleBook("梅宁风云", "示例作者", "/books/a.txt", strings.Repeat("d", 64))
	if _, err := st.SaveBook(ctx, b1, SaveOptions{}); err != nil {
		t.Fatalf("save b1: %v", err)
	}
	b2 := sampleBook("风雪归人", "月关", "/books/b.txt", strings.Repeat("e", 64))
	b2.Chapters = []model.ParsedChapter{
		{Idx: 0, VolumeIdx: -1, Title: "唯一的章", Content: "梅宁城里下着雪，<script>alert(1)</script> 他笑了。秋蝉在叫。", WordCount: 20},
	}
	if _, err := st.SaveBook(ctx, b2, SaveOptions{}); err != nil {
		t.Fatalf("save b2: %v", err)
	}
	return st, ctx
}

func TestSearch(t *testing.T) {
	st, ctx := searchFixture(t)
	t.Run("多字关键词命中并高亮", func(t *testing.T) { checkMultiRuneSearch(t, ctx, st) })
	t.Run("两字关键词与 snippet 转义", func(t *testing.T) { checkTwoRuneSearch(t, ctx, st) })
	t.Run("单字关键词返回提示", func(t *testing.T) { checkSingleRuneSearch(t, ctx, st) })
	t.Run("type=book 只查书名", func(t *testing.T) { checkBookTypeSearch(t, ctx, st) })
	t.Run("限定单本书", func(t *testing.T) { checkScopedSearch(t, ctx, st) })
}

// checkMultiRuneSearch 3 字关键词命中、片段高亮、分页字段正确。
func checkMultiRuneSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "临川市"})
	if r.Chapters.Total != 1 || len(r.Chapters.Items) != 1 {
		t.Fatalf("3字命中 = %+v", r.Chapters)
	}
	if !strings.Contains(r.Chapters.Items[0].Snippet, "<b>临川市</b>") {
		t.Errorf("snippet 无高亮: %q", r.Chapters.Items[0].Snippet)
	}
	if r.Chapters.Items[0].BookTitle != "梅宁风云" {
		t.Errorf("bookTitle = %q", r.Chapters.Items[0].BookTitle)
	}
	if r.Chapters.Page != 1 || r.Chapters.Size != 20 {
		t.Errorf("search page = %d/%d", r.Chapters.Page, r.Chapters.Size)
	}
}

// checkTwoRuneSearch 2 字关键词命中，且 snippet 里除 <b> 外不得有未转义标签。
func checkTwoRuneSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r2 := mustSearch(t, ctx, st, SearchQuery{Q: "秋蝉"})
	if r2.Chapters.Total != 1 {
		t.Fatalf("2字命中 = %+v", r2.Chapters)
	}
	if !strings.Contains(r2.Chapters.Items[0].Snippet, "<b>秋蝉</b>") {
		t.Errorf("2字 snippet = %q", r2.Chapters.Items[0].Snippet)
	}
	snip := r2.Chapters.Items[0].Snippet
	if strings.Contains(snip, "<script>") {
		t.Errorf("snippet 未转义: %q", snip)
	}
	if !strings.Contains(snip, "&lt;script&gt;") {
		t.Logf("snippet（窗口可能未覆盖 script）: %q", snip)
	}
}

// checkSingleRuneSearch 1 字关键词：空结果 + 提示，不做全表 LIKE。
func checkSingleRuneSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r3 := mustSearch(t, ctx, st, SearchQuery{Q: "梅"})
	if r3.Chapters.Total != 0 || r3.Books.Total != 0 || r3.Notice == "" {
		t.Errorf("1字查询应返回空结果+提示: %+v", r3)
	}
}

// checkBookTypeSearch type=book 只查书名（LIKE），不返回章节命中。
func checkBookTypeSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r4 := mustSearch(t, ctx, st, SearchQuery{Q: "梅宁风云", Type: "book"})
	if r4.Books.Total != 1 || r4.Books.Items[0].Title != "梅宁风云" {
		t.Errorf("book 检索 = %+v", r4.Books)
	}
	if len(r4.Chapters.Items) != 0 {
		t.Errorf("type=book 不应返回章节: %+v", r4.Chapters)
	}
}

// checkScopedSearch BookID 非 0 时只在单本书内检索。
func checkScopedSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	id2, err := st.ListBooks(ctx, BookQuery{Q: "风雪归人", Page: 1, Size: 1})
	if err != nil || len(id2.Items) != 1 {
		t.Fatalf("find b2: %+v, err=%v", id2, err)
	}
	r5 := mustSearch(t, ctx, st, SearchQuery{Q: "梅宁", BookID: id2.Items[0].ID})
	if r5.Chapters.Total != 1 || r5.Chapters.Items[0].BookID != id2.Items[0].ID {
		t.Errorf("book 内检索 = %+v", r5.Chapters)
	}
}

// mustSearch 跑一次搜索并在出错时终止子测试。
func mustSearch(t *testing.T, ctx context.Context, st *Store, q SearchQuery) *SearchResult {
	t.Helper()
	r, err := st.Search(ctx, q)
	if err != nil {
		t.Fatalf("search %+v: %v", q, err)
	}
	return r
}
