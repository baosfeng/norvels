package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// 章节正文编辑（管理页 M1）：整章替换正文。
//
// 为什么整章替换而不是 diff：正文按 span 存（block_id/block_off/block_len），块是
// zstd 压缩后的不可变字节串——块内任何一处改动都要重压整块。做「段落级 / 字符级
// diff」需要先解压、再在块内挪字节并重写**同块所有章节**的 block_off，等于给每条
// 写路径都加一套偏移表重算；整章替换把复杂度压到零：压一个新块、改一个 span。
//
// 为什么要回收旧块：章节改指向新块之后旧块没有任何引用，不回收它就永远留在
// chapter_blocks 里——读者把正文改小，/api/stats 的正文体积却只涨不降。回收口径
// 与合并 / 删除同一套（reapOrphanBlock：无人引用才整块删并冲减 text_bytes/_raw）。
//
// 为什么限额 1 MiB：块以 BlockTargetBytes（1 MB）为目标，一章超过这个量就会独占
// 一块且超出目标；同时正文来自 HTTP 请求体，跨信任边界的输入必须有上限，否则一次
// PUT 就能把库撑大（与封面 2 MiB 上限同一取舍）。
const maxChapterContentBytes = 1 << 20

// UpdateChapterContent 整章替换正文；章 id、idx、卷归属都不动。
//
// 保留章 id / idx 的理由与合并一致：阅读进度（reading_progress）与书签按 chapter id
// 指过来，卷归属与目录顺序是读者的位置信息，正文换了位置不能跟着换。
//
// 单事务：写新块 → 改 span 与 word_count → 同步三处计数 → 回收旧块，全成或全不成。
// 不做版本号乐观锁（本地单用户，后写覆盖先写是设计允许的），但绝不接受
// 「正文换了、计数没动」这类部分成功。
func (s *Store) UpdateChapterContent(ctx context.Context, bookID, chapterID int64, content string) error {
	if err := checkIDs(bookID, chapterID); err != nil {
		return err
	}
	if err := checkChapterContent(content); err != nil {
		return err
	}
	return s.inTx(ctx, "update chapter content", func(tx *sql.Tx) error {
		sp, err := loadChapterSpan(ctx, tx, bookID, chapterID)
		if err != nil {
			return err
		}
		words := countWords(content)
		ref, packed, raw, err := writeContentBlock(ctx, tx, bookID, sp, content)
		if err != nil {
			return err
		}
		if err := rewriteChapterSpan(ctx, tx, bookID, chapterID, ref, words); err != nil {
			return err
		}
		if err := applyContentCounts(ctx, tx, bookID, words-sp.wordCount, packed, raw); err != nil {
			return err
		}
		// 顺序不能反：章节还指着旧块时它当然「有人引用」，先改 span 再回收。
		if err := reapOrphanBlock(ctx, tx, sp.blockID); err != nil {
			return err
		}
		return refreshBlockIdxRange(ctx, tx, ref.id)
	})
}

// checkChapterContent 校验新正文：去空白后为空、超过 1 MiB 一律拒绝（fail-closed）。
//
// 按字节（UTF-8 编码后）而不是按字计数：限额的真实约束是「块大小」与「请求体大小」，
// 两者都以字节计；按字算会让一万一字的生僻字正文悄悄突破块的 1 MB 目标。
func checkChapterContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("%w: 章节正文不能为空（去空白后为空）", ErrInvalidArgument)
	}
	if len(content) > maxChapterContentBytes {
		return fmt.Errorf("%w: 章节正文 %d 字节，超过上限 %d 字节（1 MiB，UTF-8 编码后）",
			ErrInvalidArgument, len(content), maxChapterContentBytes)
	}
	return nil
}

// writeContentBlock 把新正文压成一块写入，返回新 span 与压缩 / 原始字节数。
//
// 一章独占一块（不与其他章合装）：这是整章替换在寻址上省下的麻烦——块内偏移恒为 0，
// 同块其他章节的 block_off 一个都不用动。块因此可能小于 BlockTargetBytes，可接受：
// 块是压缩单元不是填充单元，多出来的块只在极少数「编辑过的章」上出现。
func writeContentBlock(ctx context.Context, tx *sql.Tx, bookID int64, sp chapterSpan,
	content string) (blockRef, int64, int64, error) {
	raw := []byte(content)
	packed, err := compressContent(raw)
	if err != nil {
		return blockRef{}, 0, 0, fmt.Errorf("store: chapter content compress: %w", err)
	}
	id, err := insertTextBlock(ctx, tx, bookID, sp.idx, sp.idx, raw, packed)
	if err != nil {
		return blockRef{}, 0, 0, err
	}
	return blockRef{id: id, off: 0, len: len(raw)}, int64(len(packed)), int64(len(raw)), nil
}

// insertTextBlock 写入一块正文，返回块 id；firstIdx/lastIdx 是它覆盖的章节序号范围。
//
// 合并（chapter_merge.go）与整章替换共用这一处：chapter_blocks 的列（seq 取 MAX+1、
// raw_bytes 记原文逻辑长度）只在这里写一次，两条路径不会各有一套理解。
func insertTextBlock(ctx context.Context, tx *sql.Tx, bookID int64, firstIdx, lastIdx int,
	raw, packed []byte) (int64, error) {
	seq, err := nextBlockSeq(ctx, tx, bookID)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO chapter_blocks(book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, seq, firstIdx, lastIdx, len(raw), packed)
	if err != nil {
		return 0, fmt.Errorf("store: insert block: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: insert block id: %w", err)
	}
	return id, nil
}

// rewriteChapterSpan 把这一章指向新块并写入新字数（idx / volume_id / title 不碰）。
func rewriteChapterSpan(ctx context.Context, tx *sql.Tx, bookID, chapterID int64,
	ref blockRef, words int) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE chapters SET block_id = ?, block_off = ?, block_len = ?, word_count = ? "+
			"WHERE id = ? AND book_id = ?",
		ref.id, ref.off, ref.len, words, chapterID, bookID)
	if err != nil {
		return fmt.Errorf("store: rewrite chapter %d span: %w", chapterID, err)
	}
	return requireOneRow(res, bookID)
}

// applyContentCounts 把正文替换带来的增量同步到三处计数。
//
// 字数按「新算 − 存量」的增量维护，不重算全书（与删除 / 合并同一口径）：一次编辑
// 不该顺手改掉别的章的解析器历史值。adjustBookCounts 顺带刷新 books.updated_at，
// 详情页与列表按它排序，正文改过就该排到前面。
func applyContentCounts(ctx context.Context, tx *sql.Tx, bookID int64, wordDelta int, packed, raw int64) error {
	if err := adjustBookCounts(ctx, tx, bookID, 0, wordDelta); err != nil {
		return err
	}
	if err := bumpMetrics(ctx, tx, packed, int64(wordDelta)); err != nil {
		return err
	}
	return bumpMetricKey(ctx, tx, metricTextBytesRaw, raw)
}
