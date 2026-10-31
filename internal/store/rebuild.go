package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 0017 的数据重建步骤：把存量块按「8 MB 章节边界 + 0x03 码本」重做一遍。
//
// 为什么单独一个文件而不是塞进 blocks.go：0007 的搬迁是「原文 → 块」的单向转换，
// 只跑一次、只有一种输入形态；0017 是「块 → 块」的重编码，要同时照顾 0x00/0x01/0x02
// 三种老格式、跨块拼接、以及「换表」这一步。两者共享的只有 chapterAt / splitBlocks /
// joinChapters / compressContent 这几个基础函数。
//
// 内存：逐书处理，峰值 = 一本书的正文（生产最大约 75 MB）+ 一块解压结果（≤ 8 MB）。
// 绝不把多本书同时读进内存：87,626 个块的全量库一次读入会直接 OOM。
//
// 失败即整体回滚：换表是事务里的最后两条 DDL，任何一本书在重编码时报错，
// 库都停在 0017 之前的状态（老块一个字节没动、版本号不前进）。

const (
	blocksTable    = "chapter_blocks"     // 现役块表（0007 建）
	newBlocksTable = "chapter_blocks_new" // 0017 的重建目标表（本迁移 SQL 建）
)

// blockIndexDDL 是块表必须存在的两个索引（0007 建在旧表上）。
//
// 旧表随 DROP TABLE 一起带走索引，换名之后必须按**同名**重建：索引名在库里唯一，
// 先 DROP 再 RENAME 的顺序正是为了腾出这两个名字。
var blockIndexDDL = []string{
	"CREATE INDEX IF NOT EXISTS idx_chapter_blocks_range ON " + blocksTable + "(book_id, first_idx, last_idx)",
	"CREATE INDEX IF NOT EXISTS idx_chapter_blocks_book  ON " + blocksTable + "(book_id, seq)",
}

// rebuildChapter 是一章在重建时的全部输入：身份、旧块寻址、解出来的正文。
type rebuildChapter struct {
	id      int64
	idx     int
	blockID int64
	off     int
	length  int
	content string
}

// rebuildBlocksForCodebook 是 0017 的迁移步骤：逐书重编码，最后原子换表。
//
// 幂等：库里已经没有 0x01/0x02 老块、且所有块 raw_bytes ≤ 8 MB 时直接返回
// （重跑安全——删掉版本记录重跑、或人工重放这条迁移都不会改动任何数据）。
func rebuildBlocksForCodebook(ctx context.Context, tx *sql.Tx) error {
	hasNew, err := tableExistsInTx(ctx, tx, newBlocksTable)
	if err != nil {
		return err
	}
	if !hasNew {
		return fmt.Errorf("store: 迁移 0017 缺少目标表 %s（迁移 SQL 未执行？）", newBlocksTable)
	}
	need, err := blocksNeedRebuild(ctx, tx)
	if err != nil {
		return err
	}
	if !need {
		// 已经是目标形态：目标表是空的，删掉它，让库形态与迁移前完全一致。
		_, err := tx.ExecContext(ctx, "DROP TABLE "+newBlocksTable)
		return err
	}
	books, err := listBooksWithChapters(ctx, tx)
	if err != nil {
		return err
	}
	for _, bookID := range books {
		if err := rebuildBookBlocks(ctx, tx, bookID); err != nil {
			return fmt.Errorf("book %d: %w", bookID, err)
		}
	}
	return swapBlocksTable(ctx, tx)
}

// blocksNeedRebuild 判断库里还有没有「老格式的块」：0x01/0x02 的压缩块，
// 或超过 8 MB 目标的块（多章块）。两者都为 0 说明已经是 0017 的目标形态。
func blocksNeedRebuild(ctx context.Context, tx *sql.Tx) (bool, error) {
	exists, err := tableExistsInTx(ctx, tx, blocksTable)
	if err != nil || !exists {
		return false, err
	}
	var n int64
	err = tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+blocksTable+
			" WHERE raw_bytes > ? OR substr(packed, 1, 1) IN (x'01', x'02')",
		BlockTargetBytes).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: 检查块是否需要重建: %w", err)
	}
	return n > 0, nil
}

// rebuildBookBlocks 把一本书的全部章节重编码进新表（逐书调用，内存有界）。
func rebuildBookBlocks(ctx context.Context, tx *sql.Tx, bookID int64) error {
	chapters, err := readBookChaptersForRebuild(ctx, tx, bookID)
	if err != nil {
		return err
	}
	if len(chapters) == 0 {
		return nil
	}
	if err := fillRebuildContents(ctx, tx, chapters); err != nil {
		return err
	}
	sizes := make([]int, len(chapters))
	for i := range chapters {
		sizes[i] = len(chapters[i].content)
	}
	seq := 0
	for _, span := range splitBlocks(sizes) {
		written, err := rebuildBlockSpan(ctx, tx, bookID, chapters, seq+1, span)
		if err != nil {
			return err
		}
		if written {
			seq++
		}
	}
	return nil
}

// readBookChaptersForRebuild 按目录顺序读一本书的章节身份与旧块寻址（不读正文列，正文在块里）。
func readBookChaptersForRebuild(ctx context.Context, tx *sql.Tx, bookID int64) ([]rebuildChapter, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, idx, block_id, block_off, block_len FROM chapters "+
			"WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("read chapters for rebuild: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	out := make([]rebuildChapter, 0, 256)
	for rows.Next() {
		var c rebuildChapter
		if err := rows.Scan(&c.id, &c.idx, &c.blockID, &c.off, &c.length); err != nil {
			return nil, fmt.Errorf("scan chapter ref: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chapter refs: %w", err)
	}
	return out, nil
}

// fillRebuildContents 逐章解出正文，复用**同一块的解压结果**（块在目录顺序上连续）。
//
// 只保留「当前块」的解压缓存：块的覆盖范围与目录顺序一致，顺着走一块只解一次；
// 万一章节的块归属不连续（人工改过库），退化成多解一次，正确性不受影响。
func fillRebuildContents(ctx context.Context, tx *sql.Tx, chapters []rebuildChapter) error {
	var (
		curBlockID int64 = -1
		curBlock   []byte
	)
	for i := range chapters {
		c := &chapters[i]
		if c.blockID == 0 {
			if c.length != 0 {
				return fmt.Errorf("%w: chapter %d has length %d but no block",
					ErrCorruptContent, c.id, c.length)
			}
			continue
		}
		if c.blockID != curBlockID {
			block, err := loadBlock(ctx, tx, c.blockID, c.id)
			if err != nil {
				return err // *BlockError：带 book_id / seq / 章节范围
			}
			curBlockID, curBlock = c.blockID, block
		}
		text, err := chapterAt(curBlock, c.off, c.length)
		if err != nil {
			return wrapBlockSpanInTx(ctx, tx, c.blockID, c.id, err)
		}
		c.content = text
	}
	return nil
}

// wrapBlockSpanInTx 在事务内把「块内区间越界」包成带 book_id / seq 的 *BlockError，
// 与读路径的 wrapBlockSpanError 同口径（那边用 *sql.DB，这里用 *sql.Tx）。
func wrapBlockSpanInTx(ctx context.Context, tx *sql.Tx, blockID, chapterID int64, cause error) error {
	meta, err := readBlockRowMetaOnly(ctx, tx, blockID)
	if err != nil {
		return fmt.Errorf("store: chapter %d: %w", chapterID, cause)
	}
	return &BlockError{
		BlockID: meta.id, BookID: meta.bookID, Seq: meta.seq,
		FirstIdx: meta.firstIdx, LastIdx: meta.lastIdx,
		ChapterID: chapterID, Cause: cause,
	}
}

// rebuildBlockSpan 把一个章节区间拼成原始字节、压成 0x03 并写进新表，返回是否落了一行。
//
// 全空区间不建块行：与 0007 的 writeBlock 同一口径——chapters 的 block_id 保持 0，
// 读取侧见到 0 直接返回空正文。这里必须把旧块寻址清掉，否则章节还指着即将被删的旧块。
func rebuildBlockSpan(ctx context.Context, tx *sql.Tx, bookID int64, chapters []rebuildChapter,
	seq int, span [2]int) (bool, error) {
	first, last := span[0], span[1]
	raw, offsets := joinChapters(chapterTexts(chapters[first : last+1]))
	if len(raw) == 0 {
		return false, clearChapterBlockRefs(ctx, tx, chapters, first, last)
	}
	packed, err := compressContent(raw)
	if err != nil {
		return false, fmt.Errorf("compress block seq %d: %w", seq, err)
	}
	blockID, err := insertRebuiltBlock(ctx, tx, bookID, chapters, span, seq, len(raw), packed)
	if err != nil {
		return false, err
	}
	if err := fillRebuiltBlockRefs(ctx, tx, chapters, first, last, offsets, blockID); err != nil {
		return false, err
	}
	return true, nil
}

// chapterTexts 取出若干章的正文（joinChapters 的入参形态）。
func chapterTexts(chapters []rebuildChapter) []string {
	out := make([]string, len(chapters))
	for i := range chapters {
		out[i] = chapters[i].content
	}
	return out
}

// insertRebuiltBlock 往目标表写一块并返回块 id。
func insertRebuiltBlock(ctx context.Context, tx *sql.Tx, bookID int64, chapters []rebuildChapter,
	span [2]int, seq, rawBytes int, packed []byte) (int64, error) {
	first, last := span[0], span[1]
	res, err := tx.ExecContext(ctx,
		"INSERT INTO "+newBlocksTable+"(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, seq, chapters[first].idx, chapters[last].idx, rawBytes, packed)
	if err != nil {
		return 0, fmt.Errorf("insert rebuilt block: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("rebuilt block id: %w", err)
	}
	return id, nil
}

// fillRebuiltBlockRefs 回填一个块覆盖的各章：block_id / block_off / block_len。
func fillRebuiltBlockRefs(ctx context.Context, tx *sql.Tx, chapters []rebuildChapter,
	first, last int, offsets []int, blockID int64) error {
	stmt, err := tx.PrepareContext(ctx,
		"UPDATE chapters SET block_id = ?, block_off = ?, block_len = ? WHERE id = ?")
	if err != nil {
		return fmt.Errorf("prepare rebuilt refs: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	for i := first; i <= last; i++ {
		pos := i - first
		if _, err := stmt.ExecContext(ctx, blockID, offsets[pos], len(chapters[i].content), chapters[i].id); err != nil {
			return fmt.Errorf("update chapter %d rebuilt refs: %w", chapters[i].id, err)
		}
	}
	return nil
}

// clearChapterBlockRefs 把全空区间的章节解引用（block_id=0 即「空章」，读取侧不建块）。
func clearChapterBlockRefs(ctx context.Context, tx *sql.Tx, chapters []rebuildChapter, first, last int) error {
	stmt, err := tx.PrepareContext(ctx,
		"UPDATE chapters SET block_id = 0, block_off = 0, block_len = 0 WHERE id = ?")
	if err != nil {
		return fmt.Errorf("prepare clear refs: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	for i := first; i <= last; i++ {
		if _, err := stmt.ExecContext(ctx, chapters[i].id); err != nil {
			return fmt.Errorf("clear chapter %d refs: %w", chapters[i].id, err)
		}
	}
	return nil
}

// swapBlocksTable 是 0017 的落地点：删旧表、新表改名顶上、按同名重建索引。
//
// 顺序不能反：索引名（idx_chapter_blocks_range / _book）在库里唯一，
// 只有先 DROP 掉带索引的旧表，RENAME 之后才能用原名建索引。
func swapBlocksTable(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS " + blocksTable,
		"ALTER TABLE " + newBlocksTable + " RENAME TO " + blocksTable,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store: 切换块表失败（%s）: %w", stmt, err)
		}
	}
	for _, ddl := range blockIndexDDL {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("store: 重建块索引失败: %w", err)
		}
	}
	return nil
}
