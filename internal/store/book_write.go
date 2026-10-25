package store

import (
	"context"
	"database/sql"
	"strings"

	"norvels/internal/model"
)

// 本文件是「书籍写入的形状」：books 主表的写入语句、归一化后的字段集合，
// 以及随书附加信息（标签/封面）的落库入口。
//
// 单独成文件是因为 save.go 已经承担了事务编排与章节/卷写入；把列清单与字段集合
// 放在一起，改列时只看这一处，也避免单文件越过 400 行门禁。

// bookInsertSQL / bookUpdateSQL 是书籍主体的写入语句（列顺序与 bookWrite.execArgs 一致）。
//
// 0008 起的书籍身份列（language/publisher/isbn/pub_date/series/series_index/source_format/has_cover）
// 与主字段同一条语句写入：它们都描述「这本书是什么」，拆表只会多一次往返而没有收益。
const bookInsertSQL = "INSERT INTO books(title, author_id, source_path, source_name, file_size, file_sha256, " +
	"encoding, word_count, chapter_count, volume_count, intro, status, multi_book, imported_at, updated_at, " +
	"language, publisher, isbn, pub_date, series, series_index, source_format, has_cover) " +
	"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

const bookUpdateSQL = "UPDATE books SET title = ?, author_id = ?, source_path = ?, source_name = ?, file_size = ?, " +
	"file_sha256 = ?, encoding = ?, word_count = ?, chapter_count = ?, volume_count = ?, intro = ?, " +
	"status = ?, multi_book = ?, imported_at = ?, updated_at = ?, language = ?, publisher = ?, isbn = ?, " +
	"pub_date = ?, series = ?, series_index = ?, source_format = ?, has_cover = ? WHERE id = ?"

// bookWrite 是一次写入所需的已归一化字段集合：由 prepareBookWrite 产出，
// 让「校验/归一化」与「SQL 拼接」两部分各自单一职责。
type bookWrite struct {
	sourcePath   string
	sha          string
	title        string
	sourceName   string
	author       string
	intro        string
	encoding     string
	fileSize     int64
	wordCount    int
	chapterCount int
	volumeCount  int
	multiBook    int
	nowText      string

	// ---- 书籍身份（0008 起，EPUB 才有值，txt 留空）----
	language     string
	publisher    string
	isbn         string
	pubDate      string
	series       string
	seriesIndex  int
	sourceFormat string
	hasCover     int
}

// execArgs 返回书籍写入语句的占位符参数（不含 WHERE 的 id）。
func (w bookWrite) execArgs(authorID int64) []any {
	return []any{
		w.title, authorID, w.sourcePath, w.sourceName, w.fileSize, w.sha, w.encoding,
		w.wordCount, w.chapterCount, w.volumeCount, w.intro, "parsed", w.multiBook, w.nowText, w.nowText,
		w.language, w.publisher, w.isbn, w.pubDate, w.series, w.seriesIndex, w.sourceFormat, w.hasCover,
	}
}

// saveBookExtras 写入随书附加信息：分类标签（dc:subject / 词表）与封面。
//
// 标签一律**只增不删**（P1-B §2.2）：自动标签不能覆盖用户手加的同名标签，
// 覆盖导入也不该清空旧标签；同名标签（任意 source）只保留最早那条的来源。
// 用户在 PATCH 里改 tags 才是全量替换（那是显式操作）。
// 封面与 books.has_cover 严格同源——没有封面就保持 0（旧封面已在 replaceBookRow 删掉）。
func saveBookExtras(ctx context.Context, tx *sql.Tx, bookID int64, b *model.ParsedBook) error {
	if len(b.Subjects) > 0 {
		if err := addBookTagsWithSourceInTx(ctx, tx, bookID, b.Subjects, TagSourceEPUB); err != nil {
			return err
		}
	}
	if len(b.AutoTags) > 0 {
		if err := addBookTagsWithSourceInTx(ctx, tx, bookID, b.AutoTags, TagSourceAuto); err != nil {
			return err
		}
	}
	if b.Cover == nil {
		return nil
	}
	return saveCoverInTx(ctx, tx, bookID, b.Cover.MIME, b.Cover.Bytes)
}

// normalizeSourceFormat 归一化来源格式：空值按 txt 处理，未知值也回落到 txt
// （列上有 NOT NULL DEFAULT 'txt'，写进未知值只会让前端的分支落空）。
func normalizeSourceFormat(s string) string {
	switch f := strings.ToLower(strings.TrimSpace(s)); f {
	case "txt", "epub":
		return f
	default:
		return "txt"
	}
}

// maxInt 返回两个整数中的较大者（把 series_index 的负值夹到 0，0 表示未知）。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
