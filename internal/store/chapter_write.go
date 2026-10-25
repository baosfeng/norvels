package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"norvels/internal/model"
)

// 本文件负责「章节正文 → 压缩块」的写入侧，与 save.go 的书籍主体写入分开。
//
// 分开的理由不只是行数：块打包是一套独立的不变量（每块覆盖整数个章节、
// 块内偏移与章节长度严格对齐），把它和 books/authors/volumes 的写入混在
// 一个文件里，改动时很难一眼看出哪里碰了块口径。

// blockRef 是一章在块里的位置（id = 0 表示空章，无块）。
type blockRef struct {
	id  int64
	off int
	len int
}

// writeChapterBlocks 按块压缩写入正文，返回逐章的块寻址与压缩/原始字节总量。
//
// 全空块不建块行：block_id 保持 0，读取侧见到 0 直接返回空正文，
// 不必为一个空块占一行 + 一次解压。
func writeChapterBlocks(ctx context.Context, tx *sql.Tx, bookID int64,
	chapters []model.ParsedChapter, sizes []int, startSeq int) ([]blockRef, int64, int64, error) {
	refs := make([]blockRef, len(chapters))
	var packedTotal, rawTotal int64

	for seq, span := range splitBlocks(sizes) {
		first, last := span[0], span[1]
		contents := make([]string, 0, last-first+1)
		for i := first; i <= last; i++ {
			contents = append(contents, chapters[i].Content)
		}
		raw, offsets := joinChapters(contents)
		for i := first; i <= last; i++ {
			refs[i] = blockRef{off: offsets[i-first], len: sizes[i]}
		}
		rawTotal += int64(len(raw))
		if len(raw) == 0 {
			continue // 全空块
		}
		packed, err := compressContent(raw)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("store: compress block: %w", err)
		}
		id, err := insertChapterBlock(ctx, tx, bookID, startSeq+seq+1, chapters, span, raw, packed)
		if err != nil {
			return nil, 0, 0, err
		}
		packedTotal += int64(len(packed))
		for i := first; i <= last; i++ {
			refs[i].id = id
		}
	}
	return refs, packedTotal, rawTotal, nil
}

// insertChapterBlock 写入一块并返回块 id。
func insertChapterBlock(ctx context.Context, tx *sql.Tx, bookID int64, seq int,
	chapters []model.ParsedChapter, span [2]int, raw, packed []byte) (int64, error) {
	first, last := span[0], span[1]
	res, err := tx.ExecContext(ctx,
		"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, seq, chapters[first].Idx, chapters[last].Idx, len(raw), packed)
	if err != nil {
		return 0, fmt.Errorf("store: save block seq=%d: %w", seq, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: save block id: %w", err)
	}
	return id, nil
}

// insertChapterRows 批量写入章节行（只含块寻址与元信息），返回字数总和。
func insertChapterRows(ctx context.Context, tx *sql.Tx, bookID int64, chapters []model.ParsedChapter,
	volMap map[int]int64, refs []blockRef) (int64, error) {
	stmt, err := tx.PrepareContext(ctx, chapterInsertSQL)
	if err != nil {
		return 0, fmt.Errorf("store: save chapter prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	var words int64
	for i := range chapters {
		c := &chapters[i]
		wc := chapterWordCount(c)
		ref := refs[i]
		if _, err := stmt.ExecContext(ctx, bookID, volumeIDFor(c, volMap), c.Idx, c.Title, wc,
			ref.id, ref.off, ref.len); err != nil {
			return 0, fmt.Errorf("store: save chapter idx=%d: %w", c.Idx, err)
		}
		words += int64(wc)
	}
	return words, nil
}

// insertChapters 把一章正文按块压缩写入，并顺手累加 store_metrics 需要的聚合量。
//
// 写入分两步：先按「章节边界 + 1 MB 目标」把正文切成块并压进 chapter_blocks，
// 再写 chapters 行（只存块寻址与标题）。两步在同一事务内，失败一起回滚。
//
// 导入路径本来就要遍历所有章节，在这里累加几乎零额外成本——比事后跑
// SUM(length(packed))（要读全部压缩块）便宜得多。累加与章节写入在同一事务内完成，
// 二者不会漂移。
//
// 口径：text_bytes 统计**压缩后**的 packed 字节（整数列相加），text_bytes_raw
// 统计原文逻辑字节。两个口径都在这里累计，方便文档说明压缩比。
func insertChapters(ctx context.Context, tx *sql.Tx, bookID int64, b *model.ParsedBook, volMap map[int]int64) error {
	sizes := make([]int, len(b.Chapters))
	for i := range b.Chapters {
		sizes[i] = len(b.Chapters[i].Content)
	}

	refs, packedBytes, rawBytes, err := writeChapterBlocks(ctx, tx, bookID, b.Chapters, sizes, 0)
	if err != nil {
		return err
	}
	words, err := insertChapterRows(ctx, tx, bookID, b.Chapters, volMap, refs)
	if err != nil {
		return err
	}

	if err := bumpMetrics(ctx, tx, packedBytes, words); err != nil {
		return err
	}
	if err := bumpMetricKey(ctx, tx, metricTextBytesRaw, rawBytes); err != nil {
		return err
	}
	// books 侧文本量（标题/简介/路径等）随书籍主体变化，一并增量维护，
	// 保证「增量累加」与「全量重算」在同一个口径上（有测试守着）。
	if _, err := bumpBookTextBytes(ctx, tx, bookID); err != nil {
		return err
	}
	return nil
}

// ---------- 新建章节（P3 §管理侧补全） ----------

// insertPosition 是新章节的落点：目录序号与卷归属。
type insertPosition struct {
	idx      int
	volumeID int64
}

// InsertChapter 在指定章节之后插入一章；afterChapterID=0 表示追加到末尾。
//
// 正文走压缩块路径（与整章替换同一套 writeContentBlock）：新章独占一块、块内偏移 0，
// 因此插入不需要重压其它章，也不用改它们的 block_off。插入后 idx 顺延 + 重排，
// books.chapter_count / word_count 与 store_metrics 在同一事务内维护。
//
// 卷归属：插到某章之后时继承那一章的卷；追加到末尾时继承最后一章的卷。
// 「接着这本书往下写」不该把新章甩进未分卷，否则详情页会把同一本书的目录拆成两段。
func (s *Store) InsertChapter(ctx context.Context, bookID int64, ch model.ParsedChapter,
	afterChapterID int64) (int64, error) {
	if bookID <= 0 {
		return 0, fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if afterChapterID < 0 {
		return 0, fmt.Errorf("%w: afterChapterId %d 不能为负", ErrInvalidArgument, afterChapterID)
	}
	title, err := cleanChapterTitle(ch.Title)
	if err != nil {
		return 0, err
	}
	if err := checkChapterContent(ch.Content); err != nil {
		return 0, err
	}
	var newID int64
	err = s.inTx(ctx, "insert chapter", func(tx *sql.Tx) error {
		pos, err := insertPositionInTx(ctx, tx, bookID, afterChapterID)
		if err != nil {
			return err
		}
		newID, err = insertChapterRowInTx(ctx, tx, bookID, pos, title, &ch)
		return err
	})
	if err != nil {
		return 0, err
	}
	return newID, nil
}

// insertPositionInTx 决定新章的 idx 与卷归属；书不存在返回 ErrNotFound。
//
// 追加路径要顺带确认书存在：空书没有章节行可供「最后一章」推导，
// 不确认的话后面写块时才撞外键，错误信息会退化成 SQLite 的约束报错。
func insertPositionInTx(ctx context.Context, tx *sql.Tx, bookID, afterChapterID int64) (insertPosition, error) {
	if afterChapterID > 0 {
		sp, err := loadChapterSpan(ctx, tx, bookID, afterChapterID)
		if err != nil {
			return insertPosition{}, err
		}
		return insertPosition{idx: sp.idx + 1, volumeID: sp.volumeID}, nil
	}
	var (
		lastIdx  int
		volumeID int64
	)
	err := tx.QueryRowContext(ctx,
		"SELECT idx, volume_id FROM chapters WHERE book_id = ? ORDER BY idx DESC, id DESC LIMIT 1", bookID).
		Scan(&lastIdx, &volumeID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
			return insertPosition{}, err
		}
		return insertPosition{}, nil // 空书：idx 0、未分卷
	}
	if err != nil {
		return insertPosition{}, fmt.Errorf("store: last chapter of book %d: %w", bookID, err)
	}
	return insertPosition{idx: lastIdx + 1, volumeID: volumeID}, nil
}

// insertChapterRowInTx 写正文块与章节行，并同步三处计数。
func insertChapterRowInTx(ctx context.Context, tx *sql.Tx, bookID int64, pos insertPosition,
	title string, ch *model.ParsedChapter) (int64, error) {
	// 先把落点之后的章节整体后移，空出 idx：chapters 没有 UNIQUE(book_id, idx)，
	// 一条 UPDATE 平移即可（与删除/拆分同一套 shiftChaptersIdx）。
	if err := shiftChaptersIdx(ctx, tx, bookID, pos.idx-1, 1); err != nil {
		return 0, err
	}
	words := chapterWordCount(ch)
	ref, packed, raw, err := writeContentBlock(ctx, tx, bookID, chapterSpan{idx: pos.idx}, ch.Content)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, chapterInsertSQL,
		bookID, pos.volumeID, pos.idx, title, words, ref.id, ref.off, ref.len)
	if err != nil {
		return 0, fmt.Errorf("store: insert chapter: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: insert chapter id: %w", err)
	}
	// adjustBookCounts 顺带刷新 books.updated_at：新增章节就是这本书的最后更新。
	if err := adjustBookCounts(ctx, tx, bookID, 1, words); err != nil {
		return 0, err
	}
	if err := bumpMetrics(ctx, tx, packed, int64(words)); err != nil {
		return 0, err
	}
	if err := bumpMetricKey(ctx, tx, metricTextBytesRaw, raw); err != nil {
		return 0, err
	}
	if _, err := bumpBookTextBytes(ctx, tx, bookID); err != nil {
		return 0, err
	}
	return id, refreshBookBlockRanges(ctx, tx, bookID)
}

// refreshBookBlockRanges 校正全书各块的 first_idx/last_idx。
//
// 插入会让落点之后的章节 idx 整体 +1，块上记的章节范围随之失效；这两个字段只用于
// 块损坏时的定位文案，但漂移会误导排查。逐块子查询是 O(块数 × 章数)，插入章节
// 是低频管理操作，值这个代价。
func refreshBookBlockRanges(ctx context.Context, tx *sql.Tx, bookID int64) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE chapter_blocks SET "+
			"first_idx = COALESCE((SELECT MIN(idx) FROM chapters WHERE block_id = chapter_blocks.id), first_idx), "+
			"last_idx = COALESCE((SELECT MAX(idx) FROM chapters WHERE block_id = chapter_blocks.id), last_idx) "+
			"WHERE book_id = ?", bookID)
	if err != nil {
		return fmt.Errorf("store: refresh block ranges of book %d: %w", bookID, err)
	}
	return nil
}
