package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRealDBQueries 只在设置 NORVELS_REAL_DB 时运行，对真实导入库做抽查并测量查询延迟。
//
//	NORVELS_REAL_DB=/tmp/test-norvels.db go test ./internal/store/ -run TestRealDBQueries -v
func TestRealDBQueries(t *testing.T) {
	dbPath := os.Getenv("NORVELS_REAL_DB")
	if dbPath == "" {
		t.Skip("设置 NORVELS_REAL_DB 指向已导入的库再运行")
	}
	ctx := context.Background()
	st, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	var page Page[BookSummary]
	t.Run("统计", func(t *testing.T) { probeStats(t, ctx, st) })
	t.Run("书架首页", func(t *testing.T) { page = probeBookList(t, ctx, st) })
	t.Run("详情与目录", func(t *testing.T) { probeBookDetail(t, ctx, st, page) })
	t.Run("作者列表", func(t *testing.T) { probeAuthors(t, ctx, st) })
	t.Run("搜索延迟", func(t *testing.T) { probeSearchLatency(t, ctx, st) })
}

// probeStats 打出一份库级统计与耗时。
func probeStats(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	start := time.Now()
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	t.Logf("统计：书籍=%d 章节=%d 作者=%d 字数=%d 库=%.2fMB 用时=%v",
		stats.Books, stats.Chapters, stats.Authors, stats.Words,
		float64(stats.DBBytes)/(1<<20), time.Since(start))
}

// probeBookList 抽查书架首页并返回给后续子测试复用。
func probeBookList(t *testing.T, ctx context.Context, st *Store) Page[BookSummary] {
	t.Helper()
	page, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 20, Sort: "updated"})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatalf("ListBooks: 首页为空，无法抽查详情")
	}
	t.Logf("ListBooks：total=%d 首页=%d 本，首本=%q 作者=%q 章节=%d 字数=%d",
		page.Total, len(page.Items), page.Items[0].Title, page.Items[0].AuthorName,
		page.Items[0].ChapterCount, page.Items[0].WordCount)
	return page
}

// probeBookDetail 抽查书籍详情与目录首页。
func probeBookDetail(t *testing.T, ctx context.Context, st *Store, page Page[BookSummary]) {
	t.Helper()
	if len(page.Items) == 0 {
		t.Skip("书架首页为空，无详情可查")
	}
	detail, err := st.GetBook(ctx, page.Items[0].ID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	chPage, err := st.ListChapters(ctx, ChapterQuery{BookID: detail.ID, From: 1, Limit: 50})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	t.Logf("GetBook：%q 卷=%d 章节=%d；目录首页=%d 条",
		detail.Title, len(detail.Volumes), detail.ChapterCount, len(chPage.Items))
}

// probeAuthors 抽查作者列表首位及聚合列。
func probeAuthors(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	if len(authors.Items) == 0 {
		t.Fatalf("ListAuthors: 空列表，无法抽查")
	}
	t.Logf("ListAuthors：total=%d 首位=%q（%d 本 / %d 字）",
		authors.Total, authors.Items[0].Name, authors.Items[0].BookCount, authors.Items[0].WordCount)
}

// probeSearchLatency 对若干关键词各取 3 次，报告书名/作者/章节标题三类命中的耗时。
func probeSearchLatency(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, kw := range []string{"长夜", "青川", "第一章", "风起"} {
		books, chapters, best, avg, worst := measureSearch(t, ctx, st, kw)
		t.Logf("Search(%q)：书名/作者命中=%d 章节标题命中=%d 最好=%v 平均=%v 最差=%v",
			kw, books, chapters, best, avg, worst)
	}
}

// measureSearch 对同一关键词跑 3 次，返回两类命中数与最好/平均/最差耗时。
func measureSearch(t *testing.T, ctx context.Context, st *Store, kw string) (books, chapters int, best, avg, worst time.Duration) {
	t.Helper()
	var total time.Duration
	for i := 0; i < 3; i++ {
		start := time.Now()
		res, err := st.Search(ctx, SearchQuery{Q: kw, Page: 1, Size: 20})
		d := time.Since(start)
		if err != nil {
			t.Fatalf("Search(%q): %v", kw, err)
		}
		books, chapters = res.Books.Total, res.Chapters.Total
		if i == 0 || d < best {
			best = d
		}
		if i == 0 || d > worst {
			worst = d
		}
		total += d
	}
	return books, chapters, best, total / 3, worst
}
