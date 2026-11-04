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

// writeChapterBlocks 按块压缩写入**正文库**，返回逐章的块寻址与压缩/原始字节总量。
//
// 全空块不建块行：block_id 保持 0，读取侧见到 0 直接返回空正文，
// 不必为一个空块占一行 + 一次解压。
func writeChapterBlocks(ctx context.Context, c *contentTx, bookID int64,
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
		id, err := c.insertBlock(ctx, bookID, startSeq+seq+1,
			chapters[first].Idx, chapters[last].Idx, raw, packed)
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

// chapterWordCounts 返回各章的字数（与 chapters.word_count 同一口径）。
//
// 单独返回而不是让调用方再算一遍：长度直方图的桶必须与写入库里的 word_count
// 逐章一致，重算两遍迟早因为口径漂移对不上。
func chapterWordCounts(chapters []model.ParsedChapter) []int {
	out := make([]int, len(chapters))
	for i := range chapters {
		out[i] = chapterWordCount(&chapters[i])
	}
	return out
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
// 写入分两步：先按「章节边界 + 8 MB 目标」把正文切成块并压进**正文库**的
// chapter_blocks，再写主库的 chapters 行（只存块寻址与标题）。两步分属两个事务，
// 提交顺序与失败补偿由 writeThrough 保证。
//
// 导入路径本来就要遍历所有章节，在这里累加几乎零额外成本——比事后跑
// SUM(length(packed))（要读全部压缩块）便宜得多。累加与章节写入在同一主库事务内
// 完成，二者不会漂移。
//
// 口径：text_bytes 统计**压缩后**的 packed 字节（整数列相加），text_bytes_raw
// 统计原文逻辑字节。两个口径都在这里累计，方便文档说明压缩比。
func insertChapters(ctx context.Context, wt *writeTx, bookID int64, b *model.ParsedBook, volMap map[int]int64) error {
	sizes := make([]int, len(b.Chapters))
	for i := range b.Chapters {
		sizes[i] = len(b.Chapters[i].Content)
	}

	// seq 从本书现有块的 MAX 续写：覆盖写入（--force）时旧块要到主库提交后才回收，
	// 此刻它们仍占着 1..N 的序号，从 0 重排会撞 UNIQUE(book_id, seq)。
	startSeq, err := wt.content.nextSeq(ctx, bookID)
	if err != nil {
		return err
	}
	refs, packedBytes, rawBytes, err := writeChapterBlocks(ctx, wt.content, bookID, b.Chapters, sizes, startSeq)
	if err != nil {
		return err
	}
	words, err := insertChapterRows(ctx, wt.main, bookID, b.Chapters, volMap, refs)
	if err != nil {
		return err
	}

	// 导入了 len(b.Chapters) 章：字数与章数都要进缓存，长度直方图也在这里落账
	// （导入本来就要写这些行，顺手累加是零额外扫描）。
	if err := bumpMetricsCount(ctx, wt.main, packedBytes, words, int64(len(b.Chapters))); err != nil {
		return err
	}
	if err := bumpMetricKey(ctx, wt.main, metricTextBytesRaw, rawBytes); err != nil {
		return err
	}
	if err := bumpLengthHist(ctx, wt.main, chapterWordCounts(b.Chapters), nil); err != nil {
		return err
	}
	// books 侧文本量（标题/简介/路径等）随书籍主体变化，一并增量维护，
	// 保证「增量累加」与「全量重算」在同一个口径上（有测试守着）。
	if _, err := bumpBookTextBytes(ctx, wt.main, bookID); err != nil {
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
	err = s.writeThrough(ctx, "insert chapter", func(wt *writeTx) error {
		pos, err := insertPositionInTx(ctx, wt.main, bookID, afterChapterID)
		if err != nil {
			return err
		}
		newID, err = insertChapterRowInTx(ctx, wt, bookID, pos, title, &ch)
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

// insertChapterRowInTx 写正文块（正文库）与章节行（主库），并同步三处计数。
func insertChapterRowInTx(ctx context.Context, wt *writeTx, bookID int64, pos insertPosition,
	title string, ch *model.ParsedChapter) (int64, error) {
	tx := wt.main
	// 先把落点之后的章节整体后移，空出 idx：chapters 没有 UNIQUE(book_id, idx)，
	// 一条 UPDATE 平移即可（与删除/拆分同一套 shiftChaptersIdx）。
	if err := shiftChaptersIdx(ctx, tx, bookID, pos.idx-1, 1); err != nil {
		return 0, err
	}
	words := chapterWordCount(ch)
	ref, packed, raw, err := writeContentBlock(ctx, wt.content, bookID, chapterSpan{idx: pos.idx}, ch.Content)
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
	if err := bumpMetricsCount(ctx, tx, packed, int64(words), 1); err != nil {
		return 0, err
	}
	if err := bumpMetricKey(ctx, tx, metricTextBytesRaw, raw); err != nil {
		return 0, err
	}
	if err := bumpLengthHist(ctx, tx, []int{words}, nil); err != nil {
		return 0, err
	}
	if _, err := bumpBookTextBytes(ctx, tx, bookID); err != nil {
		return 0, err
	}
	return id, refreshBookBlockRanges(ctx, wt, bookID)
}

// refreshBookBlockRanges 校正全书各块的 first_idx/last_idx。
//
// 插入会让落点之后的章节 idx 整体 +1，块上记的章节范围随之失效；这两个字段只用于
// 块损坏时的定位文案，但漂移会误导排查。逐块读章节范围（主库 GROUP BY）再回写正文库，
// 插入章节是低频管理操作，值这个代价。
func refreshBookBlockRanges(ctx context.Context, wt *writeTx, bookID int64) error {
	rows, err := wt.main.QueryContext(ctx,
		"SELECT block_id, MIN(idx), MAX(idx) FROM chapters WHERE book_id = ? AND block_id > 0 "+
			"GROUP BY block_id", bookID)
	if err != nil {
		return fmt.Errorf("store: read block ranges of book %d: %w", bookID, err)
	}
	type blockRange struct {
		id       int64
		firstIdx int
		lastIdx  int
	}
	ranges := make([]blockRange, 0, 16)
	for rows.Next() {
		var r blockRange
		if err := rows.Scan(&r.id, &r.firstIdx, &r.lastIdx); err != nil {
			rows.Close() //nolint:errcheck // 读取失败后关闭，错误已返回
			return fmt.Errorf("store: scan block range: %w", err)
		}
		ranges = append(ranges, r)
	}
	err = rows.Err()
	rows.Close() //nolint:errcheck // 同上
	if err != nil {
		return fmt.Errorf("store: read block ranges rows: %w", err)
	}
	for _, r := range ranges {
		if _, err := wt.content.tx.ExecContext(ctx,
			"UPDATE chapter_blocks SET first_idx = ?, last_idx = ? WHERE id = ?",
			r.firstIdx, r.lastIdx, r.id); err != nil {
			return fmt.Errorf("store: refresh block range %d: %w", r.id, err)
		}
	}
	return nil
}
