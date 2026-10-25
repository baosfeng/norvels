package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"norvels/internal/store"
)

func TestRunImportsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("示例书名", "示例作者"))
	writeTxt(t, dir, "b.txt", sampleText("风雪归人", "月关"))
	writeTxt(t, dir, "ignore.md", "not a novel")

	rec := &progressRecorder{}
	res, err := Run(ctx, st, opts(dir), rec.callback())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	t.Run("汇总与进度回调", func(t *testing.T) { checkImportSummary(t, res, rec) })
	t.Run("任务记录", func(t *testing.T) { checkImportJobCompleted(t, ctx, st, res) })
	t.Run("书籍与检索", func(t *testing.T) { checkImportedBooksSearchable(t, ctx, st) })
	t.Run("二次导入幂等", func(t *testing.T) { checkSecondRunIdempotent(t, ctx, st, dir) })
}

// checkImportSummary 校验首次导入的计数与进度回调（串行、末次快照）。
func checkImportSummary(t *testing.T, res *Result, rec *progressRecorder) {
	t.Helper()
	if res.Failed != 0 || res.Skipped != 0 || len(res.Imported) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if rec.overlap {
		t.Errorf("进度回调出现并发调用")
	}
	seen := rec.seen
	if len(seen) != 2 || seen[len(seen)-1].Total != 2 || seen[len(seen)-1].Index != 2 {
		t.Fatalf("progress = %+v", seen)
	}
}

// checkImportJobCompleted 校验任务记录终态与详情。
func checkImportJobCompleted(t *testing.T, ctx context.Context, st *store.Store, res *Result) {
	t.Helper()
	if res.Job == nil || res.Job.Status != "completed" || res.Job.Total != 2 || res.Job.Imported != 2 {
		t.Fatalf("job = %+v", res.Job)
	}
	detail, err := st.GetImportJob(ctx, res.Job.ID)
	if err != nil || detail.Status != "completed" || len(detail.Errors) != 0 || detail.FinishedAt == nil {
		t.Fatalf("job detail = %+v, err=%v", detail, err)
	}
}

// checkImportedBooksSearchable 校验入库书籍的元信息与检索范围。
func checkImportedBooksSearchable(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	assertImportedBookMeta(t, ctx, st)
	assertChapterTitleSearchable(t, ctx, st)
	assertBodyNotSearchable(t, ctx, st)
}

// assertImportedBookMeta 校验两本书的元信息都被补齐。
func assertImportedBookMeta(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	books, err := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if err != nil || books.Total != 2 {
		t.Fatalf("books = %+v, err=%v", books, err)
	}
	for _, b := range books.Items {
		if b.ChapterCount < 1 || b.WordCount <= 0 || b.AuthorName == "" {
			t.Errorf("book = %+v", b)
		}
	}
}

// assertChapterTitleSearchable 导入后按章节标题可检索。
// 合成书稿的章节标题固定为「第一章 风起」等，不依赖 parser 的书名推断。
func assertChapterTitleSearchable(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	r, err := st.Search(ctx, store.SearchQuery{Q: "第一章"})
	if err != nil || r.Chapters.Total < 1 {
		t.Fatalf("search 第一章 = %+v, err=%v", r.Chapters, err)
	}
	// 正文检索已下线，但标题命中要带章序上下文（「第 N 章 · 共 M 章」）。
	if got := r.Chapters.Items[0].Snippet; got != "第 1 章 · 共 3 章" {
		t.Errorf("Snippet 应为章序上下文，实际 %q", got)
	}
}

// assertBodyNotSearchable 校验正文里的词不再命中：确认检索没退回正文扫表。
func assertBodyNotSearchable(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	inBody, err := st.Search(ctx, store.SearchQuery{Q: "梅宁市"})
	if err != nil {
		t.Fatalf("search 梅宁市: %v", err)
	}
	if inBody.Chapters.Total != 0 || inBody.Books.Total != 0 {
		t.Errorf("正文关键词不应命中：chapters=%d books=%d", inBody.Chapters.Total, inBody.Books.Total)
	}
}

// checkSecondRunIdempotent 第二次导入：全部走 SHA256 幂等跳过。
func checkSecondRunIdempotent(t *testing.T, ctx context.Context, st *store.Store, dir string) {
	t.Helper()
	res2, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if res2.Skipped != 2 || len(res2.Imported) != 0 || res2.Failed != 0 {
		t.Fatalf("second result = %+v", res2)
	}
	if res2.Job == nil || res2.Job.Status != "completed" || res2.Job.Skipped != 2 {
		t.Fatalf("second job = %+v", res2.Job)
	}
	books, _ := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if books.Total != 2 {
		t.Errorf("幂等后书籍数 = %d", books.Total)
	}
}

func TestRunDryRunDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("示例书名", "示例作者"))

	o := opts(dir)
	o.DryRun = true
	res, err := Run(ctx, st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 1 || res.Job != nil {
		t.Fatalf("dry result = %+v", res)
	}
	books, _ := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if books.Total != 0 {
		t.Errorf("DryRun 写了库：%d 本", books.Total)
	}
	jobs, _ := st.ListImportJobs(ctx, 10)
	if len(jobs) != 0 {
		t.Errorf("DryRun 写了任务：%+v", jobs)
	}
}

func TestRunMinChapters(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("示例书名", "示例作者"))

	o := opts(dir)
	o.MinChapters = 1000
	res, err := Run(ctx, st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Skipped != 1 || len(res.Imported) != 0 {
		t.Fatalf("min chapters result = %+v", res)
	}
	books, _ := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if books.Total != 0 {
		t.Errorf("MinChapters 未生效：%d 本", books.Total)
	}
}

func TestRunReusesExternalJobID(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("示例书名", "示例作者"))

	job := createRunningJob(t, ctx, st, dir)
	t.Run("复用外部任务", func(t *testing.T) { checkReusesJob(t, ctx, st, dir, job.ID) })
	t.Run("不存在的 JobID", func(t *testing.T) { checkMissingJobID(t, ctx, st, dir) })
	t.Run("已完成任务不可复用", func(t *testing.T) { checkCompletedJobRejected(t, ctx, st, dir, job.ID) })
}

// createRunningJob 建一个处于 running 的导入任务。
func createRunningJob(t *testing.T, ctx context.Context, st *store.Store, dir string) *store.ImportJob {
	t.Helper()
	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	return job
}

// checkReusesJob 传入 JobID 时应复用该任务并把终态写在同一条记录上。
func checkReusesJob(t *testing.T, ctx context.Context, st *store.Store, dir string, jobID int64) {
	t.Helper()
	o := opts(dir)
	o.JobID = jobID
	res, err := Run(ctx, st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Job == nil || res.Job.ID != jobID {
		t.Fatalf("未复用任务：%+v", res.Job)
	}
	detail, err := st.GetImportJob(ctx, jobID)
	if err != nil || detail.Status != "completed" || detail.Total != 1 || detail.Imported != 1 {
		t.Fatalf("detail = %+v, err=%v", detail, err)
	}
}

// checkMissingJobID 不存在的 JobID 应返回 ErrNotFound。
func checkMissingJobID(t *testing.T, ctx context.Context, st *store.Store, dir string) {
	t.Helper()
	o := opts(dir)
	o.JobID = 99999
	if _, err := Run(ctx, st, o, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing job err = %v", err)
	}
}

// checkCompletedJobRejected 已完成的任务不能复用，应返回 ErrConflict。
func checkCompletedJobRejected(t *testing.T, ctx context.Context, st *store.Store, dir string, jobID int64) {
	t.Helper()
	o := opts(dir)
	o.JobID = jobID
	if _, err := Run(ctx, st, o, nil); !errors.Is(err, store.ErrConflict) {
		t.Errorf("completed job reuse err = %v", err)
	}
}

func TestRunErrorsAndEdgeCases(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	t.Run("空目录", func(t *testing.T) { checkEmptyDir(t, ctx, st) })
	t.Run("不存在的目录", func(t *testing.T) { checkMissingDir(t, ctx, st) })
	t.Run("损坏的符号链接", func(t *testing.T) { checkBrokenSymlink(t, ctx, st) })
}

// checkEmptyDir 空目录返回 ErrNoFiles。
func checkEmptyDir(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	empty := t.TempDir()
	if _, err := Run(ctx, st, opts(empty), nil); !errors.Is(err, ErrNoFiles) {
		t.Errorf("empty dir err = %v", err)
	}
}

// checkMissingDir 不存在的目录直接报错。
func checkMissingDir(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	empty := t.TempDir()
	if _, err := Run(ctx, st, opts(filepath.Join(empty, "nope")), nil); err == nil {
		t.Errorf("missing dir 应报错")
	}
}

// checkBrokenSymlink 损坏的符号链接 → 单文件失败，任务 completed_with_errors。
func checkBrokenSymlink(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	dir := t.TempDir()
	writeTxt(t, dir, "ok.txt", sampleText("正常书", "作者"))
	if err := os.Symlink(filepath.Join(dir, "missing-target"), filepath.Join(dir, "broken.txt")); err != nil {
		t.Skipf("symlink 不可用: %v", err)
	}
	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	checkBrokenSymlinkResult(t, res)
	checkBrokenSymlinkJob(t, ctx, st, res.Job)
}

// checkBrokenSymlinkResult 校验失败计数与错误明细指向损坏文件。
func checkBrokenSymlinkResult(t *testing.T, res *Result) {
	t.Helper()
	if res.Failed != 1 || len(res.Imported) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0].FilePath, "broken.txt") {
		t.Errorf("errors = %+v", res.Errors)
	}
}

// checkBrokenSymlinkJob 校验任务落成 completed_with_errors 并带 1 条失败明细。
func checkBrokenSymlinkJob(t *testing.T, ctx context.Context, st *store.Store, job *store.ImportJob) {
	t.Helper()
	if job == nil {
		t.Fatalf("job 为空")
	}
	detail, err := st.GetImportJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.Status != "completed_with_errors" || detail.Failed != 1 || len(detail.Errors) != 1 {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestRunCanceled(t *testing.T) {
	st := newStore(t)
	dir := t.TempDir()
	for i := 0; i < 6; i++ {
		writeTxt(t, dir, string(rune('a'+i))+".txt", sampleText("书"+string(rune('A'+i)), "作者"))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	o := opts(dir)
	o.Workers = 1
	res, err := Run(ctx, st, o, func(p Progress) {
		once.Do(cancel)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if res == nil || res.Job == nil {
		t.Fatalf("res = %+v", res)
	}
	detail, gerr := st.GetImportJob(context.Background(), res.Job.ID)
	if gerr != nil {
		t.Fatalf("GetImportJob: %v", gerr)
	}
	if detail.Status != "canceled" {
		t.Errorf("status = %q, want canceled", detail.Status)
	}
}

func TestRunRecursiveVsFlat(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTxt(t, root, "top.txt", sampleText("顶层书", "作者"))
	writeTxt(t, sub, "deep.txt", sampleText("深层书", "作者"))

	o := opts(root)
	o.Recursive = false
	res, err := Run(ctx, st, o, nil)
	if err != nil {
		t.Fatalf("flat Run: %v", err)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("非递归 result = %+v", res)
	}

	o = opts(root)
	o.Recursive = true
	res, err = Run(ctx, st, o, nil)
	if err != nil {
		t.Fatalf("recursive Run: %v", err)
	}
	if len(res.Imported) != 1 || res.Skipped != 1 {
		t.Fatalf("递归 result = %+v", res)
	}
}
