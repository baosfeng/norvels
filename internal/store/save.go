package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"norvels/internal/model"
)

// bookInsertSQL / bookUpdateSQL 是书籍主体的写入语句（列顺序与 execBookWrite 的参数顺序一致）。
const bookInsertSQL = "INSERT INTO books(title, author_id, source_path, source_name, file_size, file_sha256, " +
	"encoding, word_count, chapter_count, volume_count, intro, status, multi_book, imported_at, updated_at) " +
	"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

const bookUpdateSQL = "UPDATE books SET title = ?, author_id = ?, source_path = ?, source_name = ?, file_size = ?, " +
	"file_sha256 = ?, encoding = ?, word_count = ?, chapter_count = ?, volume_count = ?, intro = ?, " +
	"status = ?, multi_book = ?, imported_at = ?, updated_at = ? WHERE id = ?"

const chapterInsertSQL = "INSERT INTO chapters(book_id, volume_id, idx, title, title_seg, content, content_seg, word_count) " +
	"VALUES (?, ?, ?, ?, ?, ?, ?, ?)"

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
}

// execArgs 返回书籍写入语句的占位符参数（不含 WHERE 的 id）。
func (w bookWrite) execArgs(authorID int64) []any {
	return []any{
		w.title, authorID, w.sourcePath, w.sourceName, w.fileSize, w.sha, w.encoding,
		w.wordCount, w.chapterCount, w.volumeCount, w.intro, "parsed", w.multiBook, w.nowText, w.nowText,
	}
}

// SaveBook 幂等写入一本书及其卷、章节，并同步维护 FTS 索引。返回 BookID。
//
// 幂等语义：source_path 或 file_sha256 命中既有记录时返回 Skipped；
// opt.Force 为 true 时保留原 id 覆盖更新（先删旧卷/章节，触发器同步清理 FTS）。
func (s *Store) SaveBook(ctx context.Context, b *model.ParsedBook, opt SaveOptions) (SaveResult, error) {
	w, usedVols, err := prepareBookWrite(b)
	if err != nil {
		return SaveResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SaveResult{}, fmt.Errorf("store: save book begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	existingID, oldAuthorID, err := lookupExistingBook(ctx, tx, w.sourcePath, w.sha)
	if err != nil {
		return SaveResult{}, err
	}
	if existingID != 0 && !opt.Force {
		return SaveResult{BookID: existingID, Skipped: true}, nil
	}

	bookID, err := saveBookInTx(ctx, tx, b, w, usedVols, existingID, oldAuthorID)
	if err != nil {
		return SaveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SaveResult{}, fmt.Errorf("store: save book commit: %w", err)
	}
	return SaveResult{BookID: bookID, Updated: existingID != 0}, nil
}

// prepareBookWrite 校验入参并归一化展示字段，同时返回按序号排好的卷引用列表。
func prepareBookWrite(b *model.ParsedBook) (bookWrite, []int, error) {
	if b == nil {
		return bookWrite{}, nil, fmt.Errorf("%w: nil book", ErrInvalidArgument)
	}
	sourcePath := strings.TrimSpace(b.SourcePath)
	if sourcePath == "" {
		return bookWrite{}, nil, fmt.Errorf("%w: empty source path", ErrInvalidArgument)
	}
	sha := strings.ToLower(strings.TrimSpace(b.SHA256))
	if sha == "" {
		return bookWrite{}, nil, fmt.Errorf("%w: empty sha256", ErrInvalidArgument)
	}

	usedVols := b.VolumeIdxSet()
	sort.Ints(usedVols)

	w := bookWrite{
		sourcePath:   sourcePath,
		sha:          sha,
		title:        bookTitle(b),
		sourceName:   bookSourceName(b, sourcePath),
		author:       bookAuthor(b),
		intro:        truncateRunes(strings.TrimSpace(b.Intro), 500),
		encoding:     b.Encoding,
		fileSize:     b.FileSize,
		wordCount:    bookWordCount(b),
		chapterCount: len(b.Chapters),
		volumeCount:  len(usedVols),
		multiBook:    boolToInt(b.MultiBook),
		nowText:      formatTime(nowUTC()),
	}
	return w, usedVols, nil
}

// bookTitle 取展示标题：书名 → 源文件名 → 「未命名」。
func bookTitle(b *model.ParsedBook) string {
	if t := strings.TrimSpace(b.Title); t != "" {
		return t
	}
	if t := strings.TrimSpace(b.SourceName); t != "" {
		return t
	}
	return "未命名"
}

// bookSourceName 取源文件名：优先解析结果，缺失时回落到路径 basename。
func bookSourceName(b *model.ParsedBook, sourcePath string) string {
	if n := strings.TrimSpace(b.SourceName); n != "" {
		return n
	}
	return filepath.Base(sourcePath)
}

// bookAuthor 取作者名，缺失时落「佚名」，避免 author_id=0 导致按作者筛选丢书。
func bookAuthor(b *model.ParsedBook) string {
	if a := strings.TrimSpace(b.Author); a != "" {
		return a
	}
	return unknownAuthor
}

// bookWordCount 取整书字数：解析结果缺失（<=0）时按章节正文重算。
func bookWordCount(b *model.ParsedBook) int {
	if b.WordCount > 0 {
		return b.WordCount
	}
	total := 0
	for i := range b.Chapters {
		total += chapterWordCount(&b.Chapters[i])
	}
	return total
}

// chapterWordCount 取单章字数：缺失（<=0）时按正文重算。
func chapterWordCount(c *model.ParsedChapter) int {
	if c.WordCount > 0 {
		return c.WordCount
	}
	return countWords(c.Content)
}

// lookupExistingBook 按 source_path 或 file_sha256 找既有记录；未命中返回 (0, 0, nil)。
func lookupExistingBook(ctx context.Context, tx *sql.Tx, sourcePath, sha string) (int64, int64, error) {
	var existingID, oldAuthorID int64
	err := tx.QueryRowContext(ctx,
		"SELECT id, author_id FROM books WHERE source_path = ? OR file_sha256 = ? ORDER BY id LIMIT 1",
		sourcePath, sha).Scan(&existingID, &oldAuthorID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, 0, nil
	case err != nil:
		return 0, 0, fmt.Errorf("store: save book lookup: %w", err)
	}
	return existingID, oldAuthorID, nil
}

// saveBookInTx 在事务内完成书籍主体、卷、章节写入与作者聚合刷新，返回书籍 id。
// existingID 为 0 表示新建，否则为覆盖更新（保留原 id）。
func saveBookInTx(ctx context.Context, tx *sql.Tx, b *model.ParsedBook, w bookWrite, usedVols []int, existingID, oldAuthorID int64) (int64, error) {
	authorID, err := ensureAuthor(ctx, tx, w.author)
	if err != nil {
		return 0, err
	}

	bookID := existingID
	if bookID != 0 {
		// 覆盖写入会先删旧章节，旧值必须先从 metrics 里减掉，否则会重复计数。
		oldText, oldWords, err := bookTextStats(ctx, tx, bookID)
		if err != nil {
			return 0, fmt.Errorf("store: save book old metrics: %w", err)
		}
		if err := bumpMetrics(ctx, tx, -oldText, -oldWords); err != nil {
			return 0, err
		}
		if err := subtractBookTextBytes(ctx, tx, bookID); err != nil {
			return 0, err
		}
		if err := replaceBookRow(ctx, tx, bookID, w, authorID); err != nil {
			return 0, err
		}
	} else if bookID, err = insertBookRow(ctx, tx, w, authorID); err != nil {
		return 0, err
	}

	volMap, err := insertVolumes(ctx, tx, bookID, b, usedVols)
	if err != nil {
		return 0, err
	}
	if err := insertChapters(ctx, tx, bookID, b, volMap); err != nil {
		return 0, err
	}
	if err := refreshBookAuthors(ctx, tx, authorID, oldAuthorID); err != nil {
		return 0, err
	}
	return bookID, nil
}

// insertBookRow 插入书籍主体并返回新 id。
func insertBookRow(ctx context.Context, tx *sql.Tx, w bookWrite, authorID int64) (int64, error) {
	res, err := tx.ExecContext(ctx, bookInsertSQL, w.execArgs(authorID)...)
	if err != nil {
		return 0, fmt.Errorf("store: save book insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: save book id: %w", err)
	}
	return id, nil
}

// replaceBookRow 覆盖既有书籍：先清旧章节（触发器同步删 FTS 行）再清旧卷，最后更新主体。
func replaceBookRow(ctx context.Context, tx *sql.Tx, bookID int64, w bookWrite, authorID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM chapters WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear chapters: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM volumes WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear volumes: %w", err)
	}
	args := append(w.execArgs(authorID), bookID)
	if _, err := tx.ExecContext(ctx, bookUpdateSQL, args...); err != nil {
		return fmt.Errorf("store: save book update: %w", err)
	}
	return nil
}

// insertVolumes 写入被章节引用到的卷，返回「解析侧卷序号 → 库内卷 id」的映射。
func insertVolumes(ctx context.Context, tx *sql.Tx, bookID int64, b *model.ParsedBook, usedVols []int) (map[int]int64, error) {
	volMap := make(map[int]int64, len(usedVols))
	for _, vi := range usedVols {
		if vi < 0 || vi >= len(b.Volumes) {
			continue
		}
		v := b.Volumes[vi]
		res, err := tx.ExecContext(ctx, "INSERT INTO volumes(book_id, idx, title) VALUES (?, ?, ?)",
			bookID, v.Idx, strings.TrimSpace(v.Title))
		if err != nil {
			return nil, fmt.Errorf("store: save volume %d: %w", v.Idx, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("store: save volume id: %w", err)
		}
		volMap[vi] = id
	}
	return volMap, nil
}

// insertChapters 用一条预处理语句批量写入章节（含 FTS 镜像列 title_seg/content_seg），
// 并顺手累加 store_metrics 需要的聚合量。
//
// 导入路径本来就要遍历所有章节，在这里累加几乎零额外成本——比事后跑
// SUM(length(content)) 全表扫（42 万行实测 54 s）便宜三个数量级。
// 累加与章节写入在同一事务内完成，二者不会漂移。
//
// 口径必须与 SQL 的 length() 一致：SQLite 的 length(TEXT) 返回**字符数**，
// 因此这里用 utf8.RuneCountInString 而不是 len()（后者是字节数）。
func insertChapters(ctx context.Context, tx *sql.Tx, bookID int64, b *model.ParsedBook, volMap map[int]int64) error {
	stmt, err := tx.PrepareContext(ctx, chapterInsertSQL)
	if err != nil {
		return fmt.Errorf("store: save chapter prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	var textRunes, words int64
	for i := range b.Chapters {
		c := &b.Chapters[i]
		titleSeg := Segment(c.Title)
		contentSeg := Segment(c.Content)
		wc := chapterWordCount(c)
		if _, err := stmt.ExecContext(ctx, bookID, volumeIDFor(c, volMap), c.Idx, c.Title,
			titleSeg, c.Content, contentSeg, wc); err != nil {
			return fmt.Errorf("store: save chapter idx=%d: %w", c.Idx, err)
		}
		textRunes += runeLen(c.Title) + runeLen(c.Content) + runeLen(titleSeg) + runeLen(contentSeg)
		words += int64(wc)
	}
	if err := bumpMetrics(ctx, tx, textRunes, words); err != nil {
		return err
	}
	// books 侧文本量（标题/简介/路径等）随书籍主体变化，一并增量维护，
	// 保证「增量累加」与「全量重算」在同一个口径上（有测试守着）。
	bookText, err := bumpBookTextBytes(ctx, tx, bookID)
	if err != nil {
		return err
	}
	_ = bookText
	return nil
}

// bumpBookTextBytes 重算单本书的文本字节并把差值并入 book_text_bytes。
//
// books 表只有千级行，按 book_id 取一行是主键查询，代价可忽略；
// 用「重算差值」而不是「猜增量」，因此覆盖写入/标题变更都不会漏。
func bumpBookTextBytes(ctx context.Context, tx *sql.Tx, bookID int64) (int64, error) {
	var current int64
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(length(title) + length(intro) + length(source_path) + length(source_name) + "+
			"length(encoding) + length(status), 0) FROM books WHERE id = ?", bookID).Scan(&current); err != nil {
		return 0, fmt.Errorf("store: book text bytes: %w", err)
	}
	if err := bumpMetricKey(ctx, tx, metricBookTextBytes, current); err != nil {
		return 0, err
	}
	return current, nil
}

// subtractBookTextBytes 在删除/覆盖前把这本书的文本字节从 book_text_bytes 里减掉。
func subtractBookTextBytes(ctx context.Context, tx *sql.Tx, bookID int64) error {
	var current int64
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(length(title) + length(intro) + length(source_path) + length(source_name) + "+
			"length(encoding) + length(status), 0) FROM books WHERE id = ?", bookID).Scan(&current); err != nil {
		return fmt.Errorf("store: book text bytes: %w", err)
	}
	return bumpMetricKey(ctx, tx, metricBookTextBytes, -current)
}

// runeLen 返回字符串的字符数，与 SQLite 的 length(TEXT) 口径一致。
func runeLen(s string) int64 { return int64(utf8.RuneCountInString(s)) }

// volumeIDFor 返回章节所属卷的库内 id；未归卷（VolumeIdx < 0 或卷未落库）返回 0。
func volumeIDFor(c *model.ParsedChapter, volMap map[int]int64) int64 {
	if c.VolumeIdx < 0 {
		return 0
	}
	return volMap[c.VolumeIdx]
}

// refreshBookAuthors 刷新新旧作者的聚合列（作者变更时两边都要重算）。
func refreshBookAuthors(ctx context.Context, tx *sql.Tx, authorID, oldAuthorID int64) error {
	if err := refreshAuthorStats(ctx, tx, authorID); err != nil {
		return err
	}
	if oldAuthorID != 0 && oldAuthorID != authorID {
		return refreshAuthorStats(ctx, tx, oldAuthorID)
	}
	return nil
}

// ensureAuthor 按名字取作者 id，不存在则创建；空名返回 0（未知作者）。
func ensureAuthor(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	if name == "" {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO authors(name) VALUES (?) ON CONFLICT(name) DO NOTHING", name); err != nil {
		return 0, fmt.Errorf("store: ensure author: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM authors WHERE name = ?", name).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: select author: %w", err)
	}
	return id, nil
}

// refreshAuthorStats 重算单个作者的聚合列（books 表只有千级行，成本可忽略）。
func refreshAuthorStats(ctx context.Context, tx *sql.Tx, authorID int64) error {
	if authorID == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		"UPDATE authors SET "+
			"book_count = (SELECT COUNT(*) FROM books WHERE author_id = authors.id), "+
			"word_count = (SELECT COALESCE(SUM(word_count), 0) FROM books WHERE author_id = authors.id) "+
			"WHERE id = ?", authorID)
	if err != nil {
		return fmt.Errorf("store: refresh author stats: %w", err)
	}
	return nil
}
