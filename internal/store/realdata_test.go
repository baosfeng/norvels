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

	start := time.Now()
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	t.Logf("统计：书籍=%d 章节=%d 作者=%d 字数=%d 库=%.2fMB 用时=%v",
		stats.Books, stats.Chapters, stats.Authors, stats.Words,
		float64(stats.DBBytes)/(1<<20), time.Since(start))

	// 书架首页。
	page, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 20, Sort: "updated"})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	t.Logf("ListBooks：total=%d 首页=%d 本，首本=%q 作者=%q 章节=%d 字数=%d",
		page.Total, len(page.Items), page.Items[0].Title, page.Items[0].AuthorName,
		page.Items[0].ChapterCount, page.Items[0].WordCount)

	// 详情 + 目录。
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

	// 作者列表。
	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListAuthors: %v", err)
	}
	t.Logf("ListAuthors：total=%d 首位=%q（%d 本 / %d 字）",
		authors.Total, authors.Items[0].Name, authors.Items[0].BookCount, authors.Items[0].WordCount)

	// 搜索延迟（每个关键词取 3 次中位数）。
	keywords := []string{"梅宁", "他的", "第一章", "风起"}
	for _, kw := range keywords {
		var best, worst, total time.Duration
		hits := 0
		for i := 0; i < 3; i++ {
			s := time.Now()
			res, err := st.Search(ctx, SearchQuery{Q: kw, Page: 1, Size: 20})
			d := time.Since(s)
			if err != nil {
				t.Fatalf("Search(%q): %v", kw, err)
			}
			hits = res.Chapters.Total
			if i == 0 || d < best {
				best = d
			}
			if i == 0 || d > worst {
				worst = d
			}
			total += d
			if i == 0 && len(res.Chapters.Items) > 0 {
				snip := []rune(res.Chapters.Items[0].Snippet)
				if len(snip) > 60 {
					snip = snip[:60]
				}
				t.Logf("  样例：%q / %q … %q", res.Chapters.Items[0].BookTitle, res.Chapters.Items[0].ChapterTitle, string(snip))
			}
		}
		t.Logf("Search(%q)：命中=%d 最好=%v 平均=%v 最差=%v", kw, hits, best, total/3, worst)
	}
}
