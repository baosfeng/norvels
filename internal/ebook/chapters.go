package ebook

import (
	"fmt"
	"path"
	"strings"
)

// chapterMediaTypes 是 spine 里当作正文的媒体类型；图片/音频等一律跳过。
var chapterMediaTypes = map[string]bool{
	"application/xhtml+xml": true,
	"text/html":             true,
}

// isHTMLHref 判断 href 是否是 HTML/XHTML 文档。
func isHTMLHref(href string) bool {
	switch strings.ToLower(path.Ext(href)) {
	case ".xhtml", ".html", ".htm":
		return true
	}
	return false
}

// loadChapters 按 spine 顺序抽取章节正文；空文档（封面页、纯图片页）直接跳过。
//
// 章节的顺序与取舍一律以 spine 为准；TOC（nav.xhtml 优先、toc.ncx 兜底）只提供
// 标题与卷归属，不增删章节——见 docs/specs/2026-10-25-import-p1.md §1。
func (b *Book) loadChapters(a *archive, doc *opfDoc) error {
	toc := loadTOC(a, doc, b)
	hrefs := make([]string, 0, len(doc.spine))
	for _, ref := range doc.spine {
		if strings.EqualFold(strings.TrimSpace(ref.Linear), "no") {
			continue // linear="no" 是目录/注释这类附页，不当作章节
		}
		item, ok := doc.items[ref.IDRef]
		if !ok {
			b.warnf("spine 引用了不存在的清单项 %q，已跳过", ref.IDRef)
			continue
		}
		href := resolveHref(doc.dir, item.Href)
		if toc != nil && toc.docs[href] {
			b.warnf("spine 把目录页 %q 当成正文，已跳过", href)
			continue
		}
		title, content, err := b.readChapter(a, doc, item)
		if err != nil {
			b.Warnings = append(b.Warnings, err.Error())
			continue
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		b.Chapters = append(b.Chapters, Chapter{Title: title, Content: content})
		hrefs = append(hrefs, href)
	}
	if len(b.Chapters) == 0 {
		return fmt.Errorf("ebook: 没有可读章节（spine %d 项）", len(doc.spine))
	}
	b.applyTOC(toc, hrefs)
	return nil
}

// readChapter 读取单个 spine 文档并转成「标题 + 纯文本段落」。
func (b *Book) readChapter(a *archive, doc *opfDoc, item opfItem) (string, string, error) {
	if !chapterMediaTypes[strings.ToLower(strings.TrimSpace(item.MediaType))] && !isHTMLHref(item.Href) {
		return "", "", fmt.Errorf("ebook: spine 项 %q 不是 XHTML（%s），已跳过", item.Href, item.MediaType)
	}
	href := resolveHref(doc.dir, item.Href)
	data, _, err := a.read(href, maxEntryBytes)
	if err != nil {
		return "", "", fmt.Errorf("ebook: 读取章节 %q 失败: %w", href, err)
	}
	title, text, fromHeading := htmlChapter(data, href)
	if !fromHeading && title == b.Title {
		// 有些 epub 每章的 <title> 都写成书名，退回文件名更接近真实章节名。
		if fallback := chapterTitleFromHref(href); fallback != "" {
			title = fallback
		}
	}
	return title, text, nil
}
