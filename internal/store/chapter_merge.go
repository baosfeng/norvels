package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// 合并相邻章节（管理页 M1，与 chapter_split.go 的拆分互逆）。
//
// 合并**一律新建一块**装拼接后的正文，不再走「同块连续区间直接拉长 span」的
// 零拷贝快路径：合并要在相邻两章正文之间插一个换行（见 mergeSeparator），
// 插入的字节让正文不等于原来的任何一个连续区间，span 直拼物理上做不到。
// 两条路径只留一条，合并结果就不会随「拆完再合并」还是「跨块合并」而变。
//
// 代价是每次合并都解压重压一遍（单章几百 KB 量级，毫秒级）。旧块若因此无人引用，
// 由 reapBlocksOf 整块回收并从 metrics 里冲减，体积不会虚增。

// mergeSeparator 是合并时插在相邻两章正文之间的分隔符：**恰好一个换行**。
//
// 为什么必须插：合并是把两段**独立的正文**接成一章。原文各自的章尾与章首之间本来就
// 有一处「章界」——章尾常常没有句末标点（「…远处有人拉起了手风琴」，
// 下一章起句「夜雾沿着河堤…」）。直拼会让读者把两句连读成一句，等于合并把断句吃掉了。
// 一个换行是最小代价的段落断开：不引入新字符（不改字、不改标点），又是所有阅读器
// 都认的段落边界。
//
// 尾部不额外加：分隔符只出现在两段之间，合并结果不会因为合并本身以换行结尾。
const mergeSeparator = "\n"

// MergeChapters 把 cids 指定的相邻同卷章节合并成一章，保留**首章 id**。
//
// 首章保留 id 的理由：阅读进度（reading_progress）与书签都按 chapter id 指过来，
// 保住目录里的第一章就是保住读者的位置。标题取首章原标题（不重编号——那由
// RenumberChapters 负责），正文按章序用 mergeSeparator 拼接。
// 三处计数在同一事务里按增量维护，books.updated_at 一并刷新（详情页与列表都按它排序）。
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

// applyMerge 落实合并：拼正文写新块 → 改写首章 → 删其余章 → 重排序号与计数 → 回收旧块。
func applyMerge(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan) error {
	text, err := mergeChapterText(ctx, tx, spans)
	if err != nil {
		return err
	}
	ref, packed, raw, err := writeMergeBlock(ctx, tx, bookID, spans, text)
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

// mergeChapterText 按章序取出各章正文，相邻两段之间插 mergeSeparator。
//
// 空章（block_id=0 且长度 0）既不贡献字节也不贡献分隔符：否则合并结果会多出
// 空行或以换行开头（空章夹在中间时），而「插入的分隔符数量 = 有正文的段数 − 1」
// 才是不变量的真实表述。
func mergeChapterText(ctx context.Context, tx *sql.Tx, spans []chapterSpan) ([]byte, error) {
	parts := make([][]byte, 0, len(spans))
	total := 0
	for _, sp := range spans {
		raw, err := chapterPartText(ctx, tx, sp)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		parts = append(parts, raw)
		total += len(raw) + len(mergeSeparator)
	}
	if len(parts) == 0 {
		return nil, nil
	}
	buf := make([]byte, 0, total)
	for i, p := range parts {
		if i > 0 {
			buf = append(buf, mergeSeparator...)
		}
		buf = append(buf, p...)
	}
	return buf, nil
}

// chapterPartText 取出一章的正文；空章返回空串而不是报错。
//
// 合并时把空章并进来是合法操作，但「有长度却没有块」仍是块级损坏，
// 必须照常报错而不是静默丢字。
func chapterPartText(ctx context.Context, tx *sql.Tx, sp chapterSpan) ([]byte, error) {
	if sp.blockID <= 0 {
		if sp.length != 0 {
			return nil, fmt.Errorf("%w: 第 %d 章有长度 %d 却没有正文块",
				ErrBlockCorrupted, sp.idx+1, sp.length)
		}
		return nil, nil
	}
	return spanText(ctx, tx, sp)
}

// writeMergeBlock 把合并后的正文压成一个新块，返回首章的新 span 与字节增量。
//
// 全空合并不建块（与写入侧「全空块不建块行」同一口径）：首章变成空章，block_id=0。
//
// 拼出来的正文可能远超 1 MB 分块目标（几章各占一块、合起来 1.3 MB）：块不设硬上限，
// 一章独占一块即可（与 splitBlocks「单章超过目标就独占」同一口径）。压缩或写块失败
// 直接返回错误，整个合并事务回滚——章节行、块行与 metrics 都停在合并前，
// 不会出现「首章已被改写、正文只写了一半」的半章状态。
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

// insertMergeBlock 写入合并后的新块（块覆盖首章到末章的序号范围），返回块 id。
//
// 写块本身与整章替换共用 insertTextBlock（chapter_content.go）：chapter_blocks 的列
// 只有一套口径，合并不会因为「自己的 SQL」而在 raw_bytes / seq 上漂移。
func insertMergeBlock(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan,
	raw, packed []byte) (int64, error) {
	return insertTextBlock(ctx, tx, bookID, spans[0].idx, spans[len(spans)-1].idx, raw, packed)
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
// 为什么不重算：插进来的 mergeSeparator 是空白，store.countWords 不把空白算作字数
// （CJK 逐字计、空白与标点都不计，见 util.go），所以对中文正文重算也不会变；
// 但英文词被换行切开会被算成两个词。存量口径才是读者看到的那一列，也不受分隔符
// 影响，因此合并一律沿用各章存量之和，不跑 countWords。
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
