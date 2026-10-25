package importer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"norvels/internal/model"
	"norvels/internal/store"
)

// dryRunRoots 决定预检要解析的文件：dir 可以是目录，也可以是单个可导入文件
// （与 POST /api/import 的 dir 语义一致）。
func dryRunRoots(dir string, recursive bool) ([]string, []string, bool, error) {
	single, err := isSingleImportableFile(dir)
	if err != nil {
		return nil, nil, false, err
	}
	if single {
		return []string{dir}, nil, false, nil
	}
	return collectDryRunRoots(dir, recursive)
}

// collectDryRunRoots 收集预检要解析的文件：支持格式前 200 个（超出置 truncated），
// 不支持的扩展名只计数、不解析。
func collectDryRunRoots(dir string, recursive bool) (roots, unsupported []string, truncated bool, err error) {
	c := &dryRunCollector{
		roots:       make([]string, 0, 32),
		unsupported: make([]string, 0, 8),
	}
	if recursive {
		err = c.walk(dir)
	} else {
		err = c.readTop(dir)
	}
	return c.roots, c.unsupported, c.truncated, err
}

// dryRunCollector 累积预检要解析的文件与不支持的文件（超过 200 个候选即截断）。
type dryRunCollector struct {
	roots       []string
	unsupported []string
	truncated   bool
}

// add 把一个目录项归类：支持格式进 roots（达到上限置 truncated），其余进 unsupported。
func (c *dryRunCollector) add(path string, d fs.DirEntry) {
	name := d.Name()
	if d.IsDir() || strings.HasPrefix(name, ".") {
		return
	}
	if !SupportedExt(name) {
		c.unsupported = append(c.unsupported, path)
		return
	}
	if len(c.roots) >= dryRunMaxFiles {
		c.truncated = true
		return
	}
	c.roots = append(c.roots, path)
}

// readTop 只扫顶层目录。
func (c *dryRunCollector) readTop(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("importer: read dir: %w", err)
	}
	for _, e := range entries {
		c.add(filepath.Join(dir, e.Name()), e)
	}
	return nil
}

// walk 递归扫描；达到上限后提前结束遍历。
func (c *dryRunCollector) walk(dir string) error {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil || path == dir {
			return nil
		}
		c.add(path, d)
		if c.truncated {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("importer: scan dir: %w", err)
	}
	return nil
}

// countDryRunIssue 把解包阶段的问题计入报告（不支持的条目与整包失败分开计）。
func countDryRunIssue(rep *Report, issue expandIssue) {
	if issue.kind == issueKindUnsupported {
		rep.Files.Unsupported++
	} else {
		rep.Files.Failed++
	}
	addReportError(rep, issue.source, issue.reason)
}

// classifyDryRunOutcome 给单个候选条目分类：失败 / 重复 / 将导入。
func classifyDryRunOutcome(ctx context.Context, st *store.Store, opt Options, oc parseOutcome, rep *Report) error {
	switch {
	case oc.err != nil:
		rep.Files.Failed++
		addReportError(rep, oc.cand.source, oc.err.Error())
		return nil
	case oc.skipped:
		rep.Files.Duplicate++
		rep.Books.Duplicate++
		addReportSample(rep, oc, true, "duplicate")
		return nil
	case len(oc.books) == 0:
		rep.Files.Failed++
		addReportError(rep, oc.cand.source, "parse: empty result")
		return nil
	}
	return classifyDryRunBooks(ctx, st, opt, oc, rep)
}

// classifyDryRunBooks 逐本判断是否已存在（与 SaveBook 的匹配顺序一致）。
func classifyDryRunBooks(ctx context.Context, st *store.Store, opt Options, oc parseOutcome, rep *Report) error {
	newBooks, dupBooks := 0, 0
	for _, b := range oc.books {
		dup, err := dryRunBookDuplicate(ctx, st, opt, b)
		if err != nil {
			return err
		}
		if dup {
			dupBooks++
			rep.Books.Duplicate++
			continue
		}
		newBooks++
		rep.Books.New++
	}
	if len(oc.books) > 1 {
		rep.Books.Split++
	}
	if newBooks > 0 {
		rep.Files.WillImport++
		addReportSample(rep, oc, false, "")
		return nil
	}
	rep.Files.Duplicate++
	addReportSample(rep, oc, true, "duplicate")
	return nil
}

// dryRunBookDuplicate 判断一本书在库里是否已存在；force 覆盖时不算重复。
func dryRunBookDuplicate(ctx context.Context, st *store.Store, opt Options, b *model.ParsedBook) (bool, error) {
	if opt.Force && !opt.Update {
		return false, nil
	}
	return st.BookExistsForImport(ctx, b.SourcePath, b.SHA256, b.Title, b.Author, opt.Update)
}

// addReportSample 追加一条样例（最多 20 条，按扫描顺序）。
func addReportSample(rep *Report, oc parseOutcome, duplicate bool, reason string) {
	if len(rep.Sample) >= dryRunSampleMax {
		return
	}
	s := ReportSample{Source: oc.cand.source, Duplicate: duplicate, Reason: reason}
	if len(oc.books) > 0 {
		s.Title = oc.books[0].Title
		s.Chapters = len(oc.books[0].Chapters)
		s.Words = oc.books[0].WordCount
	}
	rep.Sample = append(rep.Sample, s)
}

// addReportError 追加一条错误（最多 50 条）。
func addReportError(rep *Report, source, reason string) {
	if len(rep.Errors) >= dryRunErrorsMax {
		return
	}
	rep.Errors = append(rep.Errors, ReportError{Source: source, Reason: reason})
}
