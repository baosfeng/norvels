package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), filepath.Join(t.TempDir(), "norvels.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st
}

func sampleBook(title, author, path, sha string) *model.ParsedBook {
	return &model.ParsedBook{
		Title:      title,
		Author:     author,
		SourcePath: path,
		SourceName: filepath.Base(path),
		Encoding:   "utf-8",
		FileSize:   1024,
		SHA256:     sha,
		WordCount:  30,
		Intro:      "简介：" + title,
		Volumes: []model.ParsedVolume{
			{Idx: 0, Title: "第一卷"},
			{Idx: 1, Title: "第二卷"},
		},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "第一章 风起", Content: "华夏国临川市，午后的秋日，微风拂过城楼。", WordCount: 10},
			{Idx: 1, VolumeIdx: 0, Title: "第二章 云涌", Content: "他站在城楼上，望着远方的梅宁城与风雪。", WordCount: 10},
			{Idx: 2, VolumeIdx: -1, Title: "番外 小记", Content: "番外正文，没有归入任何卷。", WordCount: 10},
		},
	}
}

func TestSegmentRoundTrip(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"华夏国，午后", "华 夏 国 ，午 后 "},
		{"abc 123", "abc 123"},
		{"临川市", "临 川 市 "},
		{"", ""},
		{"a梅b", "a梅 b"},
		{"，。！", "，。！"},
	}
	for _, c := range cases {
		got := Segment(c.in)
		if got != c.want {
			t.Errorf("Segment(%q) = %q, want %q", c.in, got, c.want)
		}
		if back := Unsegment(got); back != c.in {
			t.Errorf("Unsegment(Segment(%q)) = %q", c.in, back)
		}
	}
}

func TestSaveBookAndQueries(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	book := sampleBook("梅宁风云", "示例作者", "/books/a.txt", strings.Repeat("a", 64))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if res.Skipped || res.Updated || res.BookID == 0 {
		t.Fatalf("unexpected save result: %+v", res)
	}

	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.Title != "梅宁风云" || detail.AuthorName != "示例作者" || detail.ChapterCount != 3 {
		t.Fatalf("detail = %+v", detail.BookSummary)
	}
	if len(detail.Volumes) != 2 {
		t.Fatalf("volumes = %d, want 2 (1 卷 + 伪卷)", len(detail.Volumes))
	}
	if len(detail.Volumes[0].Chapters) != 2 {
		t.Errorf("第一卷 chapters = %d, want 2", len(detail.Volumes[0].Chapters))
	}
	loose := detail.Volumes[len(detail.Volumes)-1]
	if loose.ID != 0 || loose.Idx != -1 || len(loose.Chapters) != 1 {
		t.Errorf("伪卷 = %+v", loose)
	}

	page, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 10})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("ListBooks = %+v, err=%v", page, err)
	}
	if page.Items[0].VolumeCount != 1 {
		t.Errorf("volumeCount = %d, want 1（只有第一卷被引用）", page.Items[0].VolumeCount)
	}

	chPage, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, From: 1, Limit: 2})
	if err != nil || chPage.Total != 3 || len(chPage.Items) != 2 {
		t.Fatalf("ListChapters = %+v, err=%v", chPage, err)
	}
	if chPage.Items[0].Title != "第一章 风起" {
		t.Errorf("first chapter = %q", chPage.Items[0].Title)
	}

	first := chPage.Items[0].ID
	ch, err := st.GetChapter(ctx, res.BookID, first)
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if ch.BookTitle != "梅宁风云" || ch.PrevID != 0 || ch.NextID == 0 {
		t.Errorf("chapter nav = %+v", ch)
	}
	allCh, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, From: 1, Limit: 10})
	if err != nil || len(allCh.Items) != 3 {
		t.Fatalf("ListChapters all = %+v, err=%v", allCh, err)
	}
	last := allCh.Items[len(allCh.Items)-1].ID
	ch2, err := st.GetChapter(ctx, res.BookID, last)
	if err != nil || ch2.PrevID == 0 || ch2.NextID != 0 {
		t.Errorf("last chapter nav = %+v, err=%v", ch2, err)
	}

	if _, err := st.GetChapter(ctx, res.BookID, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing chapter err = %v, want ErrNotFound", err)
	}

	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 10})
	if err != nil || authors.Total != 1 {
		t.Fatalf("ListAuthors = %+v, err=%v", authors, err)
	}
	if authors.Items[0].BookCount != 1 || authors.Items[0].WordCount != 30 {
		t.Errorf("author agg = %+v", authors.Items[0])
	}
	ad, err := st.GetAuthor(ctx, authors.Items[0].ID)
	if err != nil || len(ad.Books) != 1 {
		t.Fatalf("GetAuthor = %+v, err=%v", ad, err)
	}
	if _, err := st.GetAuthor(ctx, 12345); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing author err = %v", err)
	}

	if err := st.Checkpoint(ctx); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Books != 1 || stats.Chapters != 3 || stats.Authors != 1 || stats.Words != 30 {
		t.Errorf("stats = %+v", stats)
	}
	if stats.DBBytes <= 0 || stats.LastImportAt == nil {
		t.Errorf("stats extra = %+v", stats)
	}
}

func TestSaveBookUnknownAuthor(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("无名之书", "", "/books/nobody.txt", strings.Repeat("9", 64))
	if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 10})
	if err != nil || authors.Total != 1 {
		t.Fatalf("ListAuthors = %+v, err=%v", authors, err)
	}
	if authors.Items[0].Name != unknownAuthor || authors.Items[0].BookCount != 1 {
		t.Errorf("空作者应落「%s」: %+v", unknownAuthor, authors.Items[0])
	}
	books, err := st.ListBooks(ctx, BookQuery{AuthorID: authors.Items[0].ID, Page: 1, Size: 10})
	if err != nil || books.Total != 1 {
		t.Errorf("按作者筛选 = %+v, err=%v", books, err)
	}
}

func TestSaveBookIdempotentAndForce(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	sha1 := strings.Repeat("b", 64)
	book := sampleBook("梅宁风云", "示例作者", "/books/a.txt", sha1)
	first, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	// 同路径同哈希：幂等跳过。
	again, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if !again.Skipped || again.BookID != first.BookID {
		t.Fatalf("idempotent = %+v", again)
	}

	// 内容变了但没 Force：仍然跳过。
	changed := sampleBook("梅宁风云（修订）", "示例作者", "/books/a.txt", strings.Repeat("c", 64))
	changed.Chapters[0].Content = "新的开篇：寒江孤影，江湖故人。"
	changed.Chapters = append(changed.Chapters, model.ParsedChapter{Idx: 3, VolumeIdx: -1, Title: "第三章 新增", Content: "新章节", WordCount: 3})
	skipped, err := st.SaveBook(ctx, changed, SaveOptions{})
	if err != nil || !skipped.Skipped {
		t.Fatalf("changed without force = %+v, err=%v", skipped, err)
	}
	detail, err := st.GetBook(ctx, first.BookID)
	if err != nil || detail.ChapterCount != 3 {
		t.Fatalf("after skip chapterCount = %d, err=%v", detail.ChapterCount, err)
	}

	// Force 覆盖：id 不变、章节替换、FTS 无残留。
	forced, err := st.SaveBook(ctx, changed, SaveOptions{Force: true})
	if err != nil {
		t.Fatalf("force save: %v", err)
	}
	if !forced.Updated || forced.BookID != first.BookID {
		t.Fatalf("force result = %+v", forced)
	}
	detail, err = st.GetBook(ctx, first.BookID)
	if err != nil || detail.Title != "梅宁风云（修订）" || detail.ChapterCount != 4 {
		t.Fatalf("after force = %+v, err=%v", detail.BookSummary, err)
	}

	var ftsRows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM chapters_fts").Scan(&ftsRows); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if ftsRows != 4 {
		t.Errorf("chapters_fts rows = %d, want 4（覆盖后无残留）", ftsRows)
	}

	// 旧正文关键词不应再命中。
	old, err := st.Search(ctx, SearchQuery{Q: "午后的秋日"})
	if err != nil {
		t.Fatalf("search old: %v", err)
	}
	if old.Chapters.Total != 0 {
		t.Errorf("旧章节文本仍可搜到：%+v", old.Chapters.Items)
	}

	// 同 SHA、不同路径也算命中。
	moved := sampleBook("梅宁风云（修订）", "示例作者", "/books/moved.txt", strings.Repeat("c", 64))
	movedRes, err := st.SaveBook(ctx, moved, SaveOptions{})
	if err != nil || !movedRes.Skipped {
		t.Fatalf("sha dedup = %+v, err=%v", movedRes, err)
	}
}

func TestSearch(t *testing.T) {
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

	// 3 字关键词。
	r, err := st.Search(ctx, SearchQuery{Q: "临川市"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r.Chapters.Total != 1 || len(r.Chapters.Items) != 1 {
		t.Fatalf("3字命中 = %+v", r.Chapters)
	}
	if !strings.Contains(r.Chapters.Items[0].Snippet, "<b>临川市</b>") {
		t.Errorf("snippet 无高亮: %q", r.Chapters.Items[0].Snippet)
	}
	if r.Chapters.Items[0].BookTitle != "梅宁风云" {
		t.Errorf("bookTitle = %q", r.Chapters.Items[0].BookTitle)
	}

	// 2 字关键词（字级分词的关键收益）。
	r2, err := st.Search(ctx, SearchQuery{Q: "秋蝉"})
	if err != nil {
		t.Fatalf("search 2字: %v", err)
	}
	if r2.Chapters.Total != 1 {
		t.Fatalf("2字命中 = %+v", r2.Chapters)
	}
	if !strings.Contains(r2.Chapters.Items[0].Snippet, "<b>秋蝉</b>") {
		t.Errorf("2字 snippet = %q", r2.Chapters.Items[0].Snippet)
	}

	// snippet 里的 HTML 必须被转义，只留 <b>。
	snip := r2.Chapters.Items[0].Snippet
	if strings.Contains(snip, "<script>") {
		t.Errorf("snippet 未转义: %q", snip)
	}
	if !strings.Contains(snip, "&lt;script&gt;") {
		t.Logf("snippet（窗口可能未覆盖 script）: %q", snip)
	}

	// 1 字关键词：空结果 + 提示，不做全表 LIKE。
	r3, err := st.Search(ctx, SearchQuery{Q: "梅"})
	if err != nil {
		t.Fatalf("search 1字: %v", err)
	}
	if r3.Chapters.Total != 0 || r3.Books.Total != 0 || r3.Notice == "" {
		t.Errorf("1字查询应返回空结果+提示: %+v", r3)
	}

	// 书名检索（books 走 LIKE）。
	r4, err := st.Search(ctx, SearchQuery{Q: "梅宁风云", Type: "book"})
	if err != nil {
		t.Fatalf("search book: %v", err)
	}
	if r4.Books.Total != 1 || r4.Books.Items[0].Title != "梅宁风云" {
		t.Errorf("book 检索 = %+v", r4.Books)
	}
	if len(r4.Chapters.Items) != 0 {
		t.Errorf("type=book 不应返回章节: %+v", r4.Chapters)
	}

	// 限定单本书。
	id2, err := st.ListBooks(ctx, BookQuery{Q: "风雪归人", Page: 1, Size: 1})
	if err != nil || len(id2.Items) != 1 {
		t.Fatalf("find b2: %+v, err=%v", id2, err)
	}
	r5, err := st.Search(ctx, SearchQuery{Q: "梅宁", BookID: id2.Items[0].ID})
	if err != nil {
		t.Fatalf("search scoped: %v", err)
	}
	if r5.Chapters.Total != 1 || r5.Chapters.Items[0].BookID != id2.Items[0].ID {
		t.Errorf("book 内检索 = %+v", r5.Chapters)
	}

	// 分页字段。
	if r.Chapters.Page != 1 || r.Chapters.Size != 20 {
		t.Errorf("search page = %d/%d", r.Chapters.Page, r.Chapters.Size)
	}
}

func TestSearchTotalCapped(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	book := &model.ParsedBook{
		Title:      "大书",
		Author:     "作者",
		SourcePath: "/books/big.txt",
		SourceName: "big.txt",
		SHA256:     strings.Repeat("7", 64),
		WordCount:  1200 * 20,
	}
	for i := 0; i < 1200; i++ {
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

	r, err := st.Search(ctx, SearchQuery{Q: "临川市", Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if r.Chapters.Total != searchTotalCap {
		t.Errorf("total = %d, want %d", r.Chapters.Total, searchTotalCap)
	}
	if r.Notice != searchTotalCapNotice {
		t.Errorf("notice = %q, want %q", r.Notice, searchTotalCapNotice)
	}
	if len(r.Chapters.Items) != 20 {
		t.Errorf("第一页条数 = %d, want 20", len(r.Chapters.Items))
	}

	// 截断不影响深分页：仍按 LIMIT/OFFSET 取数据。
	deep, err := st.Search(ctx, SearchQuery{Q: "临川市", Page: 30, Size: 20})
	if err != nil {
		t.Fatalf("Search deep page: %v", err)
	}
	if len(deep.Chapters.Items) != 20 {
		t.Errorf("第 30 页条数 = %d, want 20", len(deep.Chapters.Items))
	}

	// 少量命中：不截断、无提示。
	few, err := st.Search(ctx, SearchQuery{Q: "暗号"})
	if err != nil {
		t.Fatalf("Search rare: %v", err)
	}
	if few.Chapters.Total != 1 || few.Notice != "" {
		t.Errorf("稀有词 total=%d notice=%q", few.Chapters.Total, few.Notice)
	}
}

func TestListBooksFilterAndPaging(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	authorNames := []string{"作者甲", "作者乙", "作者丙"}
	for i := 0; i < 3; i++ {
		b := sampleBook("书"+string(rune('A'+i)), authorNames[i], "/books/"+string(rune('a'+i))+".txt",
			strings.Repeat(string(rune('a'+i)), 64))
		if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

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

	// 按作者名模糊搜。
	q, err := st.ListBooks(ctx, BookQuery{Q: "作者甲", Page: 1, Size: 10})
	if err != nil || q.Total != 1 {
		t.Fatalf("author filter = %+v, err=%v", q, err)
	}

	// 标签过滤。
	var tagID, bookID int64
	if err := st.DB().QueryRowContext(ctx, "INSERT INTO tags(name) VALUES ('武侠') RETURNING id").Scan(&tagID); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	bookID = q.Items[0].ID
	if _, err := st.DB().ExecContext(ctx, "INSERT INTO book_tags(book_id, tag_id) VALUES (?, ?)", bookID, tagID); err != nil {
		t.Fatalf("insert book_tag: %v", err)
	}
	tagged, err := st.ListBooks(ctx, BookQuery{Tag: "武侠", Page: 1, Size: 10})
	if err != nil || tagged.Total != 1 || tagged.Items[0].ID != bookID {
		t.Fatalf("tag filter = %+v, err=%v", tagged, err)
	}
	tags, err := st.ListTags(ctx)
	if err != nil || len(tags) != 1 || tags[0].BookCount != 1 {
		t.Fatalf("ListTags = %+v, err=%v", tags, err)
	}
}

func TestImportJobs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	job, err := st.CreateImportJob(ctx, "/books")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	if job.ID == 0 || job.Status != "running" || job.StartedAt.IsZero() {
		t.Fatalf("job = %+v", job)
	}

	job.Total = 3
	job.Imported = 1
	job.Skipped = 1
	job.Failed = 1
	if err := st.UpdateImportJob(ctx, job); err != nil {
		t.Fatalf("UpdateImportJob: %v", err)
	}
	if err := st.AddImportError(ctx, job.ID, "/books/bad.txt", "decode failed"); err != nil {
		t.Fatalf("AddImportError: %v", err)
	}
	if err := st.AddImportError(ctx, job.ID, "/books/bad2.txt", "empty"); err != nil {
		t.Fatalf("AddImportError2: %v", err)
	}

	done := nowUTC()
	job.Status = "completed_with_errors"
	job.FinishedAt = &done
	job.Message = "1 file failed"
	if err := st.UpdateImportJob(ctx, job); err != nil {
		t.Fatalf("finish job: %v", err)
	}

	detail, err := st.GetImportJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.Status != "completed_with_errors" || detail.FinishedAt == nil || len(detail.Errors) != 2 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Errors[0].FilePath != "/books/bad.txt" || detail.Errors[0].CreatedAt.IsZero() {
		t.Errorf("error row = %+v", detail.Errors[0])
	}

	list, err := st.ListImportJobs(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListImportJobs = %+v, err=%v", list, err)
	}
	if _, err := st.GetImportJob(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing job err = %v", err)
	}
	if err := st.UpdateImportJob(ctx, &ImportJob{ID: 999}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing job err = %v", err)
	}
}

func TestReindexAndTriggerCleanup(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	b := sampleBook("索引测试", "作者", "/books/idx.txt", strings.Repeat("f", 64))
	saved, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := st.Reindex(ctx); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	r, err := st.Search(ctx, SearchQuery{Q: "临川市"})
	if err != nil || r.Chapters.Total != 1 {
		t.Fatalf("after reindex = %+v, err=%v", r.Chapters, err)
	}

	// 直接删书：级联删章节，触发器同步清理 FTS。
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM books WHERE id = ?", saved.BookID); err != nil {
		t.Fatalf("delete book: %v", err)
	}
	var ftsRows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM chapters_fts").Scan(&ftsRows); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if ftsRows != 0 {
		t.Errorf("级联删除后 FTS 残留 %d 行", ftsRows)
	}
	r2, err := st.Search(ctx, SearchQuery{Q: "临川市"})
	if err != nil || r2.Chapters.Total != 0 {
		t.Errorf("删除后仍能搜到: %+v, err=%v", r2.Chapters, err)
	}
}

func TestMigrateTwiceAndInvalidArgs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if _, err := st.SaveBook(ctx, nil, SaveOptions{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("nil book err = %v", err)
	}
	if _, err := st.SaveBook(ctx, &model.ParsedBook{SourcePath: "x"}, SaveOptions{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty sha err = %v", err)
	}
	if _, err := st.ListChapters(ctx, ChapterQuery{BookID: 0}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("chapter query err = %v", err)
	}
	if _, err := st.GetBook(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing book err = %v", err)
	}
}
