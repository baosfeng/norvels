package importer

import (
	"os"
	"path/filepath"
	"strings"
)

// zipExtract 缓存一次整包解包的结果，避免重试同一包的多个条目时反复解压。
type zipExtract struct {
	cands  []candidate
	issues []expandIssue
	err    error
}

// sourcesToCandidates 把可重放源解析成候选（单文件重试专用，P1-B §2.1）。
//
// 源形态：目录文件 = 绝对路径；zip 条目 = 包绝对路径:包内路径；合集拆分段沿用其伪路径。
// 源已不存在 / 条目缺失 / 上传源（upload://）无法重放 → 生成 issue（failed），
// 由调用方记终态，不中断整批。
func sourcesToCandidates(sources []string, scratch string) ([]candidate, []expandIssue) {
	cands := make([]candidate, 0, len(sources))
	issues := make([]expandIssue, 0, 4)
	cache := make(map[string]zipExtract, 2)
	seen := make(map[string]bool, len(sources))
	for _, raw := range sources {
		src := strings.TrimSpace(raw)
		if src == "" || seen[src] {
			continue
		}
		seen[src] = true
		if zipPath, inner, ok := splitZipSource(src); ok {
			cands, issues = appendZipEntry(cands, issues, cache, zipPath, inner, scratch)
			continue
		}
		if fi, err := os.Stat(src); err == nil && !fi.IsDir() && SupportedExt(fi.Name()) {
			cands = append(cands, fileCandidate(src))
			continue
		}
		issues = append(issues, expandIssue{
			source: src, path: displayPathOfSource(src), kind: issueKindFailed,
			reason: "源文件已不存在或不可导入，无法重试",
		})
	}
	return cands, issues
}

// splitZipSource 把「包路径:包内路径」拆开；不是 zip 条目形态时返回 ok=false。
//
// 用最后一个冒号切：包内路径本身可能含冒号（罕见），包路径在冒号之前。
func splitZipSource(src string) (zipPath, inner string, ok bool) {
	i := strings.LastIndex(src, ":")
	if i <= 0 || i == len(src)-1 {
		return "", "", false
	}
	zipPath, inner = src[:i], src[i+1:]
	if FormatOf(zipPath) != FormatZIP {
		return "", "", false
	}
	if _, err := os.Stat(zipPath); err != nil {
		return "", "", false
	}
	return zipPath, inner, true
}

// appendZipEntry 从（缓存的）整包解包结果里取指定条目。
func appendZipEntry(cands []candidate, issues []expandIssue, cache map[string]zipExtract,
	zipPath, inner, scratch string) ([]candidate, []expandIssue) {
	got, ok := cache[zipPath]
	if !ok {
		extracted, subIssues, err := extractZIP(zipPath, scratch)
		got = zipExtract{cands: extracted, issues: subIssues, err: err}
		cache[zipPath] = got
	}
	wanted := zipPath + ":" + inner
	if got.err != nil {
		return cands, append(issues, expandIssue{
			source: wanted, path: filepath.Base(zipPath) + ":" + inner,
			kind: issueKindFailed, reason: "重新解包失败: " + got.err.Error(),
		})
	}
	for _, c := range got.cands {
		if c.source == wanted {
			return append(cands, c), issues
		}
	}
	for _, issue := range got.issues {
		if issue.source == wanted {
			return cands, append(issues, issue)
		}
	}
	return cands, append(issues, expandIssue{
		source: wanted, path: filepath.Base(zipPath) + ":" + inner,
		kind: issueKindFailed, reason: "包内条目已不存在，无法重试",
	})
}

// displayPathOfSource 生成展示路径：zip 条目取「包名:包内路径」，其余取 base 名。
func displayPathOfSource(src string) string {
	if i := strings.LastIndex(src, ":"); i > 0 {
		if zipPath := src[:i]; FormatOf(zipPath) == FormatZIP {
			return filepath.Base(zipPath) + ":" + src[i+1:]
		}
	}
	return filepath.Base(src)
}
