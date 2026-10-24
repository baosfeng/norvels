package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ListChapters 分页返回目录（不含正文）。From 从 1 开始，Limit 默认 50、上限 500。
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
func (s *Store) GetChapter(ctx context.Context, bookID, chapterID int64) (*ChapterContent, error) {
	if chapterID <= 0 {
		return nil, fmt.Errorf("%w: chapter id %d", ErrInvalidArgument, chapterID)
	}

	q := "SELECT c.book_id, b.title, c.id, c.idx, c.title, c.content, c.word_count " +
		"FROM chapters c JOIN books b ON b.id = c.book_id WHERE c.id = ?"
	args := []any{chapterID}
	if bookID > 0 {
		q += " AND c.book_id = ?"
		args = append(args, bookID)
	}

	var out ChapterContent
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&out.BookID, &out.BookTitle, &out.ID, &out.Idx,
		&out.Title, &out.Content, &out.WordCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: chapter %d", ErrNotFound, chapterID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get chapter: %w", err)
	}

	prevQ := "SELECT id FROM chapters WHERE book_id = ? AND (idx < ? OR (idx = ? AND id < ?)) " +
		"ORDER BY idx DESC, id DESC LIMIT 1"
	if err := s.db.QueryRowContext(ctx, prevQ, out.BookID, out.Idx, out.Idx, out.ID).Scan(&out.PrevID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: get prev chapter: %w", err)
	}
	nextQ := "SELECT id FROM chapters WHERE book_id = ? AND (idx > ? OR (idx = ? AND id > ?)) " +
		"ORDER BY idx ASC, id ASC LIMIT 1"
	if err := s.db.QueryRowContext(ctx, nextQ, out.BookID, out.Idx, out.Idx, out.ID).Scan(&out.NextID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: get next chapter: %w", err)
	}
	return &out, nil
}
