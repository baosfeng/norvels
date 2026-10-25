package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// runOutcome 是一次 Run 的结局（结果与错误一起回传，便于断言两者）。
type runOutcome struct {
	res *Result
	err error
}

// TestCancelStopsRunningImport 证明「取消信号真的能让导入停下」：
//
// 用 200 个文件跑一次真实导入，跑起来后按任务 id 取消，期望 Run 返回
// context.Canceled、库里任务终态是 canceled，且已入库数量小于总数。
//
// 这一条是 api 那个 202 的下游证据：端点只负责发信号，真正停下与写 canceled
// 都发生在导入 goroutine 收尾（见 run.go 的 closing）。
func TestCancelStopsRunningImport(t *testing.T) {
	st := newStore(t)
	dir := writeHeavyDir(t, 200)
	jobID, unregister, done := runAndTrack(t, st, dir)
	defer unregister()

	// 等第一个文件落库再取消：保证「取消发生在导入进行中」而不是启动瞬间。
	waitForFirstImport(t, st)

	if !Cancel(jobID) {
		t.Fatal("Cancel 未命中运行中的任务")
	}
	const files = 200
	requireCanceledOutcome(t, done, files)
	requireJobStatus(t, st, jobID, "canceled")
	requireBooksKept(t, st)

	// 注销（api 的 spawnImport 在 goroutine 退出时做的事）之后再取消：返回 false。
	unregister()
	if Cancel(jobID) {
		t.Error("注销后的任务不该被取消")
	}
	if Cancel(0) {
		t.Error("jobID=0 不该命中")
	}
}

// TestCancelStopsUploadImport 覆盖上传管线的取消：RunUpload 走的是 run.go 的
// startParseWorkers，同样要能停下、带上 Canceled，并保留已入库的书。
func TestCancelStopsUploadImport(t *testing.T) {
	st := newStore(t)
	dir := writeHeavyDir(t, 200)
	jobID, unregister, done := runUploadAndTrack(t, st, dir)
	defer unregister()

	waitForFirstImport(t, st)
	if !Cancel(jobID) {
		t.Fatal("Cancel 未命中运行中的上传任务")
	}
	const files = 200
	requireCanceledOutcome(t, done, files)
	requireJobStatus(t, st, jobID, "canceled")
	requireBooksKept(t, st)
}

// runUploadAndTrack 与 runAndTrack 同构，但跑 RunUpload（上传管线）。
func runUploadAndTrack(t *testing.T, st *store.Store, dir string) (int64, func(), chan runOutcome) {
	t.Helper()
	job, err := st.CreateImportJob(context.Background(), dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	opt := opts(dir)
	opt.JobID = job.ID

	ctx, cancel := context.WithCancel(context.Background())
	unregister := Track(job.ID, cancel)
	done := make(chan runOutcome, 1)
	go func() {
		res, runErr := RunUpload(ctx, st, opt)
		done <- runOutcome{res: res, err: runErr}
	}()
	return job.ID, unregister, done
}

// requireBooksKept 断言取消后已入库的书仍在书架（不回滚、不删除）。
func requireBooksKept(t *testing.T, st *store.Store) {
	t.Helper()
	page, err := st.ListBooks(context.Background(), store.BookQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if page.Total < 1 {
		t.Error("取消后已入库的书应保留，实际书架为空")
	}
}

// runAndTrack 建任务、登记取消函数并在后台起 Run；返回 job id、注销函数与结局通道。
func runAndTrack(t *testing.T, st *store.Store, dir string) (int64, func(), chan runOutcome) {
	t.Helper()
	job, err := st.CreateImportJob(context.Background(), dir)
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	opt := opts(dir)
	opt.JobID = job.ID

	ctx, cancel := context.WithCancel(context.Background())
	unregister := Track(job.ID, cancel)
	done := make(chan runOutcome, 1)
	go func() {
		res, runErr := Run(ctx, st, opt, nil)
		done <- runOutcome{res: res, err: runErr}
	}()
	return job.ID, unregister, done
}

// requireCanceledOutcome 断言 Run 以 context.Canceled 结束且确实没导完。
func requireCanceledOutcome(t *testing.T, done chan runOutcome, total int) {
	t.Helper()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("Run err = %v，期望 context.Canceled", got.err)
		}
		if got.res == nil {
			t.Fatal("取消应返回可读的 Result（含已入库列表）")
		}
		if !got.res.Canceled {
			t.Error("取消提前结束时 Result.Canceled 应为 true")
		}
		if len(got.res.Imported) >= total {
			t.Errorf("取消后仍导入了 %d 本（总数 %d）", len(got.res.Imported), total)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("取消失败：导入未在 30s 内停止")
	}
}

// requireJobStatus 断言任务记录的终态。
func requireJobStatus(t *testing.T, st *store.Store, jobID int64, want string) {
	t.Helper()
	detail, err := st.GetImportJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.Status != want {
		t.Errorf("任务状态 = %q，期望 %q", detail.Status, want)
	}
}

// writeHeavyDir 在临时目录里写 n 个偏厚的待导入文件（让一次导入有可取消的时长）。
func writeHeavyDir(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("示例书%03d", i)
		writeTxt(t, dir, title+".txt", heavyText(title))
	}
	return dir
}

// heavyText 造一份「3 章 + 一大段正文」的待导入文本。
func heavyText(title string) string {
	return sampleText(title, "作者甲") + strings.Repeat(bodyA+"\n\n"+bodyB+"\n\n", 300)
}

// waitForFirstImport 轮询任务已入库数，直到 ≥1（或超时）。
func waitForFirstImport(t *testing.T, st *store.Store) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		jobs, err := st.ListImportJobs(context.Background(), 1)
		if err != nil {
			t.Fatalf("ListImportJobs: %v", err)
		}
		if len(jobs) == 1 && jobs[0].Imported >= 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("20s 内没有导入任何文件，取消用例不成立")
}
