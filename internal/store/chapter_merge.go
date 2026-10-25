package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// 合并相邻章节（管理页 M1，与 chapter_split.go 的拆分互逆）。
//
// 与拆分一样不重压正文：若各章本就在同一块的连续区间（拆分留下的典型形态），
// 合并只是把首章的 span 拉长到覆盖整段。只有跨块的合并才必须新建一个块——
// 跨块的正文在物理上不相邻，一次 span 表达不了。新建块时旧块若因此无人引用，
// 由 reapBlocksOf 整块回收并从 metrics 里冲减，体积不会虚增。

// MergeChapters 把 cids 指定的相邻同卷章节合并成一章，保留**首章 id**。
//
// 首章保留 id 的理由：阅读进度（reading_progress）与书签都按 chapter id 指过来，
// 保住目录里的第一章就是保住读者的位置。标题取首章原标题（不重编号——那由
// RenumberChapters 负责），正文按章序拼接。三处计数在同一事务里按增量维护，
// books.updated_at 一并刷新（详情页与列表都按它排序）。
func (s *Store) MergeChapters(ctx context.Context, bookID int64, cids []int64) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	ids, err := normalizeMergeIDs(cids)
	if err != nil {
		return err
	}
	return s.inTx(ctx, "merge chapters", func(tx *sql.Tx) error {
		spans, err := loadChapterSpans(ctx, tx, bookID, ids)
		if err != nil {
			return err
		}
		// 拼接顺序由目录顺序（idx）决定，不跟调用方给的参数顺序走：
		// 正文顺序是数据事实，不能因为请求把 id 排反了就翻过来。
		sort.SliceStable(spans, func(i, j int) bool { return spans[i].idx < spans[j].idx })
		if err := checkMergeable(spans); err != nil {
			return err
		}
		return applyMerge(ctx, tx, bookID, spans)
	})
}

// normalizeMergeIDs 校验合并入参：空列表算参数错误，只有一章则「不可合并」。
func normalizeMergeIDs(cids []int64) ([]int64, error) {
	ids, err := normalizeChapterIDs(cids)
	if err != nil {
		return nil, err
	}
	if len(ids) < 2 {
		return nil, fmt.Errorf("%w: 少于 2 章（给了 %d 章）", ErrNotMergeable, len(ids))
	}
	return ids, nil
}

// checkMergeable 校验「同卷 + 相邻连续」，不满足时给出可判定的错误与具体原因。
//
// 跨卷与不连续都是**结构**层面的拒绝，不是参数写错：调用方（api 层）要能把它们
// 映射成 409 并直接展示原因，因此用 ErrNotMergeable 而不是 ErrInvalidArgument。
func checkMergeable(spans []chapterSpan) error {
	head := spans[0]
	for _, sp := range spans[1:] {
		if sp.volumeID != head.volumeID {
			return fmt.Errorf("%w: 跨卷（第 %d 章在卷 %d，第 %d 章在卷 %d）",
				ErrNotMergeable, head.idx+1, head.volumeID, sp.idx+1, sp.volumeID)
		}
	}
	for i := 1; i < len(spans); i++ {
		if spans[i].idx != spans[i-1].idx+1 {
			return fmt.Errorf("%w: 不连续（第 %d 章之后是第 %d 章）",
				ErrNotMergeable, spans[i-1].idx+1, spans[i].idx+1)
		}
	}
	return nil
}

// applyMerge 落实合并：算新 span → 改写首章 → 删其余章 → 重排序号与计数 → 回收旧块。
func applyMerge(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan) error {
	ref, packed, raw, err := mergedSpan(ctx, tx, bookID, spans)
	if err != nil {
		return err
	}
	if err := rewriteMergedHead(ctx, tx, bookID, spans, ref); err != nil {
		return err
	}
	if err := deleteChapterRows(ctx, tx, bookID, tailChapterIDs(spans)); err != nil {
		return err
	}
	if err := renumberChapterIdx(ctx, tx, bookID); err != nil {
		return err
	}
	if err := applyMergeCounts(ctx, tx, bookID, spans, packed, raw); err != nil {
		return err
	}
	if err := reapBlocksOf(ctx, tx, spans); err != nil {
		return err
	}
	return refreshBlockIdxRange(ctx, tx, ref.id)
}

// mergedSpan 计算合并后首章的 span，并返回并入 metrics 的字节增量。
func mergedSpan(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan) (blockRef, int64, int64, error) {
	if ref, ok := contiguousRef(spans); ok {
		return ref, 0, 0, nil
	}
	text, err := concatSpanText(ctx, tx, spans)
	if err != nil {
		return blockRef{}, 0, 0, err
	}
	return writeMergeBlock(ctx, tx, bookID, spans, text)
}

// contiguousRef 判断各章是否本就在同一块的连续区间；是则返回合并后的 span。
//
// 判据是「同一块 + 前一段的 end 正好是后一段的 start」。拆章只切 span、不重压，
// 拆完再合并回来必然命中这条，于是整体零拷贝：不用解压、不用重压、不改 metrics。
func contiguousRef(spans []chapterSpan) (blockRef, bool) {
	head := spans[0]
	if head.blockID <= 0 {
		return blockRef{}, false
	}
	end := head.offset + head.length
	for _, sp := range spans[1:] {
		if sp.blockID != head.blockID || sp.offset != end {
			return blockRef{}, false
		}
		end = sp.offset + sp.length
	}
	return blockRef{id: head.blockID, off: head.offset, len: end - head.offset}, true
}

// concatSpanText 按章序取出各章正文并拼接。
//
// 空章（block_id=0 且长度 0）贡献空串而不是报错：合并时把空章并进来是合法操作，
// 但「有长度却没有块」仍是块级损坏，必须照常报错而不是静默丢字。
func concatSpanText(ctx context.Context, tx *sql.Tx, spans []chapterSpan) ([]byte, error) {
	var buf []byte
	for _, sp := range spans {
		if sp.blockID <= 0 {
			if sp.length != 0 {
				return nil, fmt.Errorf("%w: 第 %d 章有长度 %d 却没有正文块",
					ErrBlockCorrupted, sp.idx+1, sp.length)
			}
			continue
		}
		raw, err := spanText(ctx, tx, sp)
		if err != nil {
			return nil, err
		}
		buf = append(buf, raw...)
	}
	return buf, nil
}

// writeMergeBlock 把合并后的正文压成一个新块，返回首章的新 span 与字节增量。
//
// 全空合并不建块（与写入侧「全空块不建块行」同一口径）：首章变成空章，block_id=0。
func writeMergeBlock(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan,
	text []byte) (blockRef, int64, int64, error) {
	raw := int64(len(text))
	if raw == 0 {
		return blockRef{}, 0, 0, nil
	}
	packed, err := compressContent(text)
	if err != nil {
		return blockRef{}, 0, 0, fmt.Errorf("store: merge compress block: %w", err)
	}
	id, err := insertMergeBlock(ctx, tx, bookID, spans, text, packed)
	if err != nil {
		return blockRef{}, 0, 0, err
	}
	return blockRef{id: id, off: 0, len: len(text)}, int64(len(packed)), raw, nil
}

// nextBlockSeq 返回本书下一个可用块序号（chapter_blocks 上有 UNIQUE(book_id, seq)）。
//
// 合并会留下 seq 空洞：被回收的块不会把序号让出来。seq 只用于块级错误文案里的
// 「第几块」，不参与寻址，取 MAX+1 比维护连续更划算。
func nextBlockSeq(ctx context.Context, tx *sql.Tx, bookID int64) (int, error) {
	var seq int
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) + 1 FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&seq); err != nil {
		return 0, fmt.Errorf("store: next block seq %d: %w", bookID, err)
	}
	return seq, nil
}

// insertMergeBlock 写入合并后的新块，返回块 id。
func insertMergeBlock(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan,
	raw, packed []byte) (int64, error) {
	seq, err := nextBlockSeq(ctx, tx, bookID)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, seq, spans[0].idx, spans[len(spans)-1].idx, len(raw), packed)
	if err != nil {
		return 0, fmt.Errorf("store: merge block insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: merge block id: %w", err)
	}
	return id, nil
}

// rewriteMergedHead 把首章指向合并后的 span，字数取各章 word_count 之和。
func rewriteMergedHead(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan,
	ref blockRef) error {
	head := spans[0]
	total := 0
	for _, sp := range spans {
		total += sp.wordCount
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE chapters SET block_id = ?, block_off = ?, block_len = ?, word_count = ? "+
			"WHERE id = ? AND book_id = ?",
		ref.id, ref.off, ref.len, total, head.id, bookID); err != nil {
		return fmt.Errorf("store: merge head chapter %d: %w", head.id, err)
	}
	return nil
}

// tailChapterIDs 返回除首章以外的章 id（按章序）。
func tailChapterIDs(spans []chapterSpan) []int64 {
	ids := make([]int64, 0, len(spans)-1)
	for _, sp := range spans[1:] {
		ids = append(ids, sp.id)
	}
	return ids
}

// applyMergeCounts 维护三处计数与新写块的字节增量。
//
// 字数**净变化为 0**：合并前这些字分摊在各章里，合并后全落在首章（首章字数取的
// 正是各章存量 word_count 之和），总数不变。因此只减章数、不动字数；books.updated_at
// 仍由 adjustBookCounts 顺带刷新。
//
// 为什么不用 countWords 重算拼接后的正文：章界处若原文没有分隔符，重算会把两个
// 英文词粘成一个，读者会觉得「合并吃字了」；存量口径才是读者看到的那一列。
func applyMergeCounts(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan,
	packed, raw int64) error {
	if err := adjustBookCounts(ctx, tx, bookID, -(len(spans) - 1), 0); err != nil {
		return err
	}
	if err := bumpMetrics(ctx, tx, packed, 0); err != nil {
		return err
	}
	return bumpMetricKey(ctx, tx, metricTextBytesRaw, raw)
}
