package importer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// scanFiles 收集候选文件（.txt/.epub/.zip，跳过隐藏文件），结果按路径排序保证可复现。
//
// root 可以是目录，也可以是单个可导入文件；后者直接作为唯一候选返回，
// recursive 对它没有意义（一个文件没有子目录可递归）。
func scanFiles(ctx context.Context, root string, recursive bool) ([]string, error) {
	single, err := isSingleImportableFile(root)
	if err != nil {
		return nil, err
	}
	if single {
		return []string{root}, nil
	}

	files := make([]string, 0, 256)
	if recursive {
		err = walkCandidates(ctx, root, &files)
	} else {
		err = readTopLevelCandidates(root, &files)
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// isSingleImportableFile 判断 path 是否是一个可直接导入的文件（.txt/.epub/.zip）。
//
// 目录返回 false；存在但不是可导入文件返回错误——否则用户会看到
// 「没有可导入的文件」，误以为是自己目录选错了，实际是扩展名不对。
func isSingleImportableFile(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("importer: stat: %w", err)
	}
	if fi.IsDir() {
		return false, nil
	}
	if !SupportedExt(fi.Name()) {
		return false, fmt.Errorf("importer: %s 不是 .txt / .epub / .zip 文件", filepath.Base(path))
	}
	return true, nil
}

// walkCandidates 递归收集候选文件；单个条目不可读时跳过，不影响整批。
func walkCandidates(ctx context.Context, root string, files *[]string) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if path == root {
			return nil
		}
		appendCandidate(files, path, d)
		return nil
	})
	if err != nil {
		return fmt.Errorf("importer: scan dir: %w", err)
	}
	return nil
}

// readTopLevelCandidates 只收集顶层目录下的候选文件。
func readTopLevelCandidates(root string, files *[]string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("importer: read dir: %w", err)
	}
	for _, e := range entries {
		appendCandidate(files, filepath.Join(root, e.Name()), e)
	}
	return nil
}

// appendCandidate 把非目录、非隐藏的可导入文件追加到候选列表。
func appendCandidate(files *[]string, path string, d fs.DirEntry) {
	if d.IsDir() || !isImportableCandidate(d.Name()) {
		return
	}
	*files = append(*files, path)
}

// isImportableCandidate 判断文件名是否是候选：非隐藏且扩展名可导入（TXT/EPUB）。
func isImportableCandidate(name string) bool {
	return !strings.HasPrefix(name, ".") && SupportedExt(name)
}
