// Package importer 负责目录扫描、并发解析、幂等入库、任务记录与进度上报。
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
// 在调用 goroutine 中串行（SQLite 单写者，串行反而更快更稳）。
// progress 的回调因此天然满足「单 goroutine 串行」约束。
//
// 编排分四步，各自独立：准备（路径/任务）→ 扫描候选 → 并发解析 + 串行入库 → 落任务终态。
func Run(ctx context.Context, st *store.Store, opt Options, cb func(Progress)) (*Result, error) {
	state, dir, files, err := prepareRun(ctx, st, opt)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	if err := state.openJob(ctx, dir, len(files), opt); err != nil {
		return nil, err
	}

	workers := resolveWorkers(opt.Workers, len(files))
	outcomes := startParseWorkers(ctx, st, parser.New(opt.ParserOpts), files, workers, opt)
	state.consume(ctx, outcomes, len(files), start, cb, opt)

	return state.closing(ctx)
}

// prepareRun 完成导入前的全部准备工作：校验入参、复用任务、校验目录、扫描候选文件。
// 任一步失败都会把已有任务记录标记为失败，并原样返回错误。
func prepareRun(ctx context.Context, st *store.Store, opt Options) (*runState, string, []string, error) {
	if st == nil {
		return nil, "", nil, errors.New("importer: nil store")
	}
	dir, err := resolveSourceDir(opt.Dir)
	if err != nil {
		return nil, "", nil, err
	}

	state := &runState{st: st, res: &Result{}}

	// 任务记录：api 可以先 CreateImportJob 拿到精确 jobId，再把 JobID 传进来复用。
	if err := state.prepareJob(ctx, opt); err != nil {
		return nil, "", nil, err
	}

	if reason, verr := verifySourceDir(dir); verr != nil {
		state.failJob(ctx, reason)
		return nil, "", nil, verr
	}

	files, err := scanFiles(ctx, dir, opt.Recursive)
	switch {
	case err != nil:
		state.failJob(ctx, err.Error())
		return nil, "", nil, err
	case len(files) == 0:
		state.failJob(ctx, "no .txt files found")
		return nil, "", nil, ErrNoFiles
	}
	return state, dir, files, nil
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
	sha := sha256Hex(data)

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
	fillBookSource(book, t.path, data, sha)
	oc.book = book
	return oc
}

// sha256Hex 返回文件内容的 SHA256 小写十六进制串（幂等键）。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fillBookSource 用磁盘上的真实信息补齐书籍的来源字段。
func fillBookSource(book *model.ParsedBook, path string, data []byte, sha string) {
	book.SourcePath = path
	book.SourceName = filepath.Base(path)
	book.FileSize = int64(len(data))
	book.SHA256 = sha
}
