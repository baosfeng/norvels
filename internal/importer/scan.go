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

// scanFiles 收集候选 txt（跳过隐藏文件），结果按路径排序保证可复现。
func scanFiles(ctx context.Context, root string, recursive bool) ([]string, error) {
	files := make([]string, 0, 256)
	var err error
	if recursive {
		err = walkTxtFiles(ctx, root, &files)
	} else {
		err = readTopLevelTxt(root, &files)
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// walkTxtFiles 递归收集候选 txt；单个条目不可读时跳过，不影响整批。
func walkTxtFiles(ctx context.Context, root string, files *[]string) error {
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

// readTopLevelTxt 只收集顶层目录下的候选 txt。
func readTopLevelTxt(root string, files *[]string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("importer: read dir: %w", err)
	}
	for _, e := range entries {
		appendCandidate(files, filepath.Join(root, e.Name()), e)
	}
	return nil
}

// appendCandidate 把非目录、非隐藏的 .txt 追加到候选列表。
func appendCandidate(files *[]string, path string, d fs.DirEntry) {
	if d.IsDir() || !isTxtCandidate(d.Name()) {
		return
	}
	*files = append(*files, path)
}

// isTxtCandidate 判断文件名是否是候选：非隐藏且扩展名为 .txt（大小写不敏感）。
func isTxtCandidate(name string) bool {
	return !strings.HasPrefix(name, ".") && strings.EqualFold(filepath.Ext(name), ".txt")
}
