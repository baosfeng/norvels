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

	"norvels/internal/ebook"
	"norvels/internal/model"
	"norvels/internal/parser"
	"norvels/internal/store"
)

// Options 描述一次导入。
type Options struct {
	Dir       string // 源路径（绝对路径）：目录或单个 .txt 文件
	Recursive bool   // 是否递归子目录
	Force     bool   // 忽略 SHA256 幂等检查，覆盖更新
	Update    bool   // 追更：仅追加新章节（前缀对不上由 store 报 ErrNotAppendable）
	DryRun    bool   // 只解析不写库，用于试跑与报告
	Workers   int    // 并发解析数，默认 min(NumCPU, 8)
	// MinChapters 章节数低于该值的文件不上架，默认 0（全收）。
	MinChapters int
	// ParserOpts 透传给 parser。
	ParserOpts parser.Options
	// ChapterRules 是调用方传入的自定义切章规则，按顺序优先于内置切章策略、命中即作为
	// 章节边界。api 的两个导入入口都在请求线程里读 settings 再注入（loadChapterRules）；为空
	// 表示只用内置策略——importer 自己不读 settings，也不依赖 store 的设置表。
	ChapterRules []parser.Rule
	// TagRules 是词表打标规则（api 从 settings 读出注入）；空表示不打自动标签。
	// 匹配口径见 tagrules.go：命中即写 source=auto 的标签，只增不删。
	TagRules []store.TagRule
	// Sources 非空表示「单文件重试」：只处理这些可重放源（绝对路径 / zip 包:包内路径），
	// 跳过目录扫描；源不可用按文件级失败记录，不中断整批。
	Sources []string
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
	// Canceled 表示本次导入因 ctx 取消而提前结束：已入库的书保留，未处理的
	// 文件不再处理。任务终态由 closing 写成 canceled，api 也会按错误映射，
	// 这里让调用方可以直接从结果里读到取消标记。
	Canceled bool
}

// ErrNoFiles 表示目录中没有可导入的文件（支持格式见 SupportedExt）。
var ErrNoFiles = errors.New("importer: 没有可导入的文件（.txt / .epub / .zip）")

// maxWorkers 是默认并发上限：解析是 CPU + 内存混合负载，过多并发反而拖慢。
const maxWorkers = 8

// jobUpdateEvery 是任务进度写库的粒度（每处理 N 个文件落一次）。
const jobUpdateEvery = 5

type fileTask struct {
	index int
	cand  candidate
}

// parseOutcome 是一个候选条目的解析结果。一本书可能拆成多本（合集），所以是切片。
type parseOutcome struct {
	index   int
	path    string // 对外路径：普通文件是文件名，zip 条目是「包名.zip:包内路径」
	cand    candidate
	books   []*model.ParsedBook
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
	// 候选先展开（zip → 条目），再按条目数开任务：zip 里的每个条目都算一条进度。
	scratch, err := makeScratch("")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	cands, issues := collectCandidates(files, opt, scratch)
	// 分类按相对导入根的目录层级推断，必须在候选展开（zip → 条目）之后做。
	annotateCategories(cands, dir)
	// 同名多卷的 .txt 先归并成一个处理单元，其余候选保持一文件一个单元。
	units := buildImportUnits(cands)

	if err := state.openJob(ctx, dir, len(units), opt); err != nil {
		return nil, err
	}
	state.recordIssues(ctx, issues)
	if len(units) == 0 {
		state.failJob(ctx, "没有可导入的文件（.txt / .epub / .zip）")
		return state.res, ErrNoFiles
	}

	workers := resolveWorkers(opt.Workers, len(units))
	outcomes := startUnitWorkers(ctx, st, parser.New(opt.ParserOpts), units, workers, opt)
	state.consume(ctx, outcomes, len(units), start, cb, opt)

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
	if len(opt.Sources) > 0 {
		// 重试：源可能已被删除/移动，逐条在候选层记失败，不在这里整体失败。
		return state, dir, nil, nil
	}

	if reason, verr := verifySource(dir); verr != nil {
		state.failJob(ctx, reason)
		return nil, "", nil, verr
	}

	files, err := scanFiles(ctx, dir, opt.Recursive)
	switch {
	case err != nil:
		state.failJob(ctx, err.Error())
		return nil, "", nil, err
	case len(files) == 0:
		state.failJob(ctx, "没有可导入的文件（.txt / .epub）")
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

// processFile 读取单文件、算 SHA256、幂等判定、解析、合集拆分，并补齐路径相关元信息。
func processFile(ctx context.Context, st *store.Store, p *parser.Parser, t fileTask, opt Options) parseOutcome {
	oc := parseOutcome{index: t.index, path: t.cand.name, cand: t.cand}
	data, err := os.ReadFile(t.cand.path)
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
	if exists && !opt.Force && !opt.Update {
		oc.skipped = true
		return oc
	}

	book, err := parseFileWithRules(p, t.cand.path, data, opt.ChapterRules)
	if err != nil {
		oc.err = fmt.Errorf("parse: %w", err)
		return oc
	}
	if book == nil {
		oc.err = errors.New("parse: empty result")
		return oc
	}
	fillBookSource(book, t.cand, data, sha)
	oc.books = splitBooks(book)
	// 词表打标在拆分后逐本算：每本书的正文范围不同，命中结果也可能不同。
	for _, b := range oc.books {
		b.AutoTags = matchTags(opt.TagRules, b)
	}
	return oc
}

// sha256Hex 返回文件内容的 SHA256 小写十六进制串（幂等键）。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// parseFile 按扩展名分发解析：.txt 走 parser；.epub 走 ebook，再转换成 ParsedBook。
func parseFile(p *parser.Parser, path string, data []byte) (*model.ParsedBook, error) {
	return parseFileWithRules(p, path, data, nil)
}

// parseFileWithRules 在 parseFile 之上注入自定义切章规则（空 rules 等价于 parseFile）。
func parseFileWithRules(p *parser.Parser, path string, data []byte, rules []parser.Rule) (*model.ParsedBook, error) {
	if FormatOf(path) != FormatEPUB {
		return p.ParseBytesWithRules(filepath.Base(path), data, rules)
	}
	eb, err := ebook.Open(filepath.Base(path), data)
	if err != nil {
		return nil, err
	}
	return epubToParsedBookWithRules(p, eb, rules), nil
}

// fillBookSource 用磁盘上的真实信息补齐书籍的来源字段。
//
// zip 条目的磁盘路径是解包出来的临时文件，来源路径必须用「包路径:包内路径」，
// 否则重导同一份包永远命中不了同一本书。
func fillBookSource(book *model.ParsedBook, cand candidate, data []byte, sha string) {
	book.SourcePath = cand.source
	book.SourceName = cand.name
	book.FileSize = int64(len(data))
	book.SHA256 = sha
	book.CategoryL1 = cand.categoryL1
	book.CategoryL2 = cand.categoryL2
}
