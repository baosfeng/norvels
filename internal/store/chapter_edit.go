package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// 章节结构编辑（管理页 M1）：改标题 / 删除 / 拆分（拆分见 chapter_split.go）。
//
// 这三件事都**不重压正文**：正文按 span 存（chapters.block_id + block_off + block_len），
// 改标题只动文本列，删除只去掉一个 span，拆分只是把一个 span 切成几段。
// 因此它们都是毫秒级的，不需要「重建整本书的块再回填偏移」那套底座。
//
// 每条写路径都守着同一组不变量：
//   - chapters.idx 在同一本书内连续（0 起），删除与拆分后立即重排；
//   - books.chapter_count / word_count 与 chapters 的实际行同步更新；
//   - store_metrics 的聚合值随写入在同一事务内增减（不留漂移）。

const (
	// maxChapterTitleRunes 是章节标题上限：与书籍元数据同一量级，够长标题用。
	maxChapterTitleRunes = 200
	// maxSplitPieces 是一次拆分最多产出的章数（拆分点 ≤ 99）。
	maxSplitPieces = 100
)

// chapterSpan 是一章在压缩块里的定位与元信息。
type chapterSpan struct {
	id        int64
	idx       int
	volumeID  int64
	title     string
	wordCount int
	blockID   int64
	offset    int
	length    int
}

// UpdateChapterTitle 改一章的标题；去空白后为空或超过 200 字都拒绝。
func (s *Store) UpdateChapterTitle(ctx context.Context, bookID, chapterID int64, title string) error {
	clean, err := cleanChapterTitle(title)
	if err != nil {
		return err
	}
	if err := checkIDs(bookID, chapterID); err != nil {
		return err
	}
	return s.inTx(ctx, "update chapter title", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			"UPDATE chapters SET title = ? WHERE id = ? AND book_id = ?", clean, chapterID, bookID)
		if err != nil {
			return fmt.Errorf("store: update chapter title %d: %w", chapterID, err)
		}
		if err := requireOneRow(res, bookID); err != nil {
			return err
		}
		// 标题变了，书的「最后更新」也该动：详情页与列表都按 updated_at 排序。
		return touchBookUpdatedAt(ctx, tx, bookID)
	})
}

// cleanChapterTitle 归一化并校验章节标题。
func cleanChapterTitle(title string) (string, error) {
	clean := strings.TrimSpace(title)
	if clean == "" {
		return "", fmt.Errorf("%w: 章节标题不能为空", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(clean) > maxChapterTitleRunes {
		return "", fmt.Errorf("%w: 章节标题超过 %d 字", ErrInvalidArgument, maxChapterTitleRunes)
	}
	return clean, nil
}

// DeleteChapter 删掉一章并把它后面的章节序号依次前移。
//
// 正文所在的块若因此没有任何章节引用，整块一起删掉并冲减 metrics——
// 否则那块压缩数据会永远留在库里，/api/stats 的正文体积也降不下来。
func (s *Store) DeleteChapter(ctx context.Context, bookID, chapterID int64) error {
	if err := checkIDs(bookID, chapterID); err != nil {
		return err
	}
	return s.inTx(ctx, "delete chapter", func(tx *sql.Tx) error {
		sp, err := loadChapterSpan(ctx, tx, bookID, chapterID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM chapters WHERE id = ? AND book_id = ?", chapterID, bookID); err != nil {
			return fmt.Errorf("store: delete chapter %d: %w", chapterID, err)
		}
		if err := shiftChaptersIdx(ctx, tx, bookID, sp.idx, -1); err != nil {
			return err
		}
		if err := adjustBookCounts(ctx, tx, bookID, -1, -sp.wordCount); err != nil {
			return err
		}
		if err := bumpMetrics(ctx, tx, 0, -int64(sp.wordCount)); err != nil {
			return err
		}
		if err := reapOrphanBlock(ctx, tx, sp.blockID); err != nil {
			return err
		}
		return refreshBlockIdxRange(ctx, tx, sp.blockID)
	})
}

// loadChapterSpan 读一章的块定位与元信息；不属于这本书或不存在都返回 ErrNotFound。
func loadChapterSpan(ctx context.Context, tx *sql.Tx, bookID, chapterID int64) (chapterSpan, error) {
	var sp chapterSpan
	err := tx.QueryRowContext(ctx,
		"SELECT id, idx, volume_id, title, word_count, block_id, block_off, block_len "+
			"FROM chapters WHERE id = ? AND book_id = ?", chapterID, bookID).
		Scan(&sp.id, &sp.idx, &sp.volumeID, &sp.title, &sp.wordCount,
			&sp.blockID, &sp.offset, &sp.length)
	if errors.Is(err, sql.ErrNoRows) {
		return sp, fmt.Errorf("%w: chapter %d of book %d", ErrNotFound, chapterID, bookID)
	}
	if err != nil {
		return sp, fmt.Errorf("store: load chapter span %d: %w", chapterID, err)
	}
	return sp, nil
}

// spanText 取出一章的正文字节（来自解压后的块）；空章或块缺失都按损坏处理。
func spanText(ctx context.Context, tx *sql.Tx, sp chapterSpan) ([]byte, error) {
	if sp.blockID <= 0 {
		return nil, fmt.Errorf("%w: 第 %d 章没有正文块", ErrBlockCorrupted, sp.idx+1)
	}
	block, err := loadBlock(ctx, tx, sp.blockID, sp.id)
	if err != nil {
		return nil, err
	}
	if sp.offset < 0 || sp.length <= 0 || sp.offset+sp.length > len(block) {
		return nil, fmt.Errorf("%w: 第 %d 章正文越界（off=%d len=%d block=%d）",
			ErrBlockCorrupted, sp.idx+1, sp.offset, sp.length, len(block))
	}
	return block[sp.offset : sp.offset+sp.length], nil
}

// shiftChaptersIdx 把 idx 大于 fromIdx 的章节整体平移 delta（删除 -1、拆分 +n）。
//
// chapters 上没有 UNIQUE(book_id, idx) 约束，单条 UPDATE 一次平移不会撞约束；
// 顺序也不需要倒着走。
func shiftChaptersIdx(ctx context.Context, tx *sql.Tx, bookID int64, fromIdx, delta int) error {
	if delta == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		"UPDATE chapters SET idx = idx + ? WHERE book_id = ? AND idx > ?", delta, bookID, fromIdx)
	if err != nil {
		return fmt.Errorf("store: shift chapter idx %d: %w", bookID, err)
	}
	return nil
}

// adjustBookCounts 按增量改书级计数（章节数与字数），书不存在返回 ErrNotFound。
func adjustBookCounts(ctx context.Context, tx *sql.Tx, bookID int64, chapterDelta, wordDelta int) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE books SET chapter_count = chapter_count + ?, word_count = word_count + ?, updated_at = ? "+
			"WHERE id = ?", chapterDelta, wordDelta, formatTime(nowUTC()), bookID)
	if err != nil {
		return fmt.Errorf("store: adjust book counts %d: %w", bookID, err)
	}
	return requireOneRow(res, bookID)
}

// touchBookUpdatedAt 只刷新书的 updated_at（改章节标题用）。
func touchBookUpdatedAt(ctx context.Context, tx *sql.Tx, bookID int64) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", formatTime(nowUTC()), bookID)
	if err != nil {
		return fmt.Errorf("store: touch book %d: %w", bookID, err)
	}
	return requireOneRow(res, bookID)
}

// requireOneRow 把「影响 0 行」翻译成 ErrNotFound（书或章节已经不在了）。
func requireOneRow(res sql.Result, bookID int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: rows affected book %d: %w", bookID, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: book %d", ErrNotFound, bookID)
	}
	return nil
}

// reapOrphanBlock 删掉不再被任何章节引用的正文块，并把它的体积从 metrics 里减去。
func reapOrphanBlock(ctx context.Context, tx *sql.Tx, blockID int64) error {
	if blockID <= 0 {
		return nil
	}
	var referenced int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapters WHERE block_id = ?", blockID).Scan(&referenced); err != nil {
		return fmt.Errorf("store: count block refs %d: %w", blockID, err)
	}
	if referenced > 0 {
		return nil
	}
	var raw, packed int64
	if err := tx.QueryRowContext(ctx,
		"SELECT raw_bytes, length(packed) FROM chapter_blocks WHERE id = ?", blockID).
		Scan(&raw, &packed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("store: read orphan block %d: %w", blockID, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM chapter_blocks WHERE id = ?", blockID); err != nil {
		return fmt.Errorf("store: delete orphan block %d: %w", blockID, err)
	}
	if err := bumpMetrics(ctx, tx, -packed, 0); err != nil {
		return err
	}
	return bumpMetricKey(ctx, tx, metricTextBytesRaw, -raw)
}

// refreshBlockIdxRange 把块的 first_idx/last_idx 校正到当前实际覆盖的章节范围。
//
// 这两个字段只用于块损坏时的定位文案（BlockError），但让它们随增删漂移会误导排查。
func refreshBlockIdxRange(ctx context.Context, tx *sql.Tx, blockID int64) error {
	if blockID <= 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		"UPDATE chapter_blocks SET "+
			"first_idx = COALESCE((SELECT MIN(idx) FROM chapters WHERE block_id = chapter_blocks.id), first_idx), "+
			"last_idx = COALESCE((SELECT MAX(idx) FROM chapters WHERE block_id = chapter_blocks.id), last_idx) "+
			"WHERE id = ?", blockID)
	if err != nil {
		return fmt.Errorf("store: refresh block range %d: %w", blockID, err)
	}
	return nil
}

// inTx 在单事务内执行 fn，成功才提交（本文件的写路径共用）。
func (s *Store) inTx(ctx context.Context, what string, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %s begin: %w", what, err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: %s commit: %w", what, err)
	}
	return nil
}

// checkIDs 统一校验书籍/章节 id 为正。
func checkIDs(bookID, chapterID int64) error {
	if bookID <= 0 || chapterID <= 0 {
		return fmt.Errorf("%w: book %d chapter %d", ErrInvalidArgument, bookID, chapterID)
	}
	return nil
}
