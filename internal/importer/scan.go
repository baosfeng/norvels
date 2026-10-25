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
//
// root 可以是目录，也可以是单个 .txt 文件；后者直接作为唯一候选返回，
// recursive 对它没有意义（一个文件没有子目录可递归）。
func scanFiles(ctx context.Context, root string, recursive bool) ([]string, error) {
	single, err := isSingleTxtFile(root)
	if err != nil {
		return nil, err
	}
	if single {
		return []string{root}, nil
	}

	files := make([]string, 0, 256)
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

// isSingleTxtFile 判断 path 是否是一个可直接导入的 .txt 文件。
//
// 目录返回 false；存在但不是 .txt 的文件返回错误——否则用户会看到
// "no .txt files found"，误以为是自己目录选错了，实际是扩展名不对。
func isSingleTxtFile(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("importer: stat: %w", err)
	}
	if fi.IsDir() {
		return false, nil
	}
	if !IsTxtFile(fi.Name()) {
		return false, fmt.Errorf("importer: %s 不是 .txt 文件", filepath.Base(path))
	}
	return true, nil
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

// isTxtCandidate 判断文件名是否是候选：非隐藏且扩展名为 .txt。
func isTxtCandidate(name string) bool {
	return !strings.HasPrefix(name, ".") && IsTxtFile(name)
}

// IsTxtFile 判断文件名是否是 .txt（大小写不敏感）。
//
// 导出给 api 层复用：POST /api/import 需要在建任务前就把「路径既不是目录
// 也不是 txt」挡回去，判定口径必须与扫描器一致。
func IsTxtFile(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".txt")
}
