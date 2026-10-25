package importer

// ZIP 批量导入的解包层。
//
// 四道闸（docs/specs/2026-10-25-import-p1.md §2.4）：条目 ≤1000、解压总量 ≤512 MiB、
// 单条目 ≤64 MiB、拒绝绝对路径 / .. 穿越 / 符号链接 / 非普通文件。
// 嵌套 zip 一律按「不支持」拒绝：批量导入不做递归解包，避免无限展开。
//
// 失败语义：「解包失败不建半本书」——整包要么全部展开，要么整包丢弃，
// 调用方把整包失败或单条目不支持逐条记进 import_errors。

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// 解包上限；改这里要同步 docs/api.md 与 docs/specs。
const (
	maxZipEntries    = 1000
	maxZipTotalBytes = 512 << 20
	maxZipEntryBytes = 64 << 20
)

// zip 解包的哨兵错误，供上传接口映射成错误码（invalid_zip / zip_too_large / empty_zip）。
var (
	// ErrZIPInvalid 表示不是合法的 zip 包，或含越界路径 / 符号链接这类危险条目。
	ErrZIPInvalid = errors.New("importer: 不合法的 zip 包")
	// ErrZIPTooLarge 表示解包超过条目数、单条目或解压总量上限。
	ErrZIPTooLarge = errors.New("importer: zip 解包超过上限")
	// ErrZIPEmpty 表示 zip 里没有任何可导入条目。
	ErrZIPEmpty = errors.New("importer: zip 里没有可导入的文件")
)

// ZIPEntry 是 zip 包里的一个条目：Format 为空表示不支持（Reason 给出人话原因）。
type ZIPEntry struct {
	Name   string
	Format string
	Reason string
}

// zipEntry 是已通过四道闸的一个条目。
type zipEntry struct {
	name string
	f    *zip.File
}

// ListZIP 只读 central directory，列出 zip 里的条目并套用四道闸。
//
// 上传接口在收完文件、建任务之前调用：accepted 用条目名「包名.zip:包内路径」，
// rejected 用 Reason 说明为什么不导入（不支持的格式、嵌套 zip 等）。
func ListZIP(zipPath string) ([]ZIPEntry, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrZIPInvalid, err)
	}
	defer func() { _ = zr.Close() }()
	entries, err := planZIP(zr.File)
	if err != nil {
		return nil, err
	}
	out := make([]ZIPEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, classifyZipEntry(e.name))
	}
	return out, nil
}

// classifyZipEntry 判定包内条目能否导入，给不出来时带上人话原因。
func classifyZipEntry(name string) ZIPEntry {
	if strings.EqualFold(filepath.Ext(name), ".zip") {
		return ZIPEntry{Name: name, Reason: "不支持嵌套 zip（包内再打包）"}
	}
	if format := FormatOf(name); format != "" {
		return ZIPEntry{Name: name, Format: format}
	}
	ext := strings.ToLower(path.Ext(name))
	if ext == "" {
		return ZIPEntry{Name: name, Reason: "暂不支持无扩展名的文件"}
	}
	return ZIPEntry{Name: name, Reason: "暂不支持 " + ext}
}

// planZIP 对整包做四道闸校验，返回可读取的条目（目录、隐藏文件与 __MACOSX 垃圾被跳过）。
func planZIP(files []*zip.File) ([]zipEntry, error) {
	if len(files) > maxZipEntries {
		return nil, fmt.Errorf("%w: 条目数 %d 超过 %d", ErrZIPTooLarge, len(files), maxZipEntries)
	}
	out := make([]zipEntry, 0, len(files))
	var total uint64
	for _, f := range files {
		name, err := normalizeZipName(f.Name)
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		if err := checkZipFile(f); err != nil {
			return nil, err
		}
		total += f.UncompressedSize64
		if total > maxZipTotalBytes {
			return nil, fmt.Errorf("%w: 解压总量超过 %d MiB", ErrZIPTooLarge, maxZipTotalBytes>>20)
		}
		out = append(out, zipEntry{name: name, f: f})
	}
	if len(out) == 0 {
		return nil, ErrZIPEmpty
	}
	return out, nil
}

// normalizeZipName 归一化包内路径：目录条目与隐藏垃圾返回空串，越界路径返回错误。
func normalizeZipName(raw string) (string, error) {
	name := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if name == "" || strings.HasSuffix(name, "/") {
		return "", nil
	}
	if isAbsZipPath(name) {
		return "", fmt.Errorf("%w: 条目 %q 是绝对路径", ErrZIPInvalid, raw)
	}
	clean := path.Clean(strings.TrimPrefix(name, "./"))
	if clean == "." || hasParentPart(clean) {
		return "", fmt.Errorf("%w: 条目 %q 含路径穿越", ErrZIPInvalid, raw)
	}
	if zipJunk(clean) {
		return "", nil
	}
	return clean, nil
}

// isAbsZipPath 判断包内路径是否是绝对路径（含 Windows 盘符写法）。
func isAbsZipPath(name string) bool {
	return strings.HasPrefix(name, "/") || strings.Contains(name, ":")
}

// hasParentPart 判断归一化后的路径里是否还有 .. 段。
func hasParentPart(clean string) bool {
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// zipJunk 判断是否打包工具附带的垃圾条目（隐藏文件、__MACOSX 资源分叉）。
func zipJunk(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || part == "__MACOSX" {
			return true
		}
	}
	return false
}

// checkZipFile 拒绝符号链接、非普通文件与超限条目。
func checkZipFile(f *zip.File) error {
	mode := f.Mode()
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: 条目 %q 是符号链接", ErrZIPInvalid, f.Name)
	}
	if !mode.IsRegular() {
		return fmt.Errorf("%w: 条目 %q 不是普通文件", ErrZIPInvalid, f.Name)
	}
	if f.UncompressedSize64 > maxZipEntryBytes {
		return fmt.Errorf("%w: 条目 %q 解压后 %d 字节，超过 %d MiB",
			ErrZIPTooLarge, f.Name, f.UncompressedSize64, maxZipEntryBytes>>20)
	}
	return nil
}

// extractZIP 展开一个 zip：可导入条目落到 root 下的独立子目录，不支持条目按 issue 返回。
// 任一环节失败整包丢弃（删掉子目录），不会留下「半本书」。
func extractZIP(zipPath, root string) ([]candidate, []expandIssue, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrZIPInvalid, err)
	}
	defer func() { _ = zr.Close() }()
	entries, err := planZIP(zr.File)
	if err != nil {
		return nil, nil, err
	}
	dst, err := os.MkdirTemp(root, "zip-")
	if err != nil {
		return nil, nil, fmt.Errorf("importer: 创建解包目录失败: %w", err)
	}
	cands, issues, err := writeZipEntries(entries, dst, zipPath)
	if err != nil || len(cands) == 0 {
		_ = os.RemoveAll(dst)
		if err != nil {
			return nil, nil, err
		}
		return nil, issues, ErrZIPEmpty
	}
	return cands, issues, nil
}

// writeZipEntries 逐个解出可导入条目；不支持条目转成 issue，不中断整包。
func writeZipEntries(entries []zipEntry, dst, zipPath string) ([]candidate, []expandIssue, error) {
	base := filepath.Base(zipPath)
	cands := make([]candidate, 0, len(entries))
	issues := make([]expandIssue, 0, 4)
	seen := make(map[string]int, len(entries))
	var total int64
	for _, e := range entries {
		entry := classifyZipEntry(e.name)
		if entry.Format == "" {
			issues = append(issues, expandIssue{path: base + ":" + e.name, reason: entry.Reason})
			continue
		}
		target, err := uniqueEntryPath(dst, e.name, seen)
		if err != nil {
			return nil, nil, err
		}
		n, err := copyZipEntry(e.f, target)
		if err != nil {
			return nil, nil, err
		}
		total += n
		if total > maxZipTotalBytes {
			return nil, nil, fmt.Errorf("%w: 解压总量超过 %d MiB", ErrZIPTooLarge, maxZipTotalBytes>>20)
		}
		cands = append(cands, candidate{
			path:   target,
			source: zipPath + ":" + e.name,
			name:   base + ":" + e.name,
		})
	}
	return cands, issues, nil
}

// uniqueEntryPath 把包内路径压平成解包目录里的唯一文件名（保留扩展名）。
func uniqueEntryPath(dst, inner string, seen map[string]int) (string, error) {
	base := filepath.Base(inner)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	n := seen[base]
	seen[base] = n + 1
	if n > 0 {
		base = fmt.Sprintf("%s-%d%s", stem, n, ext)
	}
	return filepath.Join(dst, base), nil
}

// copyZipEntry 解出一个条目，返回写入字节数；声明大小撒谎的条目由 LimitReader 兜住。
func copyZipEntry(f *zip.File, dst string) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("importer: 解压 %q 失败: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("importer: 写解包文件失败: %w", err)
	}
	n, err := io.Copy(out, io.LimitReader(rc, maxZipEntryBytes+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, fmt.Errorf("importer: 解压 %q 失败: %w", f.Name, err)
	}
	if n > maxZipEntryBytes {
		return 0, fmt.Errorf("%w: 条目 %q 解压后超过 %d MiB", ErrZIPTooLarge, f.Name, maxZipEntryBytes>>20)
	}
	return n, nil
}
