// Package importer 负责目录扫描、并发解析、幂等入库、任务记录与进度上报。
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"norvels/internal/model"
	"norvels/internal/parser"
	"norvels/internal/store"
)

// Options 描述一次导入。
type Options struct {
	Dir       string // 源目录（绝对路径）
	Recursive bool   // 是否递归子目录
	Force     bool   // 忽略 SHA256 幂等检查，覆盖更新
	DryRun    bool   // 只解析不写库，用于试跑与报告
	Workers   int    // 并发解析数，默认 min(NumCPU, 8)
	// MinChapters 章节数低于该值的文件不上架，默认 0（全收）。
	MinChapters int
	// ParserOpts 透传给 parser。
	ParserOpts parser.Options
	// JobID 由外部先 CreateImportJob 再传入（api 的 POST /api/import 走这条路径）。
	// >0 时复用该任务（要求 status=running），0 表示由 Run 内部创建。
	// DryRun 时不写任何任务记录，JobID 被忽略。
	JobID int64
}

// Progress 是一次进度回调的快照。
type Progress struct {
	Index     int    // 当前处理的第几个文件（从 1 开始）
	Total     int    // 候选文件总数
	Path      string // 当前文件
	BookTitle string
	Imported  int
	Skipped   int
	Failed    int
	ElapsedMS int64
	LastError string
}

// Result 是一次导入的汇总。
type Result struct {
	Job      *store.ImportJob
	Imported []int64 // 新写入的 book id（DryRun 时为占位 0，长度即解析成功数）
	Skipped  int
	Failed   int
	Errors   []store.ImportError
}

// ErrNoFiles 表示目录中没有可导入的 txt。
var ErrNoFiles = errors.New("importer: no .txt files found")

// maxWorkers 是默认并发上限：解析是 CPU + 内存混合负载，过多并发反而拖慢。
const maxWorkers = 8

// jobUpdateEvery 是任务进度写库的粒度（每处理 N 个文件落一次）。
const jobUpdateEvery = 5

type fileTask struct {
	index int
	path  string
}

type parseOutcome struct {
	index   int
	path    string
	book    *model.ParsedBook
	skipped bool
	err     error
}

// Run 执行导入。cb 可为 nil。
//
// 执行模型：解析在 workers 个 goroutine 中并发（CPU 密集），写库与进度回调
// 在调用 goroutine 中串行（SQLite 单写者 + 触发器同步 FTS，串行反而更快更稳）。
// progress 的回调因此天然满足「单 goroutine 串行」约束。
func Run(ctx context.Context, st *store.Store, opt Options, cb func(Progress)) (*Result, error) {
	if st == nil {
		return nil, errors.New("importer: nil store")
	}
	dir, err := filepath.Abs(strings.TrimSpace(opt.Dir))
	if err != nil {
		return nil, fmt.Errorf("importer: resolve dir: %w", err)
	}

	start := time.Now()
	res := &Result{}

	// 任务记录：api 可以先 CreateImportJob 拿到精确 jobId，再把 JobID 传进来复用。
	var job *store.ImportJob
	if !opt.DryRun && opt.JobID > 0 {
		existing, gerr := st.GetImportJob(ctx, opt.JobID)
		if gerr != nil {
			return nil, gerr
		}
		if existing.Status != "running" {
			return nil, fmt.Errorf("%w: import job %d status is %q", store.ErrConflict, opt.JobID, existing.Status)
		}
		job = &existing.ImportJob
	}
	failJob := func(reason string) {
		if job == nil {
			return
		}
		job.Status = "failed"
		job.Message = reason
		finishJob(ctx, st, job, res, reason)
	}

	fi, err := os.Stat(dir)
	if err != nil {
		failJob(fmt.Sprintf("stat dir: %v", err))
		return nil, fmt.Errorf("importer: stat dir: %w", err)
	}
	if !fi.IsDir() {
		failJob(fmt.Sprintf("not a directory: %s", dir))
		return nil, fmt.Errorf("importer: %s is not a directory", dir)
	}

	files, err := scanFiles(ctx, dir, opt.Recursive)
	if err != nil {
		failJob(err.Error())
		return nil, err
	}
	if len(files) == 0 {
		failJob("no .txt files found")
		return nil, ErrNoFiles
	}

	workers := opt.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers > maxWorkers {
			workers = maxWorkers
		}
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	if !opt.DryRun && job == nil {
		job, err = st.CreateImportJob(ctx, dir)
		if err != nil {
			return nil, err
		}
	}
	if job != nil {
		job.Total = len(files)
		if err := st.UpdateImportJob(ctx, job); err != nil {
			return nil, err
		}
		res.Job = job
	}

	p := parser.New(opt.ParserOpts)
	tasks := make(chan fileTask)
	outcomes := make(chan parseOutcome, workers)

	go func() {
		defer close(tasks)
		for i, path := range files {
			select {
			case tasks <- fileTask{index: i + 1, path: path}:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
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
		}()
	}
	go func() {
		wg.Wait()
		close(outcomes)
	}()

	var (
		processed int
		lastErr   string
	)
	for oc := range outcomes {
		processed++
		title := ""
		switch {
		case oc.err != nil:
			res.Failed++
			res.Errors = append(res.Errors, store.ImportError{FilePath: oc.path, Reason: oc.err.Error()})
			lastErr = oc.err.Error()
			if job != nil {
				if err := st.AddImportError(ctx, job.ID, oc.path, oc.err.Error()); err != nil {
					lastErr = err.Error()
				}
			}
		case oc.skipped:
			res.Skipped++
		case oc.book != nil && len(oc.book.Chapters) < opt.MinChapters:
			res.Skipped++
		case oc.book != nil && opt.DryRun:
			title = oc.book.Title
			res.Imported = append(res.Imported, 0) // DryRun 不写库，这里只占位表达计数
		case oc.book != nil:
			title = oc.book.Title
			sr, err := st.SaveBook(ctx, oc.book, store.SaveOptions{Force: opt.Force})
			if err != nil {
				res.Failed++
				res.Errors = append(res.Errors, store.ImportError{FilePath: oc.path, Reason: err.Error()})
				lastErr = err.Error()
			} else if sr.Skipped {
				res.Skipped++
			} else {
				res.Imported = append(res.Imported, sr.BookID)
			}
		}

		if cb != nil {
			cb(Progress{
				Index:     processed,
				Total:     len(files),
				Path:      oc.path,
				BookTitle: title,
				Imported:  len(res.Imported),
				Skipped:   res.Skipped,
				Failed:    res.Failed,
				ElapsedMS: time.Since(start).Milliseconds(),
				LastError: lastErr,
			})
		}
		if job != nil && processed%jobUpdateEvery == 0 {
			job.Imported = len(res.Imported)
			job.Skipped = res.Skipped
			job.Failed = res.Failed
			job.Message = lastErr
			_ = st.UpdateImportJob(context.WithoutCancel(ctx), job)
		}
	}

	if cerr := ctx.Err(); cerr != nil {
		if job != nil {
			job.Status = "canceled"
			finishJob(ctx, st, job, res, lastErr)
		}
		return res, cerr
	}

	if job != nil {
		switch {
		case res.Failed > 0 && len(res.Imported) == 0 && res.Skipped == 0:
			job.Status = "failed"
		case res.Failed > 0:
			job.Status = "completed_with_errors"
		default:
			job.Status = "completed"
		}
		finishJob(ctx, st, job, res, lastErr)
	}
	return res, nil
}

// finishJob 落盘任务终态（即使 ctx 已取消也要写成功）。
func finishJob(ctx context.Context, st *store.Store, job *store.ImportJob, res *Result, lastErr string) {
	done := time.Now().UTC()
	job.FinishedAt = &done
	job.Imported = len(res.Imported)
	job.Skipped = res.Skipped
	job.Failed = res.Failed
	if lastErr != "" && job.Message == "" {
		job.Message = lastErr
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = st.UpdateImportJob(wctx, job)
}

// processFile 读取单文件、算 SHA256、幂等判定、解析，并补齐路径相关元信息。
func processFile(ctx context.Context, st *store.Store, p *parser.Parser, t fileTask, opt Options) parseOutcome {
	oc := parseOutcome{index: t.index, path: t.path}
	data, err := os.ReadFile(t.path)
	if err != nil {
		oc.err = fmt.Errorf("read: %w", err)
		return oc
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])

	exists, err := st.BookExistsBySHA256(ctx, sha)
	if err != nil {
		oc.err = fmt.Errorf("dedup: %w", err)
		return oc
	}
	if exists && !opt.Force {
		oc.skipped = true
		return oc
	}

	book, err := p.ParseBytes(filepath.Base(t.path), data)
	if err != nil {
		oc.err = fmt.Errorf("parse: %w", err)
		return oc
	}
	if book == nil {
		oc.err = errors.New("parse: empty result")
		return oc
	}
	book.SourcePath = t.path
	book.SourceName = filepath.Base(t.path)
	book.FileSize = int64(len(data))
	book.SHA256 = sha
	oc.book = book
	return oc
}

// scanFiles 收集候选 txt（跳过隐藏文件），结果按路径排序保证可复现。
func scanFiles(ctx context.Context, root string, recursive bool) ([]string, error) {
	files := make([]string, 0, 256)
	add := func(path string, d fs.DirEntry) {
		if d.IsDir() {
			return
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return
		}
		if !strings.EqualFold(filepath.Ext(name), ".txt") {
			return
		}
		files = append(files, path)
	}

	if recursive {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // 单个条目不可读时跳过，不影响整批
			}
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if path == root {
				return nil
			}
			add(path, d)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("importer: scan dir: %w", err)
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("importer: read dir: %w", err)
		}
		for _, e := range entries {
			add(filepath.Join(root, e.Name()), e)
		}
	}
	sort.Strings(files)
	return files, nil
}
