// Package ebook 解析 EPUB：解包、读 OPF 元数据、按 spine 抽章节、取封面。
//
// 解析契约见 docs/specs/2026-10-25-import-formats.md §4。产物是 package 自有类型，
// 不依赖 model——由 importer 负责转换成 model.ParsedBook，解析层不被存储字段牵着走。
//
// 只依赖标准库与 golang.org/x/net/html（纯 Go，CGO_ENABLED=0 下可用）。
package ebook

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ErrEncrypted 表示 EPUB 带加密（META-INF/encryption.xml 非空），按 spec 直接拒绝。
var ErrEncrypted = errors.New("ebook: encrypted")

// Book 是 EPUB 的解析产物，字段与 spec §4 一一对应。
type Book struct {
	Title       string
	Author      string
	Intro       string
	Language    string
	Publisher   string
	ISBN        string
	PubDate     string
	Series      string
	SeriesIndex int
	Subjects    []string
	Cover       *Cover
	Chapters    []Chapter
	Warnings    []string

	// spineCount 是 OPF spine 的条目数：用它判断「整本书只有一两个 XHTML」，
	// 这类书需要整篇交给 txt 解析器切章（见 NeedsSplit）。
	spineCount int
}

// Cover 是随书入库的封面（全库唯一允许入库的二进制）。
type Cover struct {
	MIME  string
	Bytes []byte
}

// Chapter 是 spine 中的一个 XHTML 文档。
type Chapter struct {
	Title   string
	Content string
}

// longTextChars 是「整本 epub 就是一大段文本」的字数阈值（spec §4）。
const longTextChars = 20000

// Open 解析 name（原文件名，用于书名兜底）对应的 EPUB 字节。
func Open(name string, data []byte) (*Book, error) {
	a, err := openArchive(data)
	if err != nil {
		return nil, err
	}
	if err := checkEncrypted(a, name); err != nil {
		return nil, err
	}
	doc, err := loadPackage(a)
	if err != nil {
		return nil, err
	}
	book := newBook(name, doc)
	if err := book.loadChapters(a, doc); err != nil {
		return nil, err
	}
	book.Cover = pickCover(a, doc, book)
	book.Warnings = dedupeWarnings(book.Warnings)
	return book, nil
}

// FullText 按章拼接正文（段间空行），供 importer 交给 txt 解析器重新切章。
func (b *Book) FullText() string {
	var sb strings.Builder
	for _, c := range b.Chapters {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(c.Content)
	}
	return sb.String()
}

// NeedsSplit 判断是否需要把全文交给 txt 解析器切章：spine 只有 1~2 个文档，
// 但总字数已超过长篇阈值——典型是「整本书一个 XHTML」的排版。
func (b *Book) NeedsSplit() bool {
	return b.spineCount <= 2 && utf8.RuneCountInString(b.FullText()) > longTextChars
}

// warnf 追加一条告警。
func (b *Book) warnf(format string, args ...any) {
	b.Warnings = append(b.Warnings, fmt.Sprintf(format, args...))
}

// checkEncrypted 判断是否加密：encryption.xml 存在且非空即拒绝（不区分 DRM 与字体混淆）。
func checkEncrypted(a *archive, name string) error {
	if !a.has(encryptionPath) {
		return nil
	}
	data, _, err := a.read(encryptionPath, maxEntryBytes)
	if err != nil {
		return fmt.Errorf("ebook: 读取 %s 失败: %w", encryptionPath, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s 含 %s", ErrEncrypted, filepath.Base(name), encryptionPath)
}

// dedupeWarnings 去重告警（同时丢掉空串），保持首次出现的顺序。
func dedupeWarnings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, w := range in {
		w = strings.TrimSpace(w)
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
