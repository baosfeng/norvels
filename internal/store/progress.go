package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ReadingProgress 是一条「最近在读」记录（已联表补齐书名/章名）。
type ReadingProgress struct {
	BookID       int64   `json:"bookId"`
	BookTitle    string  `json:"bookTitle"`
	ChapterID    int64   `json:"chapterId"`
	ChapterIdx   int     `json:"chapterIdx"`
	ChapterTitle string  `json:"chapterTitle"`
	Percent      float64 `json:"percent"`
	UpdatedAt    string  `json:"updatedAt"`
}

// ListReadingProgress 返回最近在读的前 limit 条（按 updated_at 倒序）。
//
// 用 LEFT JOIN：章节已被删除时仍能返回记录（书名保留，章名为空），
// 避免「进度条在但章节没了」的脏数据把整个列表拖成 500。
func (s *Store) ListReadingProgress(ctx context.Context, limit int) ([]ReadingProgress, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > maxProgressLimit {
		limit = maxProgressLimit
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT p.book_id, COALESCE(b.title, ''), p.chapter_id, p.chapter_idx, "+
			"COALESCE(c.title, ''), p.percent, p.updated_at "+
			"FROM reading_progress p "+
			"LEFT JOIN books b ON b.id = p.book_id "+
			"LEFT JOIN chapters c ON c.id = p.chapter_id "+
			"ORDER BY p.updated_at DESC, p.book_id DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("store: list reading progress: %w", err)
	}
	defer rows.Close()

	items := make([]ReadingProgress, 0, limit)
	for rows.Next() {
		var it ReadingProgress
		if err := rows.Scan(&it.BookID, &it.BookTitle, &it.ChapterID, &it.ChapterIdx,
			&it.ChapterTitle, &it.Percent, &it.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan reading progress: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list reading progress rows: %w", err)
	}
	return items, nil
}

// maxProgressLimit 是「最近在读」的上限（驾驶舱只需要 5，留余量）。
const maxProgressLimit = 50

// maxPercent 是进度上限（1 = 读完）。
const maxPercent = 1.0

// SaveReadingProgress 按 book_id upsert 一条进度记录。
//
// chapter_id 必须存在且属于 book_id；percent 落在 [0,1]（越界拒绝，不静默截断）。
func (s *Store) SaveReadingProgress(ctx context.Context, p ReadingProgress) error {
	if p.BookID <= 0 || p.ChapterID <= 0 {
		return fmt.Errorf("%w: book=%d chapter=%d", ErrInvalidArgument, p.BookID, p.ChapterID)
	}
	if p.Percent < 0 || p.Percent > maxPercent {
		return fmt.Errorf("%w: percent %v 必须在 0~1 之间", ErrInvalidArgument, p.Percent)
	}
	if err := s.assertChapterOfBook(ctx, p.BookID, p.ChapterID); err != nil {
		return err
	}
	idx, err := s.chapterIdx(ctx, p.ChapterID)
	if err != nil {
		return err
	}
	return s.saveProgressTx(ctx, p, idx)
}

// saveProgressTx 在一个事务里 upsert 进度，并把「未标记」的书自动转为在读。
func (s *Store) saveProgressTx(ctx context.Context, p ReadingProgress, idx int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save progress begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	if _, err = tx.ExecContext(ctx,
		"INSERT INTO reading_progress(book_id, chapter_id, chapter_idx, percent, updated_at) "+
			"VALUES (?, ?, ?, ?, ?) "+
			"ON CONFLICT(book_id) DO UPDATE SET chapter_id=excluded.chapter_id, "+
			"chapter_idx=excluded.chapter_idx, percent=excluded.percent, updated_at=excluded.updated_at",
		p.BookID, p.ChapterID, idx, p.Percent, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: save reading progress: %w", err)
	}
	// 阅读状态的唯一自动转移：'' → reading，与进度同事务落地。
	if err := markReadingOnProgressStep(ctx, tx, p.BookID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save reading progress commit: %w", err)
	}
	return nil
}

// assertChapterOfBook 校验章节存在且属于该书；不存在映射 ErrNotFound。
func (s *Store) assertChapterOfBook(ctx context.Context, bookID, chapterID int64) error {
	var one int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM chapters WHERE id = ? AND book_id = ?", chapterID, bookID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: chapter %d of book %d", ErrNotFound, chapterID, bookID)
	}
	if err != nil {
		return fmt.Errorf("store: check chapter: %w", err)
	}
	return nil
}

// chapterIdx 读取章节序号（用于冗余写入 chapter_idx）。
func (s *Store) chapterIdx(ctx context.Context, chapterID int64) (int, error) {
	var idx int
	if err := s.db.QueryRowContext(ctx, "SELECT idx FROM chapters WHERE id = ?", chapterID).Scan(&idx); err != nil {
		return 0, fmt.Errorf("store: read chapter idx: %w", err)
	}
	return idx, nil
}

// normalizePercentParam 供 api 层复用：把 JSON 里的可选字段夹到合法范围前先判非法。
func NormalizePercent(v float64) (float64, bool) {
	if v < 0 || v > maxPercent {
		return 0, false
	}
	return v, true
}

var _ = strings.TrimSpace
