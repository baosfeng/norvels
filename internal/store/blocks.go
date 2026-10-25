package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// 0007 的数据搬迁步骤：把 chapters.content 的原文搬进 chapter_blocks 压缩块。
//
// 为什么放在 Go 里而不是纯 SQL：块边界要按「章节边界 + 1 MB 目标」切，压缩要调
// zstd，两者都没法用 SQL 表达。放进迁移事务是为了守住「失败即整体回滚」：
// 搬一半失败时，库必须回到 0006 状态（正文仍是原文、一个字节都没少）。

// bookChunk 是逐书搬迁时的游标行：只需要 id，其余按需流式读取。
type bookChunk struct {
	bookID int64
}

// rebuildChaptersForBlocks 是 0007 的迁移步骤：逐书把正文压进 chapter_blocks。
//
// 幂等判定：chapters 没有 content 列（已经搬完）或表不存在 → 直接返回。
// 逐书处理而不是全库一次性读入：单本最大约 40 MB 正文，内存占用有界；
// 422,715 章的全量库不会把正文一次吃进内存。
func rebuildChaptersForBlocks(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "chapters")
	if err != nil || !exists {
		return err
	}
	hasContent, err := columnExistsInTx(ctx, tx, "chapters", "content")
	if err != nil {
		return err
	}
	if !hasContent {
		return nil // 已迁移过
	}
	if err := ensureBlockColumns(ctx, tx); err != nil {
		return err
	}

	books, err := listBooksWithChapters(ctx, tx)
	if err != nil {
		return err
	}
	for _, bookID := range books {
		if err := migrateBookToBlocks(ctx, tx, bookID); err != nil {
			return fmt.Errorf("book %d: %w", bookID, err)
		}
	}
	return dropContentColumn(ctx, tx)
}

// ensureBlockColumns 补上 chapters 的块寻址三列（幂等）。
func ensureBlockColumns(ctx context.Context, tx *sql.Tx) error {
	columns := []struct{ name, ddl string }{
		{"block_id", "ALTER TABLE chapters ADD COLUMN block_id INTEGER NOT NULL DEFAULT 0"},
		{"block_off", "ALTER TABLE chapters ADD COLUMN block_off INTEGER NOT NULL DEFAULT 0"},
		{"block_len", "ALTER TABLE chapters ADD COLUMN block_len INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range columns {
		has, err := columnExistsInTx(ctx, tx, "chapters", c.name)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := tx.ExecContext(ctx, c.ddl); err != nil {
			return fmt.Errorf("add column %s: %w", c.name, err)
		}
	}
	return nil
}

// listBooksWithChapters 返回「有章节的书」按 id 升序，供逐书搬迁。
//
// 先取 id 再逐书读章节（而不是一条大 JOIN 全量扫描）：这样每本书的正文
// 在内存里只存在一次，且失败时容易定位到具体是哪本书。
func listBooksWithChapters(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT DISTINCT book_id FROM chapters ORDER BY book_id")
	if err != nil {
		return nil, fmt.Errorf("list books with chapters: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0, 1024)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan book id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate book ids: %w", err)
	}
	return ids, nil
}

// chapterRow 是搬迁时从旧表读出的一章。
type chapterRow struct {
	id      int64
	idx     int
	content string
}

// migrateBookToBlocks 把一本书的所有章节打成块并回填块寻址。
func migrateBookToBlocks(ctx context.Context, tx *sql.Tx, bookID int64) error {
	chapters, err := readBookChapters(ctx, tx, bookID)
	if err != nil {
		return err
	}
	if len(chapters) == 0 {
		return nil
	}
	contents := make([]string, len(chapters))
	sizes := make([]int, len(chapters))
	for i := range chapters {
		contents[i] = chapters[i].content
		sizes[i] = len(chapters[i].content)
	}
	for seq, span := range splitBlocks(sizes) {
		if err := writeBlock(ctx, tx, bookID, chapters, contents, span, seq+1); err != nil {
			return err
		}
	}
	return nil
}

// readBookChapters 读出一本书的全部章节正文（按 idx、id 排序，与目录顺序一致）。
func readBookChapters(ctx context.Context, tx *sql.Tx, bookID int64) ([]chapterRow, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, idx, content FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("read chapters: %w", err)
	}
	defer rows.Close()

	out := make([]chapterRow, 0, 128)
	for rows.Next() {
		var c chapterRow
		if err := rows.Scan(&c.id, &c.idx, &c.content); err != nil {
			return nil, fmt.Errorf("scan chapter: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chapters: %w", err)
	}
	return out, nil
}

// writeBlock 把一个章节区间压成一块，写 chapter_blocks 并回填各章的块寻址。
//
// 全空块（整块正文都是空串）不建块行：chapters 的 block_id 保持 0，
// 读取侧见到 0 直接返回空正文，不必为一个空块占一行 + 一次解压。
func writeBlock(ctx context.Context, tx *sql.Tx, bookID int64, chapters []chapterRow,
	contents []string, span [2]int, seq int) error {
	first, last := span[0], span[1]
	segment := contents[first : last+1]
	raw, offsets := joinChapters(segment)

	if len(raw) == 0 {
		return nil // 全空块：block_id 保持 0
	}
	packed, err := compressContent(raw)
	if err != nil {
		return fmt.Errorf("compress block: %w", err)
	}
	blockID, err := insertBlock(ctx, tx, bookID, chapters, span, seq, len(raw), packed)
	if err != nil {
		return err
	}
	return fillBlockRefs(ctx, tx, chapters, first, last, offsets, blockID)
}

// insertBlock 写入一块并返回块 id。
func insertBlock(ctx context.Context, tx *sql.Tx, bookID int64, chapters []chapterRow,
	span [2]int, seq, rawBytes int, packed []byte) (int64, error) {
	first, last := span[0], span[1]
	res, err := tx.ExecContext(ctx,
		"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, seq, chapters[first].idx, chapters[last].idx, rawBytes, packed)
	if err != nil {
		return 0, fmt.Errorf("insert block: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("block id: %w", err)
	}
	return id, nil
}

// fillBlockRefs 回填一个块覆盖的各章：block_id / block_off / block_len。
func fillBlockRefs(ctx context.Context, tx *sql.Tx, chapters []chapterRow,
	first, last int, offsets []int, blockID int64) error {
	stmt, err := tx.PrepareContext(ctx,
		"UPDATE chapters SET block_id = ?, block_off = ?, block_len = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("prepare block refs: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	for i := first; i <= last; i++ {
		pos := i - first
		length := len(chapters[i].content)
		if _, err := stmt.ExecContext(ctx, blockID, offsets[pos], length, chapters[i].id); err != nil {
			return fmt.Errorf("update chapter %d block refs: %w", chapters[i].id, err)
		}
	}
	return nil
}

// dropContentColumn 去掉 chapters.content（正文已全部搬进块）。
//
// 这一步是 SQLite 的原地重建：它会把整表复制一遍再换名，因此**也是真正回收
// 那几 GB 空间的时刻**。迁移期间峰值磁盘 ≈ 旧正文 + 新压缩块，运维需要预留。
func dropContentColumn(ctx context.Context, tx *sql.Tx) error {
	has, err := columnExistsInTx(ctx, tx, "chapters", "content")
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE chapters DROP COLUMN content"); err != nil {
		return fmt.Errorf("drop content column: %w", err)
	}
	return nil
}

// blockMeta 是一个压缩块的定位元信息（加载失败时要原样带进错误里）。
type blockMeta struct {
	id     int64
	bookID int64
	seq    int
	// firstIdx / lastIdx 来自 chapter_blocks.first_idx / last_idx，
	// 语义是章节序号（chapters.idx），不是 chapter id。
	firstIdx int
	lastIdx  int
}

// loadBlock 按块 id 取出压缩字节并解压成整块原始内容。
//
// store 层不做块缓存：一块约 1 ms，缓存收益低且会让 Store 带状态。
// 需要连续读多章（预加载下一章）时由调用方自己持有缓存。
//
// 失败时返回 *BlockError（带 book_id / seq / 章节范围）：一个块覆盖约 35 章，
// 报错必须能定位到「哪本书的哪一块」，否则几万本书的库里没法排查。
// 也可以传 chapterID（0 表示块级校验），用于告诉调用方「是哪一章触发的」。
func loadBlock(ctx context.Context, q rowQuerier, blockID, chapterID int64) ([]byte, error) {
	if blockID <= 0 {
		return nil, nil
	}
	meta, packed, err := readBlockRow(ctx, q, blockID)
	if err != nil {
		return nil, err
	}
	block, derr := decompressContent(packed)
	if derr != nil {
		return nil, &BlockError{
			BlockID: meta.id, BookID: meta.bookID, Seq: meta.seq,
			FirstIdx: meta.firstIdx, LastIdx: meta.lastIdx,
			ChapterID: chapterID, Cause: derr,
		}
	}
	return block, nil
}

// readBlockRowMetaOnly 只读块的定位元信息（不含 packed，避免为报错拉一个 1 MB 的 BLOB）。
func readBlockRowMetaOnly(ctx context.Context, q rowQuerier, blockID int64) (blockMeta, error) {
	var meta blockMeta
	err := q.QueryRowContext(ctx,
		"SELECT id, book_id, seq, first_idx, last_idx FROM chapter_blocks WHERE id = ?", blockID).
		Scan(&meta.id, &meta.bookID, &meta.seq, &meta.firstIdx, &meta.lastIdx)
	if err != nil {
		return meta, fmt.Errorf("store: read block meta %d: %w", blockID, err)
	}
	return meta, nil
}

// readBlockRow 读出块的元信息与压缩字节；记录缺失也算块级损坏（块被删但章节仍指向它）。
func readBlockRow(ctx context.Context, q rowQuerier, blockID int64) (blockMeta, []byte, error) {
	var (
		meta   blockMeta
		packed []byte
	)
	err := q.QueryRowContext(ctx,
		"SELECT id, book_id, seq, first_idx, last_idx, packed FROM chapter_blocks WHERE id = ?", blockID).
		Scan(&meta.id, &meta.bookID, &meta.seq, &meta.firstIdx, &meta.lastIdx, &packed)
	if errors.Is(err, sql.ErrNoRows) {
		return meta, nil, fmt.Errorf("%w: block %d 不存在（章节仍指向它）", ErrBlockCorrupted, blockID)
	}
	if err != nil {
		return meta, nil, fmt.Errorf("store: load block %d: %w", blockID, err)
	}
	return meta, packed, nil
}

// rowQuerier 抽象 *sql.DB / *sql.Tx 的单行查询（让 loadBlock 两种都能用）。
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
