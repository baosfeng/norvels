package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// authorWorks 是「作者 → 作品」的规范子查询：主作者（books.author_id）与
// 副作者（books_authors）各算一次，UNION 去重后每本书只出现一行。
// 列表、详情、筛选、统计都走它，避免「详情能看到多作者、作者页却查不到」的数据孤儿。
const authorWorks = "(SELECT author_id AS aid, id AS book_id FROM books WHERE author_id <> 0 " +
	"UNION SELECT author_id AS aid, book_id FROM books_authors)"

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

// ListAuthors 分页列出作者，书籍数与字数来自物化聚合表（0022）。
//
// 原先每次请求都跑一遍「authors LEFT JOIN (UNION 子查询) LEFT JOIN books GROUP BY」：
// UNION 两个分支各扫一遍 books / books_authors，再全量聚合后排序（NAS 生产库
// 71,320 本 / 51,611 作者实测 319 ms，9 个驾驶舱请求并发时 1.4 s）。
// 作者的作品数与总字数只随书的增删改变化，物化进 author_agg 后读路径是
// 「索引取前 N 行 + LEFT JOIN 取名字」，与库规模无关。
//
// 缓存缺失（老库没跑 0022 / 回填未完成）时降级为原实时聚合，结果一致（有测试守着），
// 只是慢——接口照常 200，不 500。
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

	items, err := s.listAuthorsPage(ctx, q.Sort, whereSQL, args, size, (page-1)*size)
	if err != nil {
		return Page[AuthorSummary]{}, err
	}
	return Page[AuthorSummary]{Items: items, Total: total, Page: page, Size: size}, nil
}

// listAuthorsPage 取一页作者：优先读物化聚合表，不可用时降级为实时聚合。
func (s *Store) listAuthorsPage(ctx context.Context, sortKey, whereSQL string,
	args []any, size, offset int) ([]AuthorSummary, error) {
	if ok, _ := s.PrecomputeStatus(ctx); ok {
		items, err := s.listAuthorsFromAgg(ctx, sortKey, whereSQL, args, size, offset)
		if err == nil {
			return items, nil
		}
	}
	return s.listAuthorsLive(ctx, sortKey, whereSQL, args, size, offset)
}

// listAuthorsFromAgg 从 author_agg 取一页（作者名按主键回查 authors）。
//
// 用 IN 子查询圈出这一页的 author_id 再 JOIN 名字，而不是把聚合表与 authors
// 直接 JOIN 后 ORDER BY：前者能从 idx_author_agg_words / _books 直接取前 N 行，
// 后者会先把两张表全部对齐再排序（物化就白做了）。
func (s *Store) listAuthorsFromAgg(ctx context.Context, sortKey, whereSQL string,
	args []any, size, offset int) ([]AuthorSummary, error) {
	query := "SELECT a.id, a.name, g.book_count, g.word_count " +
		"FROM authors a JOIN " + tableAuthorAgg + " g ON g.author_id = a.id" + whereSQL +
		" ORDER BY " + authorAggOrder(sortKey) + " LIMIT ? OFFSET ?"
	qargs := append(append([]any{}, args...), size, offset)
	rows, err := s.db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return nil, fmt.Errorf("store: list authors from agg: %w", err)
	}
	defer rows.Close()
	return scanAuthorSummaries(rows, size)
}

// listAuthorsLive 是 0022 之前的实时聚合（降级路径与对账测试共用）。
func (s *Store) listAuthorsLive(ctx context.Context, sortKey, whereSQL string,
	args []any, size, offset int) ([]AuthorSummary, error) {
	query := "SELECT a.id, a.name, COUNT(b.id), COALESCE(SUM(b.word_count), 0) " +
		"FROM authors a LEFT JOIN " + authorWorks + " x ON x.aid = a.id " +
		"LEFT JOIN books b ON b.id = x.book_id" + whereSQL +
		" GROUP BY a.id, a.name ORDER BY " + authorOrder(sortKey) + " LIMIT ? OFFSET ?"
	qargs := append(append([]any{}, args...), size, offset)
	rows, err := s.db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return nil, fmt.Errorf("store: list authors: %w", err)
	}
	defer rows.Close()
	return scanAuthorSummaries(rows, size)
}

// scanAuthorSummaries 把「id, name, 作品数, 字数」四列的行扫描成 AuthorSummary。
func scanAuthorSummaries(rows *sql.Rows, size int) ([]AuthorSummary, error) {
	items := make([]AuthorSummary, 0, size)
	for rows.Next() {
		var a AuthorSummary
		if err := rows.Scan(&a.ID, &a.Name, &a.BookCount, &a.WordCount); err != nil {
			return nil, fmt.Errorf("store: scan author: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list authors rows: %w", err)
	}
	return items, nil
}

// authorAggOrder 把排序键映射为物化表上的 ORDER BY 子句。
//
// 与 authorOrder 逐个对应（同样的兜底列，保证分页稳定），只是列来自 author_agg。
func authorAggOrder(sortKey string) string {
	switch sortKey {
	case "name":
		return "a.name COLLATE NOCASE ASC, a.id ASC"
	case "words":
		return "g.word_count DESC, a.name COLLATE NOCASE ASC, a.id ASC"
	default: // books
		return "g.book_count DESC, a.name COLLATE NOCASE ASC, a.id ASC"
	}
}

// GetAuthor 返回作者详情与其全部作品（按更新时间倒序）。
func (s *Store) GetAuthor(ctx context.Context, id int64) (*AuthorDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: author id %d", ErrInvalidArgument, id)
	}
	var a AuthorSummary
	err := s.db.QueryRowContext(ctx,
		"SELECT a.id, a.name, COUNT(b.id), COALESCE(SUM(b.word_count), 0) "+
			"FROM authors a LEFT JOIN "+authorWorks+" x ON x.aid = a.id "+
			"LEFT JOIN books b ON b.id = x.book_id WHERE a.id = ? GROUP BY a.id, a.name", id).
		Scan(&a.ID, &a.Name, &a.BookCount, &a.WordCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: author %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get author: %w", err)
	}

	books, err := s.loadBooks(ctx, []string{"b.id IN (SELECT book_id FROM " + authorWorks + " WHERE aid = ?)"},
		[]any{id}, bookOrder("updated"), -1, 0)
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
