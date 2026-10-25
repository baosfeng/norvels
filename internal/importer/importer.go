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
	"strings"
	"time"
	"unicode"

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

// ErrNoFiles 表示目录中没有可导入的文件（支持格式见 SupportedExt）。
var ErrNoFiles = errors.New("importer: 没有可导入的文件（.txt / .epub）")

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

	book, err := parseFile(p, t.path, data)
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

// parseFile 按扩展名分发解析：.txt 走 parser；.epub 走 ebook，再转换成 ParsedBook。
func parseFile(p *parser.Parser, path string, data []byte) (*model.ParsedBook, error) {
	if FormatOf(path) != FormatEPUB {
		return p.ParseBytes(filepath.Base(path), data)
	}
	eb, err := ebook.Open(filepath.Base(path), data)
	if err != nil {
		return nil, err
	}
	return epubToParsedBook(p, eb), nil
}

// epubToParsedBook 把 ebook.Book 转成 store 认识的 ParsedBook：元数据与封面原样搬运，
// 章节按 spine 顺序编号；「整本一页」的大书改走 txt 切章策略（spec §4）。
func epubToParsedBook(p *parser.Parser, eb *ebook.Book) *model.ParsedBook {
	book := &model.ParsedBook{
		Title:        eb.Title,
		Author:       eb.Author,
		Intro:        eb.Intro,
		Language:     eb.Language,
		Publisher:    eb.Publisher,
		ISBN:         eb.ISBN,
		PubDate:      eb.PubDate,
		Series:       eb.Series,
		SeriesIndex:  eb.SeriesIndex,
		Subjects:     eb.Subjects,
		SourceFormat: FormatEPUB,
		Encoding:     "utf-8",
		Warnings:     eb.Warnings,
	}
	if eb.Cover != nil {
		book.Cover = &model.CoverImage{MIME: eb.Cover.MIME, Bytes: eb.Cover.Bytes}
	}
	if eb.NeedsSplit() {
		splitByParser(p, book, eb)
	} else {
		book.Chapters = epubChapters(eb.Chapters)
	}
	book.WordCount = totalWords(book.Chapters)
	return book
}

// splitByParser 把「spine 只有一两个 XHTML」的全文交给 txt 解析器重新切章，
// 复用 di-unit 等既有策略；切章失败时退回 spine 原章节，不让一本书因此导入失败。
func splitByParser(p *parser.Parser, book *model.ParsedBook, eb *ebook.Book) {
	parsed, err := p.ParseBytes(book.Title+".txt", []byte(eb.FullText()))
	if err != nil || parsed == nil || len(parsed.Chapters) == 0 {
		book.Chapters = epubChapters(eb.Chapters)
		book.Warnings = append(book.Warnings, "全文切章失败，已按 spine 原章节导入")
		return
	}
	book.Chapters = parsed.Chapters
	book.Warnings = append(book.Warnings, keepSplitWarnings(parsed.Warnings)...)
	book.Warnings = append(book.Warnings, "spine 只有一个正文文档，已按 txt 切章策略重新分章")
}

// keepSplitWarnings 过滤掉 parser 对书名/作者推断的告警：切章用的是一个临时文件名，
// 那两条只反映临时文件名的推断失败，EPUB 元数据才是权威来源。
func keepSplitWarnings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, w := range in {
		if w == "author_missing" || strings.HasPrefix(w, "未能推断书名") {
			continue
		}
		out = append(out, w)
	}
	return out
}

// epubChapters 把 spine 章节转成模型章节；EPUB 没有卷的概念，VolumeIdx 统一为 -1。
func epubChapters(chapters []ebook.Chapter) []model.ParsedChapter {
	out := make([]model.ParsedChapter, 0, len(chapters))
	for i, c := range chapters {
		out = append(out, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: -1,
			Title:     c.Title,
			Content:   c.Content,
			WordCount: countTextWords(c.Content),
		})
	}
	return out
}

// totalWords 汇总全文字数。
func totalWords(chapters []model.ParsedChapter) int {
	total := 0
	for _, c := range chapters {
		total += c.WordCount
	}
	return total
}

// countTextWords 统计纯文本字数：CJK 逐字计 1，连续拉丁字母/数字计 1 个词。
// 与 parser 的字数口径一致，保证 EPUB 与 TXT 的统计数据可比。
func countTextWords(s string) int {
	n, inWord := 0, false
	for _, r := range s {
		switch {
		case isCJKRune(r):
			n++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				n++
				inWord = true
			}
		default:
			inWord = false
		}
	}
	return n
}

// isCJKRune 判断是否为需要逐字计数的表意文字（汉字/假名/谚文）。
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}

// fillBookSource 用磁盘上的真实信息补齐书籍的来源字段。
func fillBookSource(book *model.ParsedBook, path string, data []byte, sha string) {
	book.SourcePath = path
	book.SourceName = filepath.Base(path)
	book.FileSize = int64(len(data))
	book.SHA256 = sha
}
