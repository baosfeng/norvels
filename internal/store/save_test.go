package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// savedStoreFixture 建一个空库并保存一本样例书，返回库、上下文与保存结果。
func savedStoreFixture(t *testing.T) (*Store, context.Context, SaveResult) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("梅宁风云", "示例作者", "/books/a.txt", strings.Repeat("a", 64))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return st, ctx, res
}

func TestSaveBookAndQueries(t *testing.T) {
	st, ctx, res := savedStoreFixture(t)
	if res.Skipped || res.Updated || res.BookID == 0 {
		t.Fatalf("unexpected save result: %+v", res)
	}
	t.Run("详情含目录树", func(t *testing.T) { checkBookDetailTree(t, ctx, st, res.BookID) })
	t.Run("列表", func(t *testing.T) { checkBookList(t, ctx, st) })
	t.Run("章节导航", func(t *testing.T) { checkChapterNav(t, ctx, st, res.BookID) })
	t.Run("作者与统计", func(t *testing.T) { checkAuthorsAndStats(t, ctx, st) })
}

// checkBookDetailTree 校验详情页的卷/章节树（含未归卷伪卷）。
func checkBookDetailTree(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	detail, err := st.GetBook(ctx, bookID)
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
}

// checkBookList 校验列表返回与卷计数（只有被章节引用的卷才计数）。
func checkBookList(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	page, err := st.ListBooks(ctx, BookQuery{Page: 1, Size: 10})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("ListBooks = %+v, err=%v", page, err)
	}
	if page.Items[0].VolumeCount != 1 {
		t.Errorf("volumeCount = %d, want 1（只有第一卷被引用）", page.Items[0].VolumeCount)
	}
}

// checkChapterNav 校验章节目录分页、首尾章导航与缺失章节的错误。
func checkChapterNav(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	chPage := listChapters(t, ctx, st, ChapterQuery{BookID: bookID, From: 1, Limit: 2})
	if chPage.Total != 3 || len(chPage.Items) != 2 {
		t.Fatalf("ListChapters = %+v", chPage)
	}
	if chPage.Items[0].Title != "第一章 风起" {
		t.Errorf("first chapter = %q", chPage.Items[0].Title)
	}
	checkFirstChapterNav(t, ctx, st, bookID, chPage.Items[0].ID)

	allCh := listChapters(t, ctx, st, ChapterQuery{BookID: bookID, From: 1, Limit: 10})
	if len(allCh.Items) != 3 {
		t.Fatalf("ListChapters all = %+v", allCh)
	}
	checkLastChapterNav(t, ctx, st, bookID, allCh.Items[len(allCh.Items)-1].ID)
	if _, err := st.GetChapter(ctx, bookID, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing chapter err = %v, want ErrNotFound", err)
	}
}

// checkFirstChapterNav 首章：无前一章、有后一章。
func checkFirstChapterNav(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64) {
	t.Helper()
	ch, err := st.GetChapter(ctx, bookID, chapterID)
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if ch.BookTitle != "梅宁风云" || ch.PrevID != 0 || ch.NextID == 0 {
		t.Errorf("chapter nav = %+v", ch)
	}
}

// checkLastChapterNav 末章：有前一章、无后一章。
func checkLastChapterNav(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64) {
	t.Helper()
	ch, err := st.GetChapter(ctx, bookID, chapterID)
	if err != nil {
		t.Fatalf("GetChapter(last): %v", err)
	}
	if ch.PrevID == 0 || ch.NextID != 0 {
		t.Errorf("last chapter nav = %+v", ch)
	}
}

// listChapters 跑一次目录查询并在出错时终止子测试。
func listChapters(t *testing.T, ctx context.Context, st *Store, q ChapterQuery) Page[ChapterBrief] {
	t.Helper()
	page, err := st.ListChapters(ctx, q)
	if err != nil {
		t.Fatalf("ListChapters %+v: %v", q, err)
	}
	return page
}

// checkAuthorsAndStats 校验作者聚合列与库级统计。
func checkAuthorsAndStats(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	checkAuthorAggregates(t, ctx, st)
	checkStats(t, ctx, st)
}

// checkAuthorAggregates 校验作者列表聚合列、作者详情与缺失作者的错误。
func checkAuthorAggregates(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	authors, err := st.ListAuthors(ctx, AuthorQuery{Page: 1, Size: 10})
	if err != nil || authors.Total != 1 {
		t.Fatalf("ListAuthors = %+v, err=%v", authors, err)
	}
	first := authors.Items[0]
	if first.BookCount != 1 || first.WordCount != 30 {
		t.Errorf("author agg = %+v", first)
	}
	ad, err := st.GetAuthor(ctx, first.ID)
	if err != nil || len(ad.Books) != 1 {
		t.Fatalf("GetAuthor = %+v, err=%v", ad, err)
	}
	if _, err := st.GetAuthor(ctx, 12345); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing author err = %v", err)
	}
}

// checkStats 校验 Checkpoint 之后的库级统计数字。
func checkStats(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
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

// revisedBook 返回「同路径、内容已变」的修订版（章节多一章，哈希不同）。
func revisedBook() *model.ParsedBook {
	b := sampleBook("梅宁风云（修订）", "示例作者", "/books/a.txt", strings.Repeat("c", 64))
	b.Chapters[0].Content = "新的开篇：寒江孤影，江湖故人。"
	b.Chapters = append(b.Chapters, model.ParsedChapter{
		Idx: 3, VolumeIdx: -1, Title: "第三章 新增", Content: "新章节", WordCount: 3,
	})
	return b
}

func TestSaveBookIdempotentAndForce(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("梅宁风云", "示例作者", "/books/a.txt", strings.Repeat("b", 64))
	first, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	changed := revisedBook()

	t.Run("幂等跳过", func(t *testing.T) { checkIdempotentSkip(t, ctx, st, book, first.BookID) })
	t.Run("变更未 Force 仍跳过", func(t *testing.T) { checkSkipWithoutForce(t, ctx, st, changed, first.BookID) })
	t.Run("Force 覆盖", func(t *testing.T) { checkForceOverwrite(t, ctx, st, changed, first.BookID) })
	t.Run("同 SHA 不同路径去重", func(t *testing.T) { checkSHA256Dedup(t, ctx, st) })
}

// checkIdempotentSkip 同路径同哈希重复保存应跳过且 id 不变。
func checkIdempotentSkip(t *testing.T, ctx context.Context, st *Store, book *model.ParsedBook, bookID int64) {
	t.Helper()
	again, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if !again.Skipped || again.BookID != bookID {
		t.Fatalf("idempotent = %+v", again)
	}
}

// checkSkipWithoutForce 内容变了但没 Force：仍跳过，库内章节数不变。
func checkSkipWithoutForce(t *testing.T, ctx context.Context, st *Store, changed *model.ParsedBook, bookID int64) {
	t.Helper()
	skipped, err := st.SaveBook(ctx, changed, SaveOptions{})
	if err != nil || !skipped.Skipped {
		t.Fatalf("changed without force = %+v, err=%v", skipped, err)
	}
	detail, err := st.GetBook(ctx, bookID)
	if err != nil || detail.ChapterCount != 3 {
		t.Fatalf("after skip chapterCount = %d, err=%v", detail.ChapterCount, err)
	}
}

// checkForceOverwrite Force 覆盖：id 不变、章节替换、FTS 无残留。
func checkForceOverwrite(t *testing.T, ctx context.Context, st *Store, changed *model.ParsedBook, bookID int64) {
	t.Helper()
	forced, err := st.SaveBook(ctx, changed, SaveOptions{Force: true})
	if err != nil {
		t.Fatalf("force save: %v", err)
	}
	if !forced.Updated || forced.BookID != bookID {
		t.Fatalf("force result = %+v", forced)
	}
	assertForceOverwrite(t, ctx, st, bookID)
}

// assertForceOverwrite 校验 Force 覆盖后的详情、FTS 行数与旧文本不可检索。
func assertForceOverwrite(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	detail, err := st.GetBook(ctx, bookID)
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
}

// checkSHA256Dedup 同 SHA、不同路径也算命中（幂等跳过）。
func checkSHA256Dedup(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	moved := sampleBook("梅宁风云（修订）", "示例作者", "/books/moved.txt", strings.Repeat("c", 64))
	movedRes, err := st.SaveBook(ctx, moved, SaveOptions{})
	if err != nil || !movedRes.Skipped {
		t.Fatalf("sha dedup = %+v, err=%v", movedRes, err)
	}
}

func TestImportJobs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	job := startImportJob(t, ctx, st)

	t.Run("运行中状态", func(t *testing.T) { checkImportJobRunning(t, job) })
	t.Run("更新与失败明细", func(t *testing.T) { checkImportJobFinished(t, ctx, st, job) })
	t.Run("不存在时的错误", func(t *testing.T) { checkImportJobMissing(t, ctx, st) })
}

// checkImportJobRunning 新建任务应为 running 且有开始时间。
func checkImportJobRunning(t *testing.T, job *ImportJob) {
	t.Helper()
	if job.ID == 0 || job.Status != "running" || job.StartedAt.IsZero() {
		t.Fatalf("job = %+v", job)
	}
}

// checkImportJobFinished 完成后状态、失败明细与列表都应符合预期。
func checkImportJobFinished(t *testing.T, ctx context.Context, st *Store, job *ImportJob) {
	t.Helper()
	finished := finishImportJob(t, ctx, st, job)
	detail, err := st.GetImportJob(ctx, finished.ID)
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
}

// checkImportJobMissing 不存在的任务读写都应返回 ErrNotFound。
func checkImportJobMissing(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if _, err := st.GetImportJob(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing job err = %v", err)
	}
	if err := st.UpdateImportJob(ctx, &ImportJob{ID: 999}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing job err = %v", err)
	}
}

// startImportJob 建一个运行中的导入任务并写入初始进度。
func startImportJob(t *testing.T, ctx context.Context, st *Store) *ImportJob {
	t.Helper()
	job, err := st.CreateImportJob(ctx, "/books")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
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
	return job
}

// finishImportJob 把任务置为「完成但有错误」并落完成时间。
func finishImportJob(t *testing.T, ctx context.Context, st *Store, job *ImportJob) *ImportJob {
	t.Helper()
	done := nowUTC()
	job.Status = "completed_with_errors"
	job.FinishedAt = &done
	job.Message = "1 file failed"
	if err := st.UpdateImportJob(ctx, job); err != nil {
		t.Fatalf("finish job: %v", err)
	}
	return job
}
