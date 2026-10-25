package importer

import (
	"strings"

	"norvels/internal/ebook"
	"norvels/internal/model"
	"norvels/internal/parser"
)

// epubToParsedBook 把 ebook.Book 转成 store 认识的 ParsedBook：元数据与封面原样搬运，
// 章节按 spine 顺序编号；「整本一页」的大书改走 txt 切章策略（spec §4）。
func epubToParsedBook(p *parser.Parser, eb *ebook.Book) *model.ParsedBook {
	return epubToParsedBookWithRules(p, eb, nil)
}

// epubToParsedBookWithRules 是 epubToParsedBook 的带规则版本。
func epubToParsedBookWithRules(p *parser.Parser, eb *ebook.Book, rules []parser.Rule) *model.ParsedBook {
	book := &model.ParsedBook{
		Title:        eb.Title,
		Author:       eb.Author,
		Intro:        eb.Intro,
		Language:     eb.Language,
		Publisher:    eb.Publisher,
		ISBN:         eb.ISBN,
		PubDate:      eb.PubDate,
		Series:       eb.Series,
		SeriesIndex:  eb.SeriesIndex,
		Subjects:     eb.Subjects,
		SourceFormat: FormatEPUB,
		Encoding:     "utf-8",
		Warnings:     eb.Warnings,
	}
	if eb.Cover != nil {
		book.Cover = &model.CoverImage{MIME: eb.Cover.MIME, Bytes: eb.Cover.Bytes}
	}
	if eb.NeedsSplit() {
		// 「整本一页」的 epub 交给 txt 切章，目录层级随之失效，按平铺口径记录。
		splitByParserWithRules(p, book, eb, rules)
		if len(eb.Volumes) > 0 {
			book.Warnings = append(book.Warnings, "EPUB 目录层级在全文重切后失效，已按平铺导入")
		}
	} else {
		book.Chapters = epubChapters(eb.Chapters)
		book.Volumes = epubVolumes(eb.Volumes)
	}
	book.WordCount = totalWords(book.Chapters)
	return book
}

// splitByParser 把「spine 只有一两个 XHTML」的全文交给 txt 解析器重新切章，
// 复用 di-unit 等既有策略；切章失败时退回 spine 原章节，不让一本书因此导入失败。
func splitByParser(p *parser.Parser, book *model.ParsedBook, eb *ebook.Book) {
	splitByParserWithRules(p, book, eb, nil)
}

// splitByParserWithRules 是 splitByParser 的带规则版本。
func splitByParserWithRules(p *parser.Parser, book *model.ParsedBook, eb *ebook.Book, rules []parser.Rule) {
	parsed, err := p.ParseBytesWithRules(book.Title+".txt", []byte(eb.FullText()), rules)
	if err != nil || parsed == nil || len(parsed.Chapters) == 0 {
		book.Chapters = epubChapters(eb.Chapters)
		book.Warnings = append(book.Warnings, "全文切章失败，已按 spine 原章节导入")
		return
	}
	book.Chapters = parsed.Chapters
	book.Warnings = append(book.Warnings, keepSplitWarnings(parsed.Warnings)...)
	book.Warnings = append(book.Warnings, "spine 只有一个正文文档，已按 txt 切章策略重新分章")
}

// keepSplitWarnings 过滤掉 parser 对书名/作者推断的告警：切章用的是一个临时文件名，
// 那两条只反映临时文件名的推断失败，EPUB 元数据才是权威来源。
func keepSplitWarnings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, w := range in {
		if w == "author_missing" || strings.HasPrefix(w, "未能推断书名") {
			continue
		}
		out = append(out, w)
	}
	return out
}

// epubChapters 把 spine 章节转成模型章节。
//
// ebook 的 VolumeIdx 是 1 起（0=未分卷），model 是 0 起（-1=未分卷），这里做唯一一次转换。
func epubChapters(chapters []ebook.Chapter) []model.ParsedChapter {
	out := make([]model.ParsedChapter, 0, len(chapters))
	for i, c := range chapters {
		out = append(out, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: modelVolumeIdx(c.VolumeIdx),
			Title:     c.Title,
			Content:   c.Content,
			WordCount: countTextWords(c.Content),
		})
	}
	return out
}

// modelVolumeIdx 把 ebook 的 1 起卷下标转成 model 的 0 起（0 → -1 未分卷）。
func modelVolumeIdx(ebookVolumeIdx int) int {
	if ebookVolumeIdx <= 0 {
		return -1
	}
	return ebookVolumeIdx - 1
}

// epubVolumes 把目录层级转成卷分组（EPUB3 nav.xhtml / EPUB2 toc.ncx 解析而来）。
func epubVolumes(vols []ebook.Volume) []model.ParsedVolume {
	if len(vols) == 0 {
		return nil
	}
	out := make([]model.ParsedVolume, 0, len(vols))
	for i, v := range vols {
		out = append(out, model.ParsedVolume{Idx: i, Title: v.Title})
	}
	return out
}
