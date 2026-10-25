package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"norvels/internal/model"
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
		if ctx.Err() != nil {
			// 已取消：worker 正在退出，这里只把通道排空，不再写库/回调——
			// 「已入库的书保留，未处理的文件不再处理」。
			r.res.Canceled = true
			continue
		}
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

// applyOutcome 把单个条目的解析结果计入汇总，返回用于进度展示的书名。
func (r *runState) applyOutcome(ctx context.Context, oc parseOutcome, opt Options) string {
	switch {
	case oc.err != nil:
		return r.recordFileError(ctx, oc.cand.source, oc.path, oc.err)
	case oc.skipped:
		r.res.Skipped++
		r.recordJobFile(ctx, oc.cand.source, oc.path, store.ImportFileStatusSkipped,
			"库中已存在同一本书（SHA-256 或源路径命中）", 0)
		return ""
	case len(oc.books) == 0:
		return ""
	}
	return r.saveBooks(ctx, oc, opt)
}

// saveBooks 逐本落库（合集拆出的多本各算一本），返回第一本的书名用于进度展示。
func (r *runState) saveBooks(ctx context.Context, oc parseOutcome, opt Options) string {
	title := ""
	for _, book := range oc.books {
		if title == "" {
			title = book.Title
		}
		if len(book.Chapters) < opt.MinChapters {
			r.res.Skipped++
			r.recordJobFile(ctx, oc.cand.source, oc.path, store.ImportFileStatusSkipped,
				fmt.Sprintf("章节数不足 %d", opt.MinChapters), 0)
			continue
		}
		if opt.DryRun {
			// DryRun 不写库，这里只占位表达计数。
			r.res.Imported = append(r.res.Imported, 0)
			continue
		}
		r.saveOne(ctx, oc.cand.source, oc.path, book, opt)
	}
	return title
}

// recordIssues 把解包阶段的问题（整包失败、条目不支持）逐条记进失败明细与文件终态。
func (r *runState) recordIssues(ctx context.Context, issues []expandIssue) {
	for _, issue := range issues {
		r.recordFileError(ctx, issue.source, issue.path, errors.New(issue.reason))
		r.recordJobFile(ctx, issue.source, issue.path, statusForIssue(issue.kind), issue.reason, 0)
	}
}

// statusForIssue 把解包问题类型翻译成文件级终态。
func statusForIssue(kind string) string {
	if kind == issueKindUnsupported {
		return store.ImportFileStatusUnsupported
	}
	return store.ImportFileStatusFailed
}

// recordFileError 记录一次文件级失败：汇总计数、错误列表、文件终态与任务明细。
func (r *runState) recordFileError(ctx context.Context, source, display string, err error) string {
	r.res.Failed++
	r.res.Errors = append(r.res.Errors, store.ImportError{FilePath: display, Reason: err.Error()})
	r.lastErr = err.Error()
	if r.job != nil {
		if aerr := r.st.AddImportError(ctx, r.job.ID, display, err.Error()); aerr != nil {
			r.lastErr = aerr.Error()
		}
	}
	r.recordJobFile(ctx, source, display, store.ImportFileStatusFailed, err.Error(), 0)
	return ""
}

// recordJobFile 写一条文件级终态；job 为 nil（dry-run）时是空操作。
//
// 写失败只记 lastErr 不打断导入：文件级终态是「重试用的辅助数据」，
// 不能因为它写不进去就让整批书导入失败。
func (r *runState) recordJobFile(ctx context.Context, source, display, status, reason string, bookID int64) {
	if r.job == nil || source == "" {
		return
	}
	if err := r.st.UpsertImportJobFile(ctx, r.job.ID, store.ImportJobFile{
		Source: source, DisplayPath: display, Status: status, Reason: reason, BookID: bookID,
	}); err != nil {
		r.lastErr = err.Error()
	}
}

// saveOne 把一本书写入库，按「新建 / 追更 / 幂等跳过 / 失败」分别计数并记文件终态。
//
// Force 优先于 Update（store 的既有语义）：显式覆盖时不走追更匹配。
func (r *runState) saveOne(ctx context.Context, source, display string, book *model.ParsedBook, opt Options) {
	sr, err := r.st.SaveBook(ctx, book, store.SaveOptions{Force: opt.Force, Update: opt.Update})
	switch {
	case err != nil:
		r.recordFileError(ctx, source, display, err)
	case sr.Skipped:
		r.res.Skipped++
		r.recordJobFile(ctx, source, display, store.ImportFileStatusSkipped, "库中已存在同一本书", 0)
	default:
		r.recordAppendBadge(ctx, sr)
		r.res.Imported = append(r.res.Imported, sr.BookID)
		r.recordJobFile(ctx, source, display, store.ImportFileStatusImported, "", sr.BookID)
	}
}

// recordAppendBadge 把本次追更追加的章数累加到 books.new_chapters（书架/详情角标）。
//
// 写失败只记 lastErr 不打断导入：角标是辅助展示数据，让一份已经入库成功的
// 追更整体失败、下次还要重导，代价远大于少一个角标。章数来自库层的
// SaveResult.Appended（appendBookInTx 的 tail 长度），这里不再自己数一遍。
func (r *runState) recordAppendBadge(ctx context.Context, sr store.SaveResult) {
	if sr.Appended <= 0 {
		return
	}
	if err := r.st.AddNewChapters(ctx, sr.BookID, sr.Appended); err != nil {
		r.lastErr = err.Error()
	}
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
		r.res.Canceled = true
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

// collectCandidates 生成候选：普通导入走目录扫描 + zip 展开；重试只处理给定源。
func collectCandidates(files []string, opt Options, scratch string) ([]candidate, []expandIssue) {
	if len(opt.Sources) > 0 {
		return sourcesToCandidates(opt.Sources, scratch)
	}
	return expandRoots(files, scratch)
}

// resolveSourceDir 把 Options.Dir 归一化为绝对路径。
func resolveSourceDir(dir string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return "", fmt.Errorf("importer: resolve dir: %w", err)
	}
	return abs, nil
}

// verifySource 确认路径存在，且是目录或单个可导入文件（.txt/.epub）。
// 返回的 reason 用于写任务记录（不带包前缀），error 用于返回给调用方。
func verifySource(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("stat: %v", err), fmt.Errorf("importer: stat: %w", err)
	}
	if fi.IsDir() || SupportedExt(fi.Name()) {
		return "", nil
	}
	reason := fmt.Sprintf("not a directory or importable file: %s", path)
	return reason, fmt.Errorf("importer: %s 既不是目录，也不是 .txt 文件或 .epub 文件", path)
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
func startParseWorkers(ctx context.Context, st *store.Store, p *parser.Parser, cands []candidate, workers int, opt Options) <-chan parseOutcome {
	tasks := make(chan fileTask)
	outcomes := make(chan parseOutcome, workers)

	go feedTasks(ctx, tasks, cands)

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
func feedTasks(ctx context.Context, tasks chan<- fileTask, cands []candidate) {
	defer close(tasks)
	for i, cand := range cands {
		select {
		case tasks <- fileTask{index: i + 1, cand: cand}:
		case <-ctx.Done():
			return
		}
	}
}

// parseWorker 逐个取任务解析，并把结果写回结果通道；ctx 取消后立即退出（上传管线）。
//
// 取消语义：命中 ctx.Done 就停止取任务、不再投递解析结果——未处理的文件不再
// 处理，已入库的书保留；取消标记由 consume 与 closing 落到 Result.Canceled。
func parseWorker(ctx context.Context, wg *sync.WaitGroup, tasks <-chan fileTask, outcomes chan<- parseOutcome,
	st *store.Store, p *parser.Parser, opt Options) {
	defer wg.Done()
	for t := range tasks {
		if ctx.Err() != nil {
			// 已取消：停止取任务，未处理的文件不再处理。
			return
		}
		select {
		case outcomes <- processFile(ctx, st, p, t, opt):
		case <-ctx.Done():
			return
		}
	}
}
