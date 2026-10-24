package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// authorOrder 把排序键映射为 ORDER BY 子句。
//
// books = 按作品数（默认）；words = 按总字数（驾驶舱「字数 Top 10」）；
// name = 按作者名。三者都追加 name/id 兜底，保证分页稳定。
func authorOrder(sortKey string) string {
	switch sortKey {
	case "name":
		return "a.name COLLATE NOCASE ASC, a.id ASC"
	case "words":
		return "COALESCE(SUM(b.word_count), 0) DESC, a.name COLLATE NOCASE ASC, a.id ASC"
	default: // books
		return "COUNT(b.id) DESC, a.name COLLATE NOCASE ASC, a.id ASC"
	}
}

// ListAuthors 分页列出作者，书籍数与字数为实时聚合。
func (s *Store) ListAuthors(ctx context.Context, q AuthorQuery) (Page[AuthorSummary], error) {
	page, size := normPage(q.Page, q.Size)

	var (
		where []string
		args  []any
	)
	if keyword := strings.TrimSpace(q.Q); keyword != "" {
		where = append(where, "a.name LIKE ? ESCAPE '!'")
		args = append(args, "%"+escapeLike(keyword)+"%")
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM authors a"+whereSQL, args...).Scan(&total); err != nil {
		return Page[AuthorSummary]{}, fmt.Errorf("store: count authors: %w", err)
	}

	query := "SELECT a.id, a.name, COUNT(b.id), COALESCE(SUM(b.word_count), 0) " +
		"FROM authors a LEFT JOIN books b ON b.author_id = a.id" + whereSQL +
		" GROUP BY a.id, a.name ORDER BY " + authorOrder(q.Sort) + " LIMIT ? OFFSET ?"
	qargs := append(append([]any{}, args...), size, (page-1)*size)

	rows, err := s.db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return Page[AuthorSummary]{}, fmt.Errorf("store: list authors: %w", err)
	}
	defer rows.Close()

	items := make([]AuthorSummary, 0, size)
	for rows.Next() {
		var a AuthorSummary
		if err := rows.Scan(&a.ID, &a.Name, &a.BookCount, &a.WordCount); err != nil {
			return Page[AuthorSummary]{}, fmt.Errorf("store: scan author: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return Page[AuthorSummary]{}, fmt.Errorf("store: list authors rows: %w", err)
	}
	return Page[AuthorSummary]{Items: items, Total: total, Page: page, Size: size}, nil
}

// GetAuthor 返回作者详情与其全部作品（按更新时间倒序）。
func (s *Store) GetAuthor(ctx context.Context, id int64) (*AuthorDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: author id %d", ErrInvalidArgument, id)
	}
	var a AuthorSummary
	err := s.db.QueryRowContext(ctx,
		"SELECT a.id, a.name, COUNT(b.id), COALESCE(SUM(b.word_count), 0) "+
			"FROM authors a LEFT JOIN books b ON b.author_id = a.id WHERE a.id = ? GROUP BY a.id, a.name", id).
		Scan(&a.ID, &a.Name, &a.BookCount, &a.WordCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: author %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get author: %w", err)
	}

	books, err := s.loadBooks(ctx, []string{"b.author_id = ?"}, []any{id}, bookOrder("updated"), -1, 0)
	if err != nil {
		return nil, err
	}
	return &AuthorDetail{AuthorSummary: a, Books: books}, nil
}

// ListTags 列出标签及其书籍数（按书籍数倒序）。
func (s *Store) ListTags(ctx context.Context) ([]TagSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT t.id, t.name, COUNT(bt.book_id) FROM tags t "+
			"LEFT JOIN book_tags bt ON bt.tag_id = t.id GROUP BY t.id, t.name "+
			"ORDER BY COUNT(bt.book_id) DESC, t.name ASC")
	if err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	defer rows.Close()

	items := make([]TagSummary, 0, 16)
	for rows.Next() {
		var t TagSummary
		if err := rows.Scan(&t.ID, &t.Name, &t.BookCount); err != nil {
			return nil, fmt.Errorf("store: scan tag: %w", err)
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list tags rows: %w", err)
	}
	return items, nil
}
