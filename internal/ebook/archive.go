package ebook

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// 解包上限（spec §4）：Zip 炸弹防护。条目数、解压总量、单条目三档一起卡。
const (
	maxEntries    = 2000
	maxTotalBytes = 200 << 20
	maxEntryBytes = 16 << 20
)

// archive 是 EPUB 字节的只读视图：按归一化路径索引 zip 条目，并累计已解压字节数。
type archive struct {
	files map[string]*zip.File
	total int64 // 已解压字节数，用于解压总量上限
}

// openArchive 打开 zip 并建立条目索引；条目过多直接拒绝（一个字节都不解压）。
func openArchive(data []byte) (*archive, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("ebook: 不是合法的 EPUB（zip 无法打开）: %w", err)
	}
	if len(zr.File) > maxEntries {
		return nil, fmt.Errorf("ebook: 条目数 %d 超过上限 %d", len(zr.File), maxEntries)
	}
	a := &archive{files: make(map[string]*zip.File, len(zr.File))}
	for _, f := range zr.File {
		if key := normalizeEntry(f.Name); key != "" {
			a.files[key] = f
		}
	}
	return a, nil
}

// normalizeEntry 归一化条目路径；目录条目、空名与越界路径返回 ""（直接忽略）。
func normalizeEntry(name string) string {
	raw := strings.ReplaceAll(name, "\\", "/")
	if raw == "" || strings.HasSuffix(raw, "/") {
		return ""
	}
	cleaned := path.Clean(strings.TrimPrefix(raw, "./"))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return ""
	}
	return cleaned
}

// has 判断条目是否存在。
func (a *archive) has(name string) bool {
	_, _, err := a.open(name)
	return err == nil
}

// open 按路径查条目；大小写不一致时做一次兜底匹配（部分 epub 的 href 与条目名不同名）。
func (a *archive) open(name string) (*zip.File, string, error) {
	key := normalizeEntry(name)
	if key == "" {
		return nil, "", fmt.Errorf("ebook: 非法条目名 %q", name)
	}
	if f, ok := a.files[key]; ok {
		return f, key, nil
	}
	lower := strings.ToLower(key)
	for k, f := range a.files {
		if strings.ToLower(k) == lower {
			return f, k, nil
		}
	}
	return nil, "", fmt.Errorf("ebook: 找不到条目 %q", name)
}

// read 解压单个条目并对单条目/总量上限做校验；limit 会被夹到 maxEntryBytes。
func (a *archive) read(name string, limit int64) ([]byte, string, error) {
	f, key, err := a.open(name)
	if err != nil {
		return nil, "", err
	}
	if limit <= 0 || limit > maxEntryBytes {
		limit = maxEntryBytes
	}
	if f.UncompressedSize64 > uint64(limit) {
		return nil, key, fmt.Errorf("ebook: 条目 %q 解压后 %d 字节，超过上限 %d", key, f.UncompressedSize64, limit)
	}
	buf, err := a.readLimited(f, limit)
	if err != nil {
		return nil, key, err
	}
	return buf, key, nil
}

// readLimited 用 LimitReader 兜住「声明大小撒谎」的条目，并核对解压总量预算。
func (a *archive) readLimited(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("ebook: 解压 %q 失败: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	buf, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("ebook: 读取 %q 失败: %w", f.Name, err)
	}
	if int64(len(buf)) > limit {
		return nil, fmt.Errorf("ebook: 条目 %q 解压后超过上限 %d 字节", f.Name, limit)
	}
	if a.total+int64(len(buf)) > maxTotalBytes {
		return nil, fmt.Errorf("ebook: 解压总量超过上限 %d 字节", int64(maxTotalBytes))
	}
	a.total += int64(len(buf))
	return buf, nil
}

// firstByExt 找扩展名匹配的第一个条目（按路径排序，结果可复现）。
func (a *archive) firstByExt(ext string) string {
	for _, k := range a.sortedNames() {
		if strings.EqualFold(path.Ext(k), ext) {
			return k
		}
	}
	return ""
}

// coverByName 找文件名主干为 cover 的图片条目（封面兜底规则）。
func (a *archive) coverByName() string {
	for _, k := range a.sortedNames() {
		if !strings.EqualFold(strings.TrimSuffix(path.Base(k), path.Ext(k)), "cover") {
			continue
		}
		if isCoverExt(path.Ext(k)) {
			return k
		}
	}
	return ""
}

// sortedNames 返回全部条目路径（排序后，保证兜底选择稳定）。
func (a *archive) sortedNames() []string {
	names := make([]string, 0, len(a.files))
	for k := range a.files {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
