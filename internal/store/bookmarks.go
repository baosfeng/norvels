package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Bookmark 是阅读器里的一条书签（段落级定位）。
//
// 用 (chapter_id, para_index) 而不是字符偏移：正文重排、重新分段后字符偏移会漂，
// 段落下标在段落结构不变的前提下仍然指向同一段。
type Bookmark struct {
	ID           int64  `json:"id"`
	ChapterID    int64  `json:"chapterId"`
	ChapterIdx   int    `json:"chapterIdx"`
	ChapterTitle string `json:"chapterTitle"`
	ParaIndex    int    `json:"paraIndex"`
	Label        string `json:"label"`
	CreatedAt    string `json:"createdAt"`
}

// 书签入参的跨边界上限。paraIndex 是段落下标，label 是用户可编辑的摘要。
const (
	maxBookmarkLabelRunes = 200
	maxParaIndex          = 1_000_000
)

// assertBookExists 在事务外确认书存在；书不存在返回 ErrNotFound（api 映射 404）。
func (s *Store) assertBookExists(ctx context.Context, bookID int64) error {
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM books WHERE id = ?)", bookID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check book %d: %w", bookID, err)
	}
	if !exists {
		return fmt.Errorf("%w: book %d", ErrNotFound, bookID)
	}
	return nil
}

// ListBookmarks 返回一本书的全部书签（按 created_at 倒序，同秒用 id 兜底）。
//
// 书不存在返回 ErrNotFound（api 映射 404）：空书签列表与「书都没了」是两件事。
// 章节被删时仍返回书签行（章名退化为空串），不让一条脏数据拖垮整页。
func (s *Store) ListBookmarks(ctx context.Context, bookID int64) ([]Bookmark, error) {
	if bookID <= 0 {
		return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if err := s.assertBookExists(ctx, bookID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT bm.id, bm.chapter_id, bm.chapter_idx, COALESCE(c.title, ''), "+
			"bm.para_index, bm.label, bm.created_at "+
			"FROM bookmarks bm LEFT JOIN chapters c ON c.id = bm.chapter_id "+
			"WHERE bm.book_id = ? ORDER BY bm.created_at DESC, bm.id DESC", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list bookmarks: %w", err)
	}
	defer rows.Close()

	items := make([]Bookmark, 0, 16)
	for rows.Next() {
		var bm Bookmark
		if err := rows.Scan(&bm.ID, &bm.ChapterID, &bm.ChapterIdx, &bm.ChapterTitle,
			&bm.ParaIndex, &bm.Label, &bm.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan bookmark: %w", err)
		}
		items = append(items, bm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list bookmarks rows: %w", err)
	}
	return items, nil
}

// CountBookmarks 返回一本书的书签数（详情页 bookmarkCount）。
func (s *Store) CountBookmarks(ctx context.Context, bookID int64) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM bookmarks WHERE book_id = ?", bookID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count bookmarks: %w", err)
	}
	return n, nil
}

// AddBookmark 幂等新增书签：同一 (chapterId, paraIndex) 已存在时只更新 label。
//
// 返回 (书签, created, error)：created=true 表示新建（api 映射 201），
// false 表示命中已有条目并更新了 label（api 映射 200）——第二次 POST 总数仍为 1。
func (s *Store) AddBookmark(ctx context.Context, bookID, chapterID int64, paraIndex int, label string) (Bookmark, bool, error) {
	if err := checkBookmarkArgs(bookID, chapterID, paraIndex, label); err != nil {
		return Bookmark{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Bookmark{}, false, fmt.Errorf("store: add bookmark begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
		return Bookmark{}, false, err
	}
	chapter, err := bookmarkChapterInTx(ctx, tx, bookID, chapterID)
	if err != nil {
		return Bookmark{}, false, err
	}
	bm := Bookmark{ChapterID: chapterID, ChapterIdx: chapter.idx, ChapterTitle: chapter.title,
		ParaIndex: paraIndex, Label: strings.TrimSpace(label)}
	if bm.Label == "" {
		bm.Label = defaultBookmarkLabel(chapter.title, paraIndex)
	}
	created, err := upsertBookmarkInTx(ctx, tx, bookID, &bm)
	if err != nil {
		return Bookmark{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Bookmark{}, false, fmt.Errorf("store: add bookmark commit: %w", err)
	}
	return bm, created, nil
}

// checkBookmarkArgs 校验书签入参：id 为正、段落下标非负且不越界、label 不超长。
func checkBookmarkArgs(bookID, chapterID int64, paraIndex int, label string) error {
	if bookID <= 0 || chapterID <= 0 {
		return fmt.Errorf("%w: book=%d chapter=%d", ErrInvalidArgument, bookID, chapterID)
	}
	if paraIndex < 0 || paraIndex > maxParaIndex {
		return fmt.Errorf("%w: paraIndex %d 必须在 0~%d 之间", ErrInvalidArgument, paraIndex, maxParaIndex)
	}
	if utf8.RuneCountInString(strings.TrimSpace(label)) > maxBookmarkLabelRunes {
		return fmt.Errorf("%w: label 不能超过 %d 字", ErrInvalidArgument, maxBookmarkLabelRunes)
	}
	return nil
}

// bookmarkChapter 是章节归属查询的结果。
type bookmarkChapter struct {
	idx   int
	title string
}

// bookmarkChapterInTx 校验章节存在且属于该书；不属于时按入参非法拒绝（400）。
//
// 这里不用 ErrNotFound：书存在、章节 id 也给了，错的是「它不在这本书里」，
// 属于调用方传错参数，契约 §2.2 明确要求 400。
func bookmarkChapterInTx(ctx context.Context, tx *sql.Tx, bookID, chapterID int64) (bookmarkChapter, error) {
	var c bookmarkChapter
	err := tx.QueryRowContext(ctx,
		"SELECT idx, title FROM chapters WHERE id = ? AND book_id = ?", chapterID, bookID).
		Scan(&c.idx, &c.title)
	if errors.Is(err, sql.ErrNoRows) {
		return c, fmt.Errorf("%w: chapter %d 不属于书 %d", ErrInvalidArgument, chapterID, bookID)
	}
	if err != nil {
		return c, fmt.Errorf("store: check bookmark chapter: %w", err)
	}
	return c, nil
}

// upsertBookmarkInTx 插入或更新书签，返回 created（true=新建）。
func upsertBookmarkInTx(ctx context.Context, tx *sql.Tx, bookID int64, bm *Bookmark) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM bookmarks WHERE book_id = ? AND chapter_id = ? AND para_index = ?",
		bookID, bm.ChapterID, bm.ParaIndex).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return insertBookmarkInTx(ctx, tx, bookID, bm)
	case err != nil:
		return false, fmt.Errorf("store: find bookmark: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE bookmarks SET label = ? WHERE id = ?", bm.Label, id); err != nil {
		return false, fmt.Errorf("store: update bookmark %d: %w", id, err)
	}
	return false, readBookmarkFieldsInTx(ctx, tx, id, bookID, bm)
}

// insertBookmarkInTx 插入一条书签并把 id/created_at 回填到 bm。
func insertBookmarkInTx(ctx context.Context, tx *sql.Tx, bookID int64, bm *Bookmark) (bool, error) {
	bm.CreatedAt = formatTime(nowUTC())
	res, err := tx.ExecContext(ctx,
		"INSERT INTO bookmarks(book_id, chapter_id, chapter_idx, para_index, label, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		bookID, bm.ChapterID, bm.ChapterIdx, bm.ParaIndex, bm.Label, bm.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("store: insert bookmark: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return false, fmt.Errorf("store: bookmark id: %w", err)
	}
	bm.ID = id
	return true, nil
}

// readBookmarkFieldsInTx 读回已有书签的 id 与 created_at（更新 label 后补齐响应字段）。
func readBookmarkFieldsInTx(ctx context.Context, tx *sql.Tx, id, bookID int64, bm *Bookmark) error {
	if err := tx.QueryRowContext(ctx,
		"SELECT id, created_at FROM bookmarks WHERE id = ? AND book_id = ?", id, bookID).
		Scan(&bm.ID, &bm.CreatedAt); err != nil {
		return fmt.Errorf("store: read bookmark %d: %w", id, err)
	}
	return nil
}

// DeleteBookmark 删除一条书签；不存在返回 ErrNotFound（api 映射 404）。
func (s *Store) DeleteBookmark(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: bookmark id %d", ErrInvalidArgument, id)
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM bookmarks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("store: delete bookmark %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete bookmark %d rows: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: bookmark %d", ErrNotFound, id)
	}
	return nil
}

// defaultBookmarkLabel 在调用方没给 label 时取「章名 · 第 n 段」作占位。
//
// label 默认值由前端按段首 30 字生成，后端只兜底（直接调 API 的客户端也不该出现空摘要）。
func defaultBookmarkLabel(chapterTitle string, paraIndex int) string {
	base := strings.TrimSpace(chapterTitle)
	if base == "" {
		return fmt.Sprintf("第 %d 段", paraIndex+1)
	}
	return fmt.Sprintf("%s · 第 %d 段", base, paraIndex+1)
}
