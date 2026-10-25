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
	// ChapterRules 是调用方传入的自定义切章规则，按顺序优先于内置切章策略、命中即作为
	// 章节边界。api 的两个导入入口都在请求线程里读 settings 再注入（loadChapterRules）；为空
	// 表示只用内置策略——importer 自己不读 settings，也不依赖 store 的设置表。
	ChapterRules []parser.Rule
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
	cands, issues := expandRoots(files, scratch)
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

// epubToParsedBook 把 ebook.Book 转成 store 认识的 ParsedBook：元数据与封面原样搬运，
// 章节按 spine 顺序编号；「整本一页」的大书改走 txt 切章策略（spec §4）。
func epubToParsedBook(p *parser.Parser, eb *ebook.Book) *model.ParsedBook {
	return epubToParsedBookWithRules(p, eb, nil)
}

// epubToParsedBookWithRules 是 epubToParsedBook 的带规则版本。
func epubToParsedBookWithRules(p *parser.Parser, eb *ebook.Book, rules []parser.Rule) *model.ParsedBook {
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
		// 「整本一页」的 epub 交给 txt 切章，目录层级随之失效，按平铺口径记录。
		splitByParserWithRules(p, book, eb, rules)
		if len(eb.Volumes) > 0 {
			book.Warnings = append(book.Warnings, "EPUB 目录层级在全文重切后失效，已按平铺导入")
		}
	} else {
		book.Chapters = epubChapters(eb.Chapters)
		book.Volumes = epubVolumes(eb.Volumes)
	}
	book.WordCount = totalWords(book.Chapters)
	return book
}

// splitByParser 把「spine 只有一两个 XHTML」的全文交给 txt 解析器重新切章，
// 复用 di-unit 等既有策略；切章失败时退回 spine 原章节，不让一本书因此导入失败。
func splitByParser(p *parser.Parser, book *model.ParsedBook, eb *ebook.Book) {
	splitByParserWithRules(p, book, eb, nil)
}

// splitByParserWithRules 是 splitByParser 的带规则版本。
func splitByParserWithRules(p *parser.Parser, book *model.ParsedBook, eb *ebook.Book, rules []parser.Rule) {
	parsed, err := p.ParseBytesWithRules(book.Title+".txt", []byte(eb.FullText()), rules)
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

// epubChapters 把 spine 章节转成模型章节。
//
// ebook 的 VolumeIdx 是 1 起（0=未分卷），model 是 0 起（-1=未分卷），这里做唯一一次转换。
func epubChapters(chapters []ebook.Chapter) []model.ParsedChapter {
	out := make([]model.ParsedChapter, 0, len(chapters))
	for i, c := range chapters {
		out = append(out, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: modelVolumeIdx(c.VolumeIdx),
			Title:     c.Title,
			Content:   c.Content,
			WordCount: countTextWords(c.Content),
		})
	}
	return out
}

// modelVolumeIdx 把 ebook 的 1 起卷下标转成 model 的 0 起（0 → -1 未分卷）。
func modelVolumeIdx(ebookVolumeIdx int) int {
	if ebookVolumeIdx <= 0 {
		return -1
	}
	return ebookVolumeIdx - 1
}

// epubVolumes 把目录层级转成卷分组（EPUB3 nav.xhtml / EPUB2 toc.ncx 解析而来）。
func epubVolumes(vols []ebook.Volume) []model.ParsedVolume {
	if len(vols) == 0 {
		return nil
	}
	out := make([]model.ParsedVolume, 0, len(vols))
	for i, v := range vols {
		out = append(out, model.ParsedVolume{Idx: i, Title: v.Title})
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
//
// zip 条目的磁盘路径是解包出来的临时文件，来源路径必须用「包路径:包内路径」，
// 否则重导同一份包永远命中不了同一本书。
func fillBookSource(book *model.ParsedBook, cand candidate, data []byte, sha string) {
	book.SourcePath = cand.source
	book.SourceName = cand.name
	book.FileSize = int64(len(data))
	book.SHA256 = sha
}
