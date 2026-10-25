package importer

import (
	"context"
	"errors"
	"os"
	"sort"

	"norvels/internal/parser"
	"norvels/internal/store"
)

// dry-run 预检的报告结构（P1-B §2.3）：不写库、不建任务，只把「接下来会怎样」说清楚。
type Report struct {
	Truncated bool           `json:"truncated"`
	Files     ReportFiles    `json:"files"`
	Books     ReportBooks    `json:"books"`
	Sample    []ReportSample `json:"sample"`
	Errors    []ReportError  `json:"errors"`
}

// ReportFiles 是文件级分类计数（一个候选条目算一次）。
type ReportFiles struct {
	Total       int `json:"total"`
	WillImport  int `json:"willImport"`
	Duplicate   int `json:"duplicate"`
	Unsupported int `json:"unsupported"`
	Failed      int `json:"failed"`
}

// ReportBooks 是书籍级计数（合集拆分会一本书算多本）。
type ReportBooks struct {
	New       int `json:"new"`
	Duplicate int `json:"duplicate"`
	Split     int `json:"split"`
}

// ReportSample 是一条样例（按扫描顺序，最多 20 条）。
type ReportSample struct {
	Source    string `json:"source"`
	Title     string `json:"title"`
	Chapters  int    `json:"chapters"`
	Words     int    `json:"words"`
	Duplicate bool   `json:"duplicate"`
	Reason    string `json:"reason"`
}

// ReportError 是一条预检错误（不支持 / 解析失败，最多 50 条）。
type ReportError struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// 预检的规模上限：超过 200 个候选文件只解析前 200 并置 truncated。
const (
	dryRunMaxFiles  = 200
	dryRunSampleMax = 20
	dryRunErrorsMax = 50
)

// DryRun 同步预检：不建任务、不占锁、不写库（P1-B §2.2）。
//
// 判定口径与真实导入一致：SHA-256 / 源路径命中即 duplicate（追更时再加书名+作者指纹），
// 扩展名不支持即 unsupported，解析出 0 章记 failed。
func DryRun(ctx context.Context, st *store.Store, opt Options) (*Report, error) {
	if st == nil {
		return nil, errors.New("importer: nil store")
	}
	dir, err := resolveSourceDir(opt.Dir)
	if err != nil {
		return nil, err
	}
	if _, verr := verifySource(dir); verr != nil {
		return nil, verr
	}
	roots, unsupported, truncated, err := dryRunRoots(dir, opt.Recursive)
	if err != nil {
		return nil, err
	}
	rep := &Report{Truncated: truncated, Sample: []ReportSample{}, Errors: []ReportError{}}
	rep.Files.Unsupported = len(unsupported)
	rep.Files.Total = len(unsupported)
	for _, u := range unsupported {
		addReportError(rep, u, "unsupported_format")
	}
	scratch, err := makeScratch("")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	cands, issues := expandRoots(roots, scratch)
	rep.Files.Total += len(cands) + len(issues)
	for _, issue := range issues {
		countDryRunIssue(rep, issue)
	}
	if err := classifyDryRunCandidates(ctx, st, opt, cands, rep); err != nil {
		return nil, err
	}
	return rep, nil
}

// classifyDryRunCandidates 并发解析候选并按扫描顺序分类（sample 顺序稳定）。
func classifyDryRunCandidates(ctx context.Context, st *store.Store, opt Options, cands []candidate, rep *Report) error {
	if len(cands) == 0 {
		return nil
	}
	workers := resolveWorkers(opt.Workers, len(cands))
	outcomes := startParseWorkers(ctx, st, parser.New(opt.ParserOpts), cands, workers, opt)
	all := make([]parseOutcome, 0, len(cands))
	for oc := range outcomes {
		all = append(all, oc)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].index < all[j].index })
	for _, oc := range all {
		if err := classifyDryRunOutcome(ctx, st, opt, oc, rep); err != nil {
			return err
		}
	}
	return nil
}
