package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// searchFixture 建库并写入两本书：b1 的书名/作者/章节标题可被检索，b2 带一段 script 标签。
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
	t.Run("书名命中", func(t *testing.T) { checkBookTitleHit(t, ctx, st) })
	t.Run("作者命中", func(t *testing.T) { checkAuthorHit(t, ctx, st) })
	t.Run("章节标题命中", func(t *testing.T) { checkChapterTitleHit(t, ctx, st) })
	t.Run("单字关键词返回提示", func(t *testing.T) { checkSingleRuneSearch(t, ctx, st) })
	t.Run("type=book 只查书名", func(t *testing.T) { checkBookTypeSearch(t, ctx, st) })
	t.Run("限定单本书", func(t *testing.T) { checkScopedSearch(t, ctx, st) })
	t.Run("正文不再参与检索", func(t *testing.T) { checkContentNotSearchable(t, ctx, st) })
}

// checkBookTitleHit 书名命中：books 侧返回该本，章节侧不因书名而命中。
func checkBookTitleHit(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "梅宁风云"})
	if r.Books.Total != 1 || r.Books.Items[0].Title != "梅宁风云" {
		t.Fatalf("书名命中 = %+v", r.Books)
	}
	if r.Books.Page != 1 || r.Books.Size != 20 {
		t.Errorf("search page = %d/%d", r.Books.Page, r.Books.Size)
	}
}

// checkAuthorHit 作者命中：按作者名同样返回该本书。
func checkAuthorHit(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "月关"})
	if r.Books.Total != 1 || r.Books.Items[0].Title != "风雪归人" {
		t.Fatalf("作者命中 = %+v", r.Books)
	}
}

// checkChapterTitleHit 章节标题命中：返回书籍标题与章节序号，Snippet 恒为空。
func checkChapterTitleHit(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "第一章"})
	if r.Chapters.Total != 1 || len(r.Chapters.Items) != 1 {
		t.Fatalf("章节标题命中 = %+v", r.Chapters)
	}
	hit := r.Chapters.Items[0]
	if hit.ChapterTitle != "第一章 风起" || hit.BookTitle != "梅宁风云" || hit.ChapterIdx != 0 {
		t.Errorf("命中内容 = %+v", hit)
	}
	// 正文检索下线后后端不再算片段：字段保留但必须为空串。
	if hit.Snippet != "" {
		t.Errorf("Snippet 应为空串（正文检索已下线），实际 %q", hit.Snippet)
	}
	if hit.Score != 0 {
		t.Errorf("Score 应为 0（没有 bm25 排序），实际 %v", hit.Score)
	}
}

// checkSingleRuneSearch 1 字关键词：空结果 + 提示，不做全表 LIKE。
func checkSingleRuneSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "梅"})
	if r.Chapters.Total != 0 || r.Books.Total != 0 || r.Notice == "" {
		t.Errorf("1字查询应返回空结果+提示: %+v", r)
	}
}

// checkBookTypeSearch type=book 只查书名（LIKE），不返回章节命中。
func checkBookTypeSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "梅宁风云", Type: "book"})
	if r.Books.Total != 1 || r.Books.Items[0].Title != "梅宁风云" {
		t.Errorf("book 检索 = %+v", r.Books)
	}
	if len(r.Chapters.Items) != 0 {
		t.Errorf("type=book 不应返回章节: %+v", r.Chapters)
	}
}

// checkScopedSearch BookID 非 0 时只在单本书内检索。
func checkScopedSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	found, err := st.ListBooks(ctx, BookQuery{Q: "风雪归人", Page: 1, Size: 1})
	if err != nil || len(found.Items) != 1 {
		t.Fatalf("find b2: %+v, err=%v", found, err)
	}
	// b1 的章节标题不含「唯一」，b2 的章节标题「唯一的章」含它。
	r := mustSearch(t, ctx, st, SearchQuery{Q: "唯一", BookID: found.Items[0].ID})
	if r.Chapters.Total != 1 || r.Chapters.Items[0].BookID != found.Items[0].ID {
		t.Errorf("book 内检索 = %+v", r.Chapters)
	}
}

// checkContentNotSearchable 正文里的词不再被检索到（检索范围只到章节标题）。
func checkContentNotSearchable(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	// 「秋蝉」只出现在 b2 的正文里，不在任何书名/作者/章节标题中。
	r := mustSearch(t, ctx, st, SearchQuery{Q: "秋蝉"})
	if r.Chapters.Total != 0 || r.Books.Total != 0 {
		t.Errorf("正文关键词不应命中: chapters=%+v books=%+v", r.Chapters, r.Books)
	}
	if r.Notice != "" {
		t.Errorf("正常关键词不应带提示，实际 %q", r.Notice)
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
