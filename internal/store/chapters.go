package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ListChapters 分页返回目录（不含正文）。From 从 1 开始，Limit 默认 50、上限 500。
//
// 目录查询**只读 chapters 的整数与短文本列**，绝不碰 chapter_blocks.packed：
// 否则「翻目录」会变成「解压全书正文」，5000 章的书会瞬间卡住。
func (s *Store) ListChapters(ctx context.Context, q ChapterQuery) (Page[ChapterBrief], error) {
	if q.BookID <= 0 {
		return Page[ChapterBrief]{}, fmt.Errorf("%w: book id %d", ErrInvalidArgument, q.BookID)
	}
	from := q.From
	if from < 1 {
		from = 1
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := (from - 1) * limit

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chapters WHERE book_id = ?", q.BookID).Scan(&total); err != nil {
		return Page[ChapterBrief]{}, fmt.Errorf("store: count chapters: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, idx, title, volume_id, word_count FROM chapters WHERE book_id = ? "+
			"ORDER BY idx, id LIMIT ? OFFSET ?", q.BookID, limit, offset)
	if err != nil {
		return Page[ChapterBrief]{}, fmt.Errorf("store: list chapters: %w", err)
	}
	defer rows.Close()

	items := make([]ChapterBrief, 0, limit)
	for rows.Next() {
		var c ChapterBrief
		if err := rows.Scan(&c.ID, &c.Idx, &c.Title, &c.VolumeID, &c.WordCount); err != nil {
			return Page[ChapterBrief]{}, fmt.Errorf("store: scan chapter: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return Page[ChapterBrief]{}, fmt.Errorf("store: list chapters rows: %w", err)
	}
	return Page[ChapterBrief]{Items: items, Total: total, Page: from, Size: limit}, nil
}

// GetChapter 返回正文及前后章 id（0 表示到头）。bookID 为 0 时只按章节 id 查。
//
// 正文来自 chapter_blocks：先读块的 block_id/off/len，再把该块解压后切片。
// 解压是**按块独立完成**的，不依赖其他章、也不需要跨章状态。
func (s *Store) GetChapter(ctx context.Context, bookID, chapterID int64) (*ChapterContent, error) {
	if chapterID <= 0 {
		return nil, fmt.Errorf("%w: chapter id %d", ErrInvalidArgument, chapterID)
	}

	q := "SELECT c.book_id, b.title, c.id, c.idx, c.title, c.word_count, " +
		"c.block_id, c.block_off, c.block_len " +
		"FROM chapters c JOIN books b ON b.id = c.book_id WHERE c.id = ?"
	args := []any{chapterID}
	if bookID > 0 {
		q += " AND c.book_id = ?"
		args = append(args, bookID)
	}

	var (
		out                ChapterContent
		blockID            int64
		blockOff, blockLen int
	)
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&out.BookID, &out.BookTitle, &out.ID, &out.Idx,
		&out.Title, &out.WordCount, &blockID, &blockOff, &blockLen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: chapter %d", ErrNotFound, chapterID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get chapter: %w", err)
	}

	if err := s.fillChapterContent(ctx, &out, blockID, blockOff, blockLen); err != nil {
		return nil, err
	}
	if err := s.fillChapterNav(ctx, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// fillChapterContent 解出单章正文填入 out.Content。
//
// block_id = 0 表示这一章是空章（写入侧不为全空块建块行），直接留空串。
func (s *Store) fillChapterContent(ctx context.Context, out *ChapterContent, blockID int64, off, length int) error {
	if blockID <= 0 {
		if length != 0 {
			return fmt.Errorf("%w: chapter %d has length %d but no block", ErrCorruptContent, out.ID, length)
		}
		return nil
	}
	block, err := loadBlock(ctx, s.db, blockID, out.ID)
	if err != nil {
		return err
	}
	text, err := chapterAt(block, off, length)
	if err != nil {
		// 区间越界属于块级损坏：补上块的定位信息，别只报章节号。
		return s.wrapBlockSpanError(ctx, blockID, out.ID, err)
	}
	out.Content = text
	return nil
}

// wrapBlockSpanError 把「块内区间越界」包成带 book_id / seq 的 *BlockError。
//
// 块已经解压成功、但章节记录的 off/len 指到块外面，说明 **元数据与块不一致**，
// 同样是块级故障（可能是迁移中断或手工改库），定位信息一样要有。
func (s *Store) wrapBlockSpanError(ctx context.Context, blockID, chapterID int64, cause error) error {
	meta, err := readBlockRowMetaOnly(ctx, s.db, blockID)
	if err != nil {
		return fmt.Errorf("store: chapter %d: %w", chapterID, cause)
	}
	return &BlockError{
		BlockID: meta.id, BookID: meta.bookID, Seq: meta.seq,
		FirstIdx: meta.firstIdx, LastIdx: meta.lastIdx,
		ChapterID: chapterID, Cause: cause,
	}
}

// fillChapterNav 填充前后章 id（到头的方向保持 0）。
func (s *Store) fillChapterNav(ctx context.Context, out *ChapterContent) error {
	prevQ := "SELECT id FROM chapters WHERE book_id = ? AND (idx < ? OR (idx = ? AND id < ?)) " +
		"ORDER BY idx DESC, id DESC LIMIT 1"
	if err := s.db.QueryRowContext(ctx, prevQ, out.BookID, out.Idx, out.Idx, out.ID).Scan(&out.PrevID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: get prev chapter: %w", err)
	}
	nextQ := "SELECT id FROM chapters WHERE book_id = ? AND (idx > ? OR (idx = ? AND id > ?)) " +
		"ORDER BY idx ASC, id ASC LIMIT 1"
	if err := s.db.QueryRowContext(ctx, nextQ, out.BookID, out.Idx, out.Idx, out.ID).Scan(&out.NextID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: get next chapter: %w", err)
	}
	return nil
}
