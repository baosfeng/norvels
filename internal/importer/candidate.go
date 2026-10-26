package importer

// 候选条目层：把「磁盘上的文件」统一成候选，zip 先展开成普通文件再走同一条解析链路。

import (
	"fmt"
	"os"
	"path/filepath"
)

// candidate 是一个待解析条目。
type candidate struct {
	path   string // 磁盘上可读的路径（zip 条目 = 解包后的临时文件）
	source string // 写库的来源路径（普通文件 = path；zip 条目 = 包路径 + ":" + 包内路径）
	name   string // 对外名称（普通文件 = 文件名；zip 条目 = 包名 + ":" + 包内路径）
	// 分类（0015 起）：由 annotateCategories 按相对导入根的前两级目录推断，
	// 空串表示未分类（文件直接放在根下 / 目录以下划线开头）。
	categoryL1 string
	categoryL2 string
}

// expandIssue 是解包阶段的一条问题：整包失败或单个条目不支持，都按文件级终态记录。
type expandIssue struct {
	source string // 可重放源（zip 条目 = 包绝对路径:包内路径；整包失败 = 包路径）
	path   string // 展示路径
	kind   string // unsupported（格式不支持）/ failed（整包或条目不可用）
	reason string
}

// 解包问题的两类终态（与 store.ImportFileStatus* 取值一致）。
const (
	issueKindUnsupported = "unsupported"
	issueKindFailed      = "failed"
)

// fileCandidate 把磁盘文件包装成候选。
func fileCandidate(path string) candidate {
	return candidate{path: path, source: path, name: filepath.Base(path)}
}

// expandRoots 把候选文件里的 .zip 展开成条目候选（其余格式原样返回）。
//
// zip 级失败（打不开 / 超限 / 空包 / 危险条目）与不支持条目都落到 issues 里，
// 由调用方逐条记进 import_errors；同批其他文件不受影响。
func expandRoots(roots []string, scratch string) ([]candidate, []expandIssue) {
	cands := make([]candidate, 0, len(roots))
	issues := make([]expandIssue, 0, len(roots))
	for _, root := range roots {
		if FormatOf(root) != FormatZIP {
			cands = append(cands, fileCandidate(root))
			continue
		}
		got, sub, err := extractZIP(root, scratch)
		if err != nil {
			issues = append(issues, expandIssue{
				source: root, path: filepath.Base(root), kind: issueKindFailed, reason: err.Error(),
			})
			continue
		}
		cands = append(cands, got...)
		issues = append(issues, sub...)
	}
	return cands, issues
}

// makeScratch 建一个解包临时目录（调用方负责删除）。
func makeScratch(parent string) (string, error) {
	dir, err := os.MkdirTemp(parent, "unzip-")
	if err != nil {
		return "", fmt.Errorf("importer: 创建解包临时目录失败: %w", err)
	}
	return dir, nil
}
