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

const chapterInsertSQL = "INSERT INTO chapters(book_id, volume_id, idx, title, word_count, " +
	"block_id, block_off, block_len) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"

// SaveBook 幂等写入一本书及其卷、章节。返回 BookID。
//
// 幂等语义：source_path 或 file_sha256 命中既有记录时返回 Skipped；
// opt.Force 为 true 时保留原 id 覆盖更新（先删旧卷/章节再重写）。
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

	existingID, oldAuthorID, err := lookupExistingBook(ctx, tx, w, opt)
	if err != nil {
		return SaveResult{}, err
	}
	if existingID != 0 && !opt.Force {
		return finishExistingBook(ctx, tx, existingID, b, w, opt.Update)
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

// finishExistingBook 处理「命中既有书且不覆盖」的两种结局：追更追加或幂等跳过。
//
// 追更优先于幂等跳过：Update 为真且新序列是旧序列的延伸时只追加尾部章节；
// 对不上由 appendBookInTx 返回 ErrNotAppendable（调用方整体回滚，绝不覆盖）。
// Force 走不到这里——显式覆盖优先于追更。
func finishExistingBook(ctx context.Context, tx *sql.Tx, bookID int64,
	b *model.ParsedBook, w bookWrite, update bool) (SaveResult, error) {
	if !update {
		return SaveResult{BookID: bookID, Skipped: true}, nil
	}
	if err := appendBookInTx(ctx, tx, bookID, b, w); err != nil {
		return SaveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SaveResult{}, fmt.Errorf("store: save book commit: %w", err)
	}
	return SaveResult{BookID: bookID, Updated: true}, nil
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
		language:     strings.TrimSpace(b.Language),
		publisher:    strings.TrimSpace(b.Publisher),
		isbn:         strings.TrimSpace(b.ISBN),
		pubDate:      strings.TrimSpace(b.PubDate),
		series:       strings.TrimSpace(b.Series),
		seriesIndex:  maxInt(b.SeriesIndex, 0),
		sourceFormat: normalizeSourceFormat(b.SourceFormat),
		hasCover:     boolToInt(b.Cover != nil),
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

// lookupExistingBook 找既有记录，返回 (书籍 id, 旧主作者 id)；未命中返回 (0, 0, nil)。
//
// 匹配顺序：默认用「source_path 或 file_sha256」一次查；追更模式（Update）在前两级
// 都没命中时降级到 title+author——那是「同一本书换了路径、换了内容」的最后兜底。
func lookupExistingBook(ctx context.Context, tx *sql.Tx, w bookWrite, opt SaveOptions) (int64, int64, error) {
	existingID, oldAuthorID, err := lookupByPathOrSHA(ctx, tx, w.sourcePath, w.sha)
	if err != nil || existingID != 0 || !opt.Update {
		return existingID, oldAuthorID, err
	}
	found, err := findBookIDByTitleAuthorTx(ctx, tx, w.title, w.author)
	if err != nil || found == 0 {
		return 0, 0, err
	}
	if oldAuthorID, err = bookAuthorIDInTx(ctx, tx, found); err != nil {
		return 0, 0, err
	}
	return found, oldAuthorID, nil
}

// lookupByPathOrSHA 是前两级匹配：源路径或文件哈希命中即视为同一本书。
func lookupByPathOrSHA(ctx context.Context, tx *sql.Tx, sourcePath, sha string) (int64, int64, error) {
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
	// books.author_id 是 NOT NULL 且列表/筛选都按它走，先按主作者把行写进去；
	// 多作者关联（books_authors）在章节落库后由 saveBookAuthorsInTx 统一落，
	// 它同时把 author_id 复核一遍，两处不会漂。
	authorID, err := ensureAuthor(ctx, tx, bookAuthorNames(b)[0])
	if err != nil {
		return 0, err
	}

	bookID := existingID
	if bookID != 0 {
		if err := replaceBookInTx(ctx, tx, bookID, w, authorID); err != nil {
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
	if err := saveBookExtras(ctx, tx, bookID, b); err != nil {
		return 0, err
	}
	primaryID, err := saveBookAuthorsInTx(ctx, tx, bookID, bookAuthorNames(b))
	if err != nil {
		return 0, err
	}
	if err := refreshBookAuthors(ctx, tx, primaryID, oldAuthorID); err != nil {
		return 0, err
	}
	return bookID, nil
}

// replaceBookInTx 覆盖既有书籍：先把旧口径从 metrics 里扣掉，再读旧行仍可访问的字段。
//
// 扣减顺序不能反：metrics 的旧值来自「此刻仍在库里」的块与章节，
// 一旦 replaceBookRow 删掉它们，这些数就再也算不出来了。
func replaceBookInTx(ctx context.Context, tx *sql.Tx, bookID int64, w bookWrite, authorID int64) error {
	old, err := bookTextStats(ctx, tx, bookID)
	if err != nil {
		return fmt.Errorf("store: save book old metrics: %w", err)
	}
	if err := bumpMetrics(ctx, tx, -old.packedBytes, -old.words); err != nil {
		return err
	}
	if err := bumpMetricKey(ctx, tx, metricTextBytesRaw, -old.rawBytes); err != nil {
		return err
	}
	if err := subtractBookTextBytes(ctx, tx, bookID); err != nil {
		return err
	}
	return replaceBookRow(ctx, tx, bookID, w, authorID)
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

// replaceBookRow 覆盖既有书籍：先清旧章节与压缩块，再清旧卷，最后更新主体。
//
// 块必须先于章节删除（或者说两者都必须删干净）：chapter_blocks 上有
// UNIQUE(book_id, seq) 约束，若不删旧块，重写同一本书时会直接撞约束失败。
func replaceBookRow(ctx context.Context, tx *sql.Tx, bookID int64, w bookWrite, authorID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM chapters WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear chapters: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM chapter_blocks WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear blocks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM volumes WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear volumes: %w", err)
	}
	// 旧封面必须一起清掉：has_cover 由本语句重写，book_covers 留着就成了孤儿行，
	// 前端会按 has_cover=0 走占位、而库里仍占着 2 MB。
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_covers WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear cover: %w", err)
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
		id, err := insertVolumeRow(ctx, tx, bookID, b.Volumes[vi])
		if err != nil {
			return nil, err
		}
		volMap[vi] = id
	}
	return volMap, nil
}

// insertVolumeRow 建一行卷并返回 id（全库唯一的卷写入语句，追更路径复用）。
func insertVolumeRow(ctx context.Context, tx *sql.Tx, bookID int64, v model.ParsedVolume) (int64, error) {
	res, err := tx.ExecContext(ctx, "INSERT INTO volumes(book_id, idx, title) VALUES (?, ?, ?)",
		bookID, v.Idx, strings.TrimSpace(v.Title))
	if err != nil {
		return 0, fmt.Errorf("store: save volume %d: %w", v.Idx, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: save volume id: %w", err)
	}
	return id, nil
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
