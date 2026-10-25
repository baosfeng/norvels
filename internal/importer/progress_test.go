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

	"norvels/internal/model"
	"norvels/internal/store"
)

// 本文件补的是覆盖率门禁盯着的几个分支：中断时落进度、三种任务终态、
// 多文件时才触发的进度落库、worker 提前退出。它们原本靠「大样本碰巧走到」，
// 现在显式覆盖，让覆盖率不因删掉字级分词的间接覆盖而下滑。

// TestPersistProgressWritesCounters 校验中断路径上的进度落库。
//
// ctx 取消后进度仍要写成功（内部用 WithoutCancel），否则用户看到的进度会停在中途。
func TestPersistProgressWritesCounters(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("长夜书简", "青川客"))

	job, err := st.CreateImportJob(ctx, dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	state := &runState{st: st, job: job, res: &Result{Imported: []int64{1, 2}}, lastErr: "示例失败"}
	state.res.Skipped = 3
	state.res.Failed = 4

	// 传入已取消的 ctx：persistProgress 内部要脱钩，写仍应成功。
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	state.persistProgress(canceled)

	detail, err := st.GetImportJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.Imported != 2 || detail.Skipped != 3 || detail.Failed != 4 {
		t.Errorf("进度未落库: imported=%d skipped=%d failed=%d",
			detail.Imported, detail.Skipped, detail.Failed)
	}
	if detail.Message != "示例失败" {
		t.Errorf("message = %q，期望带上最近错误", detail.Message)
	}
}

// TestJobStatusMapping 覆盖三种任务终态的判定。
func TestJobStatusMapping(t *testing.T) {
	cases := []struct {
		name string
		res  *Result
		want string
	}{
		{"全部失败", &Result{Failed: 3}, "failed"},
		{"部分失败", &Result{Imported: []int64{1}, Failed: 1}, "completed_with_errors"},
		{"全部成功", &Result{Imported: []int64{1, 2}}, "completed"},
		{"全部跳过", &Result{Skipped: 5}, "completed"},
		{"失败但有跳过", &Result{Skipped: 1, Failed: 1}, "completed_with_errors"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := jobStatus(c.res); got != c.want {
				t.Errorf("jobStatus(%+v) = %q，期望 %q", c.res, got, c.want)
			}
		})
	}
}

// TestRunPersistsProgressOnMultiFileImport 覆盖「每 jobUpdateEvery 个文件落一次进度」。
//
// 单文件导入不会触发这条分支（jobUpdateEvery = 5），因此需要足够的文件数。
func TestRunPersistsProgressOnMultiFileImport(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	const n = 7 // > jobUpdateEvery，确保至少落一次进度
	for i := 0; i < n; i++ {
		writeTxt(t, dir, string(rune('a'+i))+".txt", sampleText("书"+string(rune('A'+i)), "作者"))
	}

	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != n {
		t.Fatalf("导入 %d 本，期望 %d", len(res.Imported), n)
	}
	detail, err := st.GetImportJob(ctx, res.Job.ID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.Status != "completed" || detail.Imported != n || detail.Total != n {
		t.Errorf("任务终态 = %+v", detail.ImportJob)
	}
	if detail.FinishedAt == nil {
		t.Error("终态任务应有 finished_at")
	}
}

// TestRunAllFailedMarksFailed 全部文件失败时任务应为 failed（而不是 completed）。
func TestRunAllFailedMarksFailed(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	// 只有坏符号链接：每个候选文件都会失败。
	for i := 0; i < 2; i++ {
		name := string(rune('a'+i)) + ".txt"
		if err := os.Symlink(filepath.Join(dir, "missing-"+name), filepath.Join(dir, name)); err != nil {
			t.Skipf("symlink 不可用: %v", err)
		}
	}

	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Failed != 2 || len(res.Imported) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if res.Job.Status != "failed" {
		t.Errorf("全失败任务状态 = %q，期望 failed", res.Job.Status)
	}
}

// TestResolveSourceDirTrimsAndAbsolutizes 覆盖入参归一化。
func TestResolveSourceDirTrimsAndAbsolutizes(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveSourceDir("  " + dir + "  ")
	if err != nil {
		t.Fatalf("resolveSourceDir: %v", err)
	}
	if got != dir {
		t.Errorf("resolveSourceDir = %q，期望 %q（应去空白并取绝对路径）", got, dir)
	}
}

// TestResolveWorkersClamps 覆盖并发数收敛：0/负数走默认、按文件数夹紧、下限为 1。
//
// 注意夹紧顺序：显式指定的数值先被 maxWorkers 收敛，再夹到文件数以内，
// 因此「want 超上限」的断言要配足够的文件数才看得到 maxWorkers。
func TestResolveWorkersClamps(t *testing.T) {
	if got := resolveWorkers(0, 1000); got < 1 || got > maxWorkers {
		t.Errorf("resolveWorkers(0) = %d，应在 [1, %d]", got, maxWorkers)
	}
	if got := resolveWorkers(-3, 1000); got < 1 {
		t.Errorf("resolveWorkers(-3) = %d，应回落到默认值", got)
	}
	// 文件数不足时按文件数夹紧（不能为 0）。
	if got := resolveWorkers(maxWorkers, 3); got != 3 {
		t.Errorf("resolveWorkers(%d, 3) = %d，期望 3（按文件数夹紧）", maxWorkers, got)
	}
	// 文件数为 0 时下限为 1（否则 worker 数为 0 会让流水线卡死）。
	if got := resolveWorkers(4, 0); got != 1 {
		t.Errorf("resolveWorkers(4, 0) = %d，期望 1（下限）", got)
	}
}

// TestParseWorkerStopsOnCanceledContext 覆盖 worker 侧提前退出的分支。
func TestParseWorkerStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tasks := make(chan fileTask)
	outcomes := make(chan parseOutcome, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go parseWorker(ctx, &wg, tasks, outcomes, nil, nil, opts(t.TempDir()))

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	close(tasks) // 关闭输入让 for-range 结束
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("parseWorker 未在 ctx 取消 / 输入关闭后退出")
	}
}

// TestApplyOutcomeBranches 覆盖 applyOutcome 的四种分流。
//
// 这是「计数口径」的唯一收口处：解析失败/幂等跳过/低于最小章节数/DryRun/正常写入
// 各走一条分支，任何一条被改动都会让导入汇总与库内实际条数对不上。
func TestApplyOutcomeBranches(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	book := sampleBookForApply()

	t.Run("解析失败记一条错误", func(t *testing.T) { checkApplyParseError(t, ctx, st) })

	t.Run("幂等跳过不写库", func(t *testing.T) {
		state := &runState{st: st, res: &Result{}}
		state.applyOutcome(ctx, parseOutcome{index: 2, path: "/path/to/txt/a.txt", skipped: true}, Options{})
		if state.res.Skipped != 1 || len(state.res.Imported) != 0 {
			t.Errorf("skipped=%d imported=%d，期望 1/0", state.res.Skipped, len(state.res.Imported))
		}
	})

	t.Run("低于最小章节数算跳过", func(t *testing.T) {
		state := &runState{st: st, res: &Result{}}
		state.applyOutcome(ctx, parseOutcome{index: 3, path: "/path/to/txt/thin.txt",
			books: []*model.ParsedBook{book}}, Options{MinChapters: 999})
		if state.res.Skipped != 1 {
			t.Errorf("skipped=%d，期望 1（章节数不足应跳过）", state.res.Skipped)
		}
	})

	t.Run("DryRun 只占位不写库", func(t *testing.T) { checkApplyDryRun(t, ctx, st, book) })

	t.Run("正常写入累加 id", func(t *testing.T) {
		state := &runState{st: st, res: &Result{}}
		state.applyOutcome(ctx, parseOutcome{index: 5, path: book.SourcePath,
			books: []*model.ParsedBook{book}}, Options{})
		if len(state.res.Imported) != 1 || state.res.Imported[0] == 0 {
			t.Errorf("imported=%v，期望写入 1 本并带回真实 id", state.res.Imported)
		}
	})
}

// checkApplyParseError 解析失败：记 1 条失败 + 明细 + 最近错误。
func checkApplyParseError(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	state := &runState{st: st, res: &Result{}}
	state.applyOutcome(ctx, parseOutcome{index: 1, path: "/path/to/txt/bad.txt",
		err: errors.New("decode failed")}, Options{})
	if state.res.Failed != 1 || len(state.res.Errors) != 1 {
		t.Fatalf("failed=%d errors=%d，期望 1/1", state.res.Failed, len(state.res.Errors))
	}
	if state.res.Errors[0].FilePath != "/path/to/txt/bad.txt" {
		t.Errorf("失败明细路径 = %q", state.res.Errors[0].FilePath)
	}
	if state.lastErr == "" {
		t.Error("应记下最近错误供进度展示")
	}
}

// checkApplyDryRun DryRun：只占位计数，不写库。
func checkApplyDryRun(t *testing.T, ctx context.Context, st *store.Store, book *model.ParsedBook) {
	t.Helper()
	state := &runState{st: st, res: &Result{}}
	title := state.applyOutcome(ctx, parseOutcome{index: 4, path: "/path/to/txt/dry.txt",
		books: []*model.ParsedBook{book}}, Options{DryRun: true})
	if len(state.res.Imported) != 1 || state.res.Imported[0] != 0 {
		t.Errorf("imported=%v，期望占位 [0]", state.res.Imported)
	}
	if title != book.Title {
		t.Errorf("进度书名 = %q，期望 %q", title, book.Title)
	}
	if books, _ := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10}); books.Total != 0 {
		t.Errorf("DryRun 写了库：%d 本", books.Total)
	}
}

// sampleBookForApply 造一本最小的待写书籍（依赖 store 的模型类型）。
func sampleBookForApply() *model.ParsedBook {
	return &model.ParsedBook{
		Title:      "长夜书简",
		Author:     "青川客",
		SourcePath: "/path/to/txt/a.txt",
		SourceName: "a.txt",
		Encoding:   "utf-8",
		FileSize:   128,
		SHA256:     strings.Repeat("a", 64),
		WordCount:  6,
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章 风起", Content: "长夜书简，灯下无人。", WordCount: 6},
		},
	}
}
