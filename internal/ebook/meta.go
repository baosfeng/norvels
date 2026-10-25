package ebook

import (
	"path/filepath"
	"strconv"
	"strings"
)

// newBook 用元数据建 Book；OPF 缺书名/作者时回落文件名规则。
func newBook(name string, doc *opfDoc) *Book {
	title, author := fallbackName(name)
	b := &Book{Title: title, Author: author, spineCount: len(doc.spine)}
	applyMetadata(b, doc.meta)
	return b
}

// applyMetadata 把 OPF 元数据搬到 Book 上；每个字段只覆盖非空值，保留文件名兜底。
func applyMetadata(b *Book, m opfMetadata) {
	if v := firstNonEmpty(m.Titles); v != "" {
		b.Title = v
	}
	applyCreator(b, m.Creators)
	b.Intro = cleanIntro(firstNonEmpty(m.Descriptions))
	b.Language = firstNonEmpty(m.Languages)
	b.Publisher = firstNonEmpty(m.Publishers)
	b.PubDate = firstNonEmpty(m.Dates)
	b.ISBN = pickISBN(m.Identifiers)
	b.Subjects = cleanList(m.Subjects)
	b.Series, b.SeriesIndex = pickSeries(m)
}

// applyCreator 只取第一个 creator：库里 author 是单值聚合列，把多个作者拼成
// 一个字符串会污染 /api/authors 的作者列表，宁可变少也不变脏。
func applyCreator(b *Book, creators []string) {
	names := cleanList(creators)
	if len(names) == 0 {
		return
	}
	b.Author = names[0]
	if len(names) > 1 {
		b.warnf("EPUB 声明了 %d 个作者，只保留第一个 %q", len(names), names[0])
	}
}

// pickSeries 取系列名与序号：Calibre 的 calibre:series 优先，缺失的字段再由
// EPUB3 的 belongs-to-collection / group-position 补齐。
func pickSeries(m opfMetadata) (string, int) {
	series, index := calibreSeries(m.Metas)
	if series == "" || index == 0 {
		name, pos := epub3Series(m.Metas)
		if series == "" {
			series = name
		}
		if index == 0 {
			index = pos
		}
	}
	return series, index
}

// calibreSeries 读 Calibre 的 <meta name="calibre:series"> 写法。
func calibreSeries(metas []opfMeta) (string, int) {
	series, index := "", 0
	for _, meta := range metas {
		switch strings.ToLower(strings.TrimSpace(meta.Name)) {
		case "calibre:series":
			if series == "" {
				series = strings.TrimSpace(meta.Content)
			}
		case "calibre:series_index":
			if index == 0 {
				index = parseSeriesIndex(meta.Content)
			}
		}
	}
	return series, index
}

// epub3Series 读 EPUB3 的 belongs-to-collection / group-position 写法。
func epub3Series(metas []opfMeta) (string, int) {
	series, index := "", 0
	for _, meta := range metas {
		switch strings.ToLower(strings.TrimSpace(meta.Property)) {
		case "belongs-to-collection":
			if series == "" {
				series = strings.TrimSpace(meta.Value)
			}
		case "group-position":
			if index == 0 {
				index = parseSeriesIndex(meta.Content)
			}
		}
	}
	return series, index
}

// parseSeriesIndex 解析系列序号（Calibre 允许 1.5 这类小数，取整）。
func parseSeriesIndex(s string) int {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 || f > 100000 {
		return 0
	}
	return int(f)
}

// pickISBN 从 dc:identifier 里挑书号：先认显式的 ISBN 标识，再认像 ISBN 的 10/13 位数字串。
func pickISBN(ids []opfIdentifier) string {
	for _, id := range ids {
		v := strings.TrimSpace(id.Value)
		if strings.EqualFold(strings.TrimSpace(id.Scheme), "isbn") || strings.HasPrefix(strings.ToLower(v), "urn:isbn:") {
			if isbn := isbnDigits(v); isbn != "" {
				return isbn
			}
		}
	}
	for _, id := range ids {
		if isbn := isbnDigits(id.Value); len(isbn) == 10 || len(isbn) == 13 {
			return isbn
		}
	}
	return ""
}

// isbnDigits 只保留书号本体（数字与校验位 X）。
func isbnDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'x' || r == 'X':
			b.WriteByte('X')
		}
	}
	return b.String()
}

// cleanIntro 把简介里的 HTML 转成纯文本（dc:description 常带 <p>/<br>）。
func cleanIntro(s string) string {
	s = strings.TrimSpace(s)
	if !strings.ContainsAny(s, "<>") {
		return s
	}
	return strings.TrimSpace(textFromHTML([]byte("<div>" + s + "</div>")))
}

// fallbackName 用文件名兜底书名/作者：去扩展名后按常见分隔符切「书名 - 作者」。
func fallbackName(name string) (string, string) {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if base == "" || base == "." {
		return "", ""
	}
	for _, sep := range []string{" - ", " — ", "_"} {
		if i := strings.Index(base, sep); i > 0 {
			title := strings.TrimSpace(base[:i])
			author := strings.TrimSpace(base[i+len(sep):])
			if title != "" && author != "" {
				return title, author
			}
		}
	}
	return base, ""
}

// firstNonEmpty 返回第一个去空白后非空的字符串。
func firstNonEmpty(in []string) string {
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// cleanList 去空白、去重（保持顺序），全空时返回 nil。
func cleanList(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
