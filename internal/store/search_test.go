package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// searchFixture 建库并写入两本书：b1 的书名可被检索（章节标题含「第一章 风起」），
// b2 的正文里带一段 script 标签。
func searchFixture(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)

	b1 := sampleBook("梅宁风云", "示例作者", "/books/a.txt", strings.Repeat("d", 64))
	if _, err := st.SaveBook(ctx, b1, SaveOptions{}); err != nil {
		t.Fatalf("save b1: %v", err)
	}
	b2 := sampleBook("风雪归人", "青川", "/books/b.txt", strings.Repeat("e", 64))
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
	t.Run("作者名不再命中", func(t *testing.T) { checkAuthorNameNotSearchable(t, ctx, st) })
	t.Run("章节标题不再命中", func(t *testing.T) { checkChapterTitleNotSearchable(t, ctx, st) })
	t.Run("正文不再参与检索", func(t *testing.T) { checkContentNotSearchable(t, ctx, st) })
	t.Run("单字关键词返回提示", func(t *testing.T) { checkSingleRuneSearch(t, ctx, st) })
	t.Run("分页回填", func(t *testing.T) { checkSearchPage(t, ctx, st) })
}

// checkBookTitleHit 书名命中：返回该本书，并回填分页字段。
func checkBookTitleHit(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "梅宁风云"})
	if r.Books.Total != 1 || r.Books.Items[0].Title != "梅宁风云" {
		t.Fatalf("书名命中 = %+v", r.Books)
	}
	if r.Books.Items[0].AuthorName != "示例作者" {
		t.Errorf("作者名 = %q，期望 示例作者", r.Books.Items[0].AuthorName)
	}
}

// checkAuthorNameNotSearchable 作者名不再参与匹配（检索范围只有书名）：
// 「青川」只出现在 b2 的作者字段里，按它搜不到任何书；但同一本书按书名查得到，
// 且结果里仍带得出作者名（authors 表在 loadBooks 里只为取这一列）。
func checkAuthorNameNotSearchable(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "青川"})
	if r.Books.Total != 0 {
		t.Errorf("按作者名搜索不应命中: %+v", r.Books)
	}
	if r.Notice != "" {
		t.Errorf("正常关键词不应带提示，实际 %q", r.Notice)
	}
	byTitle := mustSearch(t, ctx, st, SearchQuery{Q: "风雪归人"})
	if byTitle.Books.Total != 1 || byTitle.Books.Items[0].AuthorName != "青川" {
		t.Errorf("按书名仍应命中并带出作者名: %+v", byTitle.Books)
	}
}

// checkChapterTitleNotSearchable 章节标题（目录）不再参与匹配：
// b1 的章节标题是「第一章 风起」，按它搜不到任何书，也不该带提示语。
func checkChapterTitleNotSearchable(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, kw := range []string{"第一章", "风起", "唯一的章"} {
		r := mustSearch(t, ctx, st, SearchQuery{Q: kw})
		if r.Books.Total != 0 {
			t.Errorf("按章节标题 %q 搜索不应命中任何书: %+v", kw, r.Books)
		}
		if r.Notice != "" {
			t.Errorf("正常关键词 %q 不应带提示，实际 %q", kw, r.Notice)
		}
	}
}

// checkContentNotSearchable 正文里的词也不参与检索（检索只到书名）。
func checkContentNotSearchable(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	// 「秋蝉」只出现在 b2 的正文里，不在任何书名/作者中。
	r := mustSearch(t, ctx, st, SearchQuery{Q: "秋蝉"})
	if r.Books.Total != 0 {
		t.Errorf("正文关键词不应命中: %+v", r.Books)
	}
	if r.Notice != "" {
		t.Errorf("正常关键词不应带提示，实际 %q", r.Notice)
	}
}

// checkSingleRuneSearch 1 字关键词：空结果 + 提示，不做全表 LIKE。
func checkSingleRuneSearch(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "梅"})
	if r.Books.Total != 0 || r.Notice == "" {
		t.Errorf("1字查询应返回空结果+提示: %+v", r)
	}
}

// checkSearchPage 分页口径：请求的 page/size 原样回填，空结果 items 也是 []。
func checkSearchPage(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	r := mustSearch(t, ctx, st, SearchQuery{Q: "梅宁风云", Page: 1, Size: 5})
	if r.Books.Page != 1 || r.Books.Size != 5 {
		t.Errorf("分页回填 = %d/%d，期望 1/5", r.Books.Page, r.Books.Size)
	}
	empty := mustSearch(t, ctx, st, SearchQuery{Q: "不存在之书"})
	if empty.Books.Items == nil {
		t.Error("空结果的 items 应为 []，实际 nil")
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
