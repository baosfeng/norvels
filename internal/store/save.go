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
//
// 跨库写入：正文块先进正文库、books/volumes/chapters 后进主库，提交顺序由
// writeThrough 保证（见 content.go）；主库提交失败时按块 id 回滚这次新写的块。
func (s *Store) SaveBook(ctx context.Context, b *model.ParsedBook, opt SaveOptions) (SaveResult, error) {
	w, usedVols, err := prepareBookWrite(b)
	if err != nil {
		return SaveResult{}, err
	}
	var res SaveResult
	err = s.writeThrough(ctx, "save book", func(wt *writeTx) error {
		existingID, oldAuthorID, err := lookupExistingBook(ctx, wt.main, w, opt)
		if err != nil {
			return err
		}
		if existingID != 0 && !opt.Force {
			res, err = finishExistingBook(ctx, wt, existingID, b, w, opt.Update)
			return err
		}
		bookID, err := saveBookInTx(ctx, wt, b, w, usedVols, existingID, oldAuthorID)
		if err != nil {
			return err
		}
		res = SaveResult{BookID: bookID, Updated: existingID != 0}
		return nil
	})
	if err != nil {
		return SaveResult{}, err
	}
	s.writeBookFingerprintsAfterCommit(ctx, res, b.Chapters)
	return res, nil
}

// finishExistingBook 处理「命中既有书且不覆盖」的两种结局：追更追加或幂等跳过。
//
// 追更优先于幂等跳过：Update 为真且新序列是旧序列的延伸时只追加尾部章节；
// 对不上由 appendBookInTx 返回 ErrNotAppendable（调用方整体回滚，绝不覆盖）。
// Force 走不到这里——显式覆盖优先于追更。提交由 writeThrough 统一完成。
func finishExistingBook(ctx context.Context, wt *writeTx, bookID int64,
	b *model.ParsedBook, w bookWrite, update bool) (SaveResult, error) {
	if !update {
		return SaveResult{BookID: bookID, Skipped: true}, nil
	}
	appended, err := appendBookInTx(ctx, wt, bookID, b, w)
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{BookID: bookID, Updated: true, Appended: appended}, nil
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
		categoryL1:   truncateRunes(strings.TrimSpace(b.CategoryL1), maxCategoryLen),
		categoryL2:   truncateRunes(strings.TrimSpace(b.CategoryL2), maxCategoryLen),
		quality:      qualityFromMetrics(b.Quality, nowUTC()),
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

// saveBookInTx 完成书籍主体、卷、章节写入与作者聚合刷新，返回书籍 id。
// existingID 为 0 表示新建，否则为覆盖更新（保留原 id）。
//
// 新书的 id 必须先由主库分配（AUTOINCREMENT），正文块才能带着 book_id 进正文库：
// 因此这里先写 books 行拿 id，再写块、最后写章节行。事务提交顺序仍是
// 「正文库在前、主库在后」，由 writeThrough 收口。
func saveBookInTx(ctx context.Context, wt *writeTx, b *model.ParsedBook, w bookWrite, usedVols []int, existingID, oldAuthorID int64) (int64, error) {
	tx := wt.main
	// books.author_id 是 NOT NULL 且列表/筛选都按它走，先按主作者把行写进去；
	// 多作者关联（books_authors）在章节落库后由 saveBookAuthorsInTx 统一落，
	// 它同时把 author_id 复核一遍，两处不会漂。
	authorID, err := ensureAuthor(ctx, tx, bookAuthorNames(b)[0])
	if err != nil {
		return 0, err
	}

	bookID := existingID
	if bookID != 0 {
		if err := replaceBookInTx(ctx, wt, bookID, w, authorID); err != nil {
			return 0, err
		}
	} else {
		if bookID, err = insertBookRow(ctx, tx, w, authorID); err != nil {
			return 0, err
		}
		// 新书进库：导入趋势按「这本书的入库日」+1 本书、+chapter_count 章。
		// 覆盖写入不重复计书数——它走 replaceBookInTx，那边只减旧值，
		// 随后 insertChapters 会把新章重新加回来（含章数与直方图）。
		if err := bumpImportDaily(ctx, tx, w.nowText, 1, w.chapterCount); err != nil {
			return 0, err
		}
	}

	volMap, err := insertVolumes(ctx, tx, bookID, b, usedVols)
	if err != nil {
		return 0, err
	}
	if err := insertChapters(ctx, wt, bookID, b, volMap); err != nil {
		return 0, err
	}
	if err := saveBookExtras(ctx, tx, bookID, b); err != nil {
		return 0, err
	}
	if _, err := saveBookAuthorsInTx(ctx, tx, bookID, bookAuthorNames(b)); err != nil {
		return 0, err
	}
	// oldAuthorID 要显式带上：这本书可能刚从它名下换走，此刻已经查不到这条关联了。
	if err := refreshBookAuthors(ctx, tx, bookID, oldAuthorID); err != nil {
		return 0, err
	}
	return bookID, nil
}

// replaceBookInTx 覆盖既有书籍：先把旧口径从 metrics 里扣掉，再读旧行仍可访问的字段。
//
// 扣减顺序不能反：metrics 的旧值来自「此刻仍在库里」的块与章节，
// 一旦 replaceBookRow 删掉它们，这些数就再也算不出来了。
func replaceBookInTx(ctx context.Context, wt *writeTx, bookID int64, w bookWrite, authorID int64) error {
	tx := wt.main
	old, err := bookTextStats(ctx, wt, bookID)
	if err != nil {
		return fmt.Errorf("store: save book old metrics: %w", err)
	}
	// 旧章节即将整批删掉：章数与长度直方图都要按「旧章逐个减」冲减。
	oldWords, err := bookChapterWordCounts(ctx, tx, bookID)
	if err != nil {
		return err
	}
	if err := bumpMetricsCount(ctx, tx, -old.packedBytes, -old.words, -int64(len(oldWords))); err != nil {
		return err
	}
	if err := bumpLengthHist(ctx, tx, nil, oldWords); err != nil {
		return err
	}
	if err := bumpMetricKey(ctx, tx, metricTextBytesRaw, -old.rawBytes); err != nil {
		return err
	}
	if err := subtractBookTextBytes(ctx, tx, bookID); err != nil {
		return err
	}
	return replaceBookRow(ctx, wt, bookID, w, authorID)
}

// insertBookRow 插入书籍主体并返回新 id。
func insertBookRow(ctx context.Context, tx *sql.Tx, w bookWrite, authorID int64) (int64, error) {
	res, err := tx.ExecContext(ctx, bookInsertSQL, w.execArgsWithQuality(authorID)...)
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
// 旧块**不在本事务里删**：chapter_blocks 上有 UNIQUE(book_id, seq)，重写同一本书
// 必须先腾出 seq，所以这里读旧块 id 挂到「主库提交后回收」清单（contentTx.scheduleReap）。
// 正文库事务先于主库提交，此刻真删而主库又提交失败的话，旧章节会指向不存在的块——
// 那比留孤儿块严重得多（见 content.go）。
func replaceBookRow(ctx context.Context, wt *writeTx, bookID int64, w bookWrite, authorID int64) error {
	tx := wt.main
	if _, err := tx.ExecContext(ctx, "DELETE FROM chapters WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear chapters: %w", err)
	}
	oldBlocks, err := wt.content.blockIDs(ctx, bookID)
	if err != nil {
		return err
	}
	for _, id := range oldBlocks {
		wt.content.scheduleReap(id)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM volumes WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear volumes: %w", err)
	}
	// 旧封面必须一起清掉：has_cover 由本语句重写，book_covers 留着就成了孤儿行，
	// 前端会按 has_cover=0 走占位、而库里仍占着 2 MB。
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_covers WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: save book clear cover: %w", err)
	}
	args := append(w.execArgsWithQuality(authorID), bookID)
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
	title, err := dedupeVolumeTitle(ctx, tx, bookID, strings.TrimSpace(v.Title))
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO volumes(book_id, idx, title) VALUES (?, ?, ?)",
		bookID, v.Idx, title)
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
