package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"norvels/internal/parser"
	"norvels/internal/store"
)

// runState 承载一次导入的运行期状态：任务记录、汇总结果与最近一次错误。
// 让 Run 的编排逻辑保持扁平：所有「改计数/改任务」都收敛成这里的方法。
type runState struct {
	st      *store.Store
	job     *store.ImportJob
	res     *Result
	lastErr string
}

// prepareJob 处理外部传入的 JobID：复用已有任务并要求其仍处于 running。
// 未传 JobID 或 DryRun 时什么都不做（任务在文件扫描完成后才创建）。
func (r *runState) prepareJob(ctx context.Context, opt Options) error {
	if opt.DryRun || opt.JobID <= 0 {
		return nil
	}
	existing, err := r.st.GetImportJob(ctx, opt.JobID)
	if err != nil {
		return err
	}
	if existing.Status != "running" {
		return fmt.Errorf("%w: import job %d status is %q", store.ErrConflict, opt.JobID, existing.Status)
	}
	r.job = &existing.ImportJob
	return nil
}

// failJob 把任务标记为失败并落盘；没有任务记录时是空操作。
func (r *runState) failJob(ctx context.Context, reason string) {
	if r.job == nil {
		return
	}
	r.job.Status = "failed"
	r.job.Message = reason
	finishJob(ctx, r.st, r.job, r.res, reason)
}

// openJob 在候选文件确定后创建（或复用）任务记录并写入文件总数。
func (r *runState) openJob(ctx context.Context, dir string, total int, opt Options) error {
	if !opt.DryRun && r.job == nil {
		job, err := r.st.CreateImportJob(ctx, dir)
		if err != nil {
			return err
		}
		r.job = job
	}
	if r.job == nil {
		return nil
	}
	r.job.Total = total
	if err := r.st.UpdateImportJob(ctx, r.job); err != nil {
		return err
	}
	r.res.Job = r.job
	return nil
}

// consume 串行消费解析结果：计数、回调进度、按粒度刷新任务记录。
// 写库与回调都在调用 goroutine 中进行，因此 Progress 天然串行。
func (r *runState) consume(ctx context.Context, outcomes <-chan parseOutcome, total int, start time.Time,
	cb func(Progress), opt Options) {
	processed := 0
	for oc := range outcomes {
		processed++
		title := r.applyOutcome(ctx, oc, opt)
		if cb != nil {
			cb(r.progress(processed, total, oc.path, title, start))
		}
		if r.job != nil && processed%jobUpdateEvery == 0 {
			r.persistProgress(ctx)
		}
	}
}

// applyOutcome 把单个文件的解析结果计入汇总，返回用于进度展示的书名。
func (r *runState) applyOutcome(ctx context.Context, oc parseOutcome, opt Options) string {
	switch {
	case oc.err != nil:
		return r.recordFileError(ctx, oc.path, oc.err)
	case oc.skipped:
		r.res.Skipped++
		return ""
	case oc.book != nil && len(oc.book.Chapters) < opt.MinChapters:
		r.res.Skipped++
		return ""
	case oc.book != nil && opt.DryRun:
		// DryRun 不写库，这里只占位表达计数。
		r.res.Imported = append(r.res.Imported, 0)
		return oc.book.Title
	case oc.book != nil:
		return r.saveParsedBook(ctx, oc, opt)
	}
	return ""
}

// recordFileError 记录一次文件级失败：汇总计数、错误列表，并同步写入任务明细。
func (r *runState) recordFileError(ctx context.Context, path string, err error) string {
	r.res.Failed++
	r.res.Errors = append(r.res.Errors, store.ImportError{FilePath: path, Reason: err.Error()})
	r.lastErr = err.Error()
	if r.job != nil {
		if aerr := r.st.AddImportError(ctx, r.job.ID, path, err.Error()); aerr != nil {
			r.lastErr = aerr.Error()
		}
	}
	return ""
}

// saveParsedBook 把解析结果写入库，按「新建/幂等跳过/失败」分别计数，返回书名。
func (r *runState) saveParsedBook(ctx context.Context, oc parseOutcome, opt Options) string {
	sr, err := r.st.SaveBook(ctx, oc.book, store.SaveOptions{Force: opt.Force})
	switch {
	case err != nil:
		r.res.Failed++
		r.res.Errors = append(r.res.Errors, store.ImportError{FilePath: oc.path, Reason: err.Error()})
		r.lastErr = err.Error()
	case sr.Skipped:
		r.res.Skipped++
	default:
		r.res.Imported = append(r.res.Imported, sr.BookID)
	}
	return oc.book.Title
}

// progress 生成一次进度快照。
func (r *runState) progress(index, total int, path, title string, start time.Time) Progress {
	return Progress{
		Index:     index,
		Total:     total,
		Path:      path,
		BookTitle: title,
		Imported:  len(r.res.Imported),
		Skipped:   r.res.Skipped,
		Failed:    r.res.Failed,
		ElapsedMS: time.Since(start).Milliseconds(),
		LastError: r.lastErr,
	}
}

// persistProgress 把当前计数落到任务记录（每 jobUpdateEvery 个文件一次，失败不打断导入）。
func (r *runState) persistProgress(ctx context.Context) {
	r.job.Imported = len(r.res.Imported)
	r.job.Skipped = r.res.Skipped
	r.job.Failed = r.res.Failed
	r.job.Message = r.lastErr
	_ = r.st.UpdateImportJob(context.WithoutCancel(ctx), r.job)
}

// closing 收尾：ctx 已取消则把任务标记为 canceled 并返回取消错误，
// 否则按汇总结果落终态。
func (r *runState) closing(ctx context.Context) (*Result, error) {
	if cerr := ctx.Err(); cerr != nil {
		if r.job != nil {
			r.job.Status = "canceled"
			finishJob(ctx, r.st, r.job, r.res, r.lastErr)
		}
		return r.res, cerr
	}
	if r.job != nil {
		r.job.Status = jobStatus(r.res)
		finishJob(ctx, r.st, r.job, r.res, r.lastErr)
	}
	return r.res, nil
}

// jobStatus 由汇总计数推出任务终态：全失败为 failed，有失败为 completed_with_errors。
func jobStatus(res *Result) string {
	switch {
	case res.Failed > 0 && len(res.Imported) == 0 && res.Skipped == 0:
		return "failed"
	case res.Failed > 0:
		return "completed_with_errors"
	default:
		return "completed"
	}
}

// resolveSourceDir 把 Options.Dir 归一化为绝对路径。
func resolveSourceDir(dir string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return "", fmt.Errorf("importer: resolve dir: %w", err)
	}
	return abs, nil
}

// verifySourceDir 确认路径存在且是目录。
// 返回的 reason 用于写任务记录（不带包前缀），error 用于返回给调用方。
func verifySourceDir(dir string) (string, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Sprintf("stat dir: %v", err), fmt.Errorf("importer: stat dir: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Sprintf("not a directory: %s", dir), fmt.Errorf("importer: %s is not a directory", dir)
	}
	return "", nil
}

// resolveWorkers 决定并发解析数：未指定时取 min(NumCPU, maxWorkers)，并夹在 [1, 文件数]。
func resolveWorkers(want, files int) int {
	if want <= 0 {
		want = runtime.NumCPU()
		if want > maxWorkers {
			want = maxWorkers
		}
	}
	if want > files {
		want = files
	}
	if want < 1 {
		want = 1
	}
	return want
}

// startParseWorkers 启动解析流水线并返回结果通道：一个 goroutine 投递任务，
// workers 个 goroutine 并发解析，全部结束后关闭结果通道。
func startParseWorkers(ctx context.Context, st *store.Store, p *parser.Parser, files []string, workers int, opt Options) <-chan parseOutcome {
	tasks := make(chan fileTask)
	outcomes := make(chan parseOutcome, workers)

	go feedTasks(ctx, tasks, files)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go parseWorker(ctx, &wg, tasks, outcomes, st, p, opt)
	}
	go func() {
		wg.Wait()
		close(outcomes)
	}()
	return outcomes
}

// feedTasks 按顺序把候选文件投递给解析 worker；ctx 取消时提前关闭任务通道。
func feedTasks(ctx context.Context, tasks chan<- fileTask, files []string) {
	defer close(tasks)
	for i, path := range files {
		select {
		case tasks <- fileTask{index: i + 1, path: path}:
		case <-ctx.Done():
			return
		}
	}
}

// parseWorker 逐个取任务解析，并把结果写回结果通道；ctx 取消后立即退出。
func parseWorker(ctx context.Context, wg *sync.WaitGroup, tasks <-chan fileTask, outcomes chan<- parseOutcome,
	st *store.Store, p *parser.Parser, opt Options) {
	defer wg.Done()
	for t := range tasks {
		if ctx.Err() != nil {
			return
		}
		select {
		case outcomes <- processFile(ctx, st, p, t, opt):
		case <-ctx.Done():
			return
		}
	}
}
