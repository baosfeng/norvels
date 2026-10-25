package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 批量删除章节（管理页 M1）。
//
// 与单章删除（DeleteChapter，见 chapter_edit.go）同一套口径：三处计数同事务维护、
// 无人引用的正文块整块回收、删完序号重排。区别只在「一次删多章」——逐章调用会把
// 块回收与序号重排重复 n 次，同一个事务里做一次就够。

// DeleteChapters 删除同一本书的若干章，并把该书的章节序号重排成 0 起连续。
//
// 先把**全部** cid 校验完再动手：中途发现某个 cid 不属于这本书时整体回滚，
// 不留「前几章删了、后面的没删」的半成品目录（回滚能兜住，但没必要先制造它）。
func (s *Store) DeleteChapters(ctx context.Context, bookID int64, cids []int64) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	ids, err := normalizeChapterIDs(cids)
	if err != nil {
		return err
	}
	return s.inTx(ctx, "delete chapters", func(tx *sql.Tx) error {
		spans, err := loadChapterSpans(ctx, tx, bookID, ids)
		if err != nil {
			return err
		}
		if err := deleteChapterRows(ctx, tx, bookID, ids); err != nil {
			return err
		}
		if err := renumberChapterIdx(ctx, tx, bookID); err != nil {
			return err
		}
		if err := applyDeleteCounts(ctx, tx, bookID, spans); err != nil {
			return err
		}
		return reapBlocksOf(ctx, tx, spans)
	})
}

// applyDeleteCounts 把删掉的章节数与字数从书级计数与库级聚合里一起减掉。
//
// 口径用各章**存量** word_count（与单章删除一致）：读者在目录里看到的就是这一列，
// 删除带来的减少必须与它相减为零；重算会把「解析器口径」的历史值悄悄改掉，
// 变成一次删除顺带修正了别的章。
func applyDeleteCounts(ctx context.Context, tx *sql.Tx, bookID int64, spans []chapterSpan) error {
	words := 0
	for _, sp := range spans {
		words += sp.wordCount
	}
	if err := adjustBookCounts(ctx, tx, bookID, -len(spans), -words); err != nil {
		return err
	}
	return bumpMetrics(ctx, tx, 0, -int64(words))
}
