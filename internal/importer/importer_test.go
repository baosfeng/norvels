package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"norvels/internal/parser"
	"norvels/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "norvels.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st
}

const bodyA = "华夏国梅宁市，午后的秋日，微风拂过城楼。他站在城楼上望着远方，心中百感交集。\n\n" +
	"风起云涌，江湖路远。少年提剑前行，脚下是万里山河。\n\n"

const bodyB = "梅宁城的雪下了三天三夜。秋蝉在雪里叫着，声音又细又长。\n\n" +
	"他说，等到春天，我们就去看海。\n\n"

func sampleText(title, author string) string {
	var b strings.Builder
	b.WriteString("书名：" + title + "\n作者：" + author + "\n\n")
	b.WriteString("第一章 风起\n")
	for i := 0; i < 6; i++ {
		b.WriteString(bodyA)
	}
	b.WriteString("第二章 云涌\n")
	for i := 0; i < 6; i++ {
		b.WriteString(bodyB)
	}
	b.WriteString("第三章 归途\n")
	for i := 0; i < 6; i++ {
		b.WriteString(bodyA)
	}
	return b.String()
}

func writeTxt(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func opts(dir string) Options {
	return Options{Dir: dir, Recursive: true, ParserOpts: parser.DefaultOptions()}
}

func TestRunImportsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("示例书名", "示例作者"))
	writeTxt(t, dir, "b.txt", sampleText("风雪归人", "月关"))
	writeTxt(t, dir, "ignore.md", "not a novel")

	var (
		mu      sync.Mutex
		seen    []Progress
		inCb    int32
		overlap bool
	)
	cb := func(p Progress) {
		mu.Lock()
		inCb++
		if inCb > 1 {
			overlap = true
		}
		mu.Unlock()
		time.Sleep(2 * time.Millisecond) // 放大并发窗口
		mu.Lock()
		seen = append(seen, p)
		inCb--
		mu.Unlock()
	}

	res, err := Run(ctx, st, opts(dir), cb)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Failed != 0 || res.Skipped != 0 || len(res.Imported) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if overlap {
		t.Errorf("进度回调出现并发调用")
	}
	if len(seen) != 2 || seen[len(seen)-1].Total != 2 || seen[len(seen)-1].Index != 2 {
		t.Fatalf("progress = %+v", seen)
	}
	if res.Job == nil || res.Job.Status != "completed" || res.Job.Total != 2 || res.Job.Imported != 2 {
		t.Fatalf("job = %+v", res.Job)
	}

	detail, err := st.GetImportJob(ctx, res.Job.ID)
	if err != nil || detail.Status != "completed" || len(detail.Errors) != 0 || detail.FinishedAt == nil {
		t.Fatalf("job detail = %+v, err=%v", detail, err)
	}

	books, err := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if err != nil || books.Total != 2 {
		t.Fatalf("books = %+v, err=%v", books, err)
	}
	for _, b := range books.Items {
		if b.ChapterCount < 1 || b.WordCount <= 0 || b.AuthorName == "" {
			t.Errorf("book = %+v", b)
		}
	}

	// 真实正文经 parser → Segment → FTS 全链路可检索。
	r, err := st.Search(ctx, store.SearchQuery{Q: "梅宁市"})
	if err != nil || r.Chapters.Total < 2 {
		t.Fatalf("search 梅宁市 = %+v, err=%v", r.Chapters, err)
	}
	if !strings.Contains(r.Chapters.Items[0].Snippet, "<b>梅宁市</b>") {
		t.Errorf("snippet = %q", r.Chapters.Items[0].Snippet)
	}

	// 第二次导入：全部走 SHA256 幂等跳过。
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
	books, _ = st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
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

	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	o := opts(dir)
	o.JobID = job.ID
	res, err := Run(ctx, st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Job == nil || res.Job.ID != job.ID {
		t.Fatalf("未复用任务：%+v", res.Job)
	}
	detail, err := st.GetImportJob(ctx, job.ID)
	if err != nil || detail.Status != "completed" || detail.Total != 1 || detail.Imported != 1 {
		t.Fatalf("detail = %+v, err=%v", detail, err)
	}

	// 不存在的 JobID。
	o.JobID = 99999
	if _, err := Run(ctx, st, o, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing job err = %v", err)
	}

	// 已完成的任务不能复用。
	o.JobID = job.ID
	if _, err := Run(ctx, st, o, nil); !errors.Is(err, store.ErrConflict) {
		t.Errorf("completed job reuse err = %v", err)
	}
}

func TestRunErrorsAndEdgeCases(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// 空目录。
	empty := t.TempDir()
	if _, err := Run(ctx, st, opts(empty), nil); !errors.Is(err, ErrNoFiles) {
		t.Errorf("empty dir err = %v", err)
	}

	// 不存在的目录。
	if _, err := Run(ctx, st, opts(filepath.Join(empty, "nope")), nil); err == nil {
		t.Errorf("missing dir 应报错")
	}

	// 损坏的符号链接 → 单文件失败，任务 completed_with_errors。
	dir := t.TempDir()
	writeTxt(t, dir, "ok.txt", sampleText("正常书", "作者"))
	if err := os.Symlink(filepath.Join(dir, "missing-target"), filepath.Join(dir, "broken.txt")); err != nil {
		t.Skipf("symlink 不可用: %v", err)
	}
	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Failed != 1 || len(res.Imported) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0].FilePath, "broken.txt") {
		t.Errorf("errors = %+v", res.Errors)
	}
	if res.Job == nil {
		t.Fatalf("job 为空")
	}
	detail, err := st.GetImportJob(ctx, res.Job.ID)
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
