package store

import (
	"context"
	"fmt"
	"strings"
)

// unknownAuthor 是作者缺失时的统一占位名。
const unknownAuthor = "佚名"

// bookSummaryColumns 是 BookSummary 的通用取数列（别名 b=books、a=authors）。
const bookSummaryColumns = "b.id, b.title, b.author_id, COALESCE(a.name, ''), " +
	"b.chapter_count, b.word_count, b.volume_count, b.intro, b.status, b.imported_at, b.updated_at"

// rowScanner 抽象 *sql.Row / *sql.Rows 的 Scan。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanBookSummary 把一行扫描成 BookSummary（时间列为文本，解析失败时留零值）。
func scanBookSummary(sc rowScanner) (BookSummary, error) {
	var (
		bs       BookSummary
		imported string
		updated  string
	)
	err := sc.Scan(&bs.ID, &bs.Title, &bs.AuthorID, &bs.AuthorName, &bs.ChapterCount, &bs.WordCount,
		&bs.VolumeCount, &bs.Intro, &bs.Status, &imported, &updated)
	if err != nil {
		return bs, err
	}
	bs.ImportedAt, _ = parseTime(imported)
	bs.UpdatedAt, _ = parseTime(updated)
	return bs, nil
}

// BookExistsBySHA256 判断文件哈希是否已入库。
func (s *Store) BookExistsBySHA256(ctx context.Context, sha string) (bool, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha == "" {
		return false, nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM books WHERE file_sha256 = ?)", sha).Scan(&exists); err != nil {
		return false, fmt.Errorf("store: exists by sha: %w", err)
	}
	return exists, nil
}

// bookOrder 把排序键映射为 ORDER BY 子句。
func bookOrder(sortKey string) string {
	switch sortKey {
	case "title":
		return "b.title COLLATE NOCASE ASC, b.id ASC"
	case "chapters":
		return "b.chapter_count DESC, b.id DESC"
	case "words":
		return "b.word_count DESC, b.id DESC"
	case "imported":
		return "b.imported_at DESC, b.id DESC"
	default: // updated
		return "b.updated_at DESC, b.id DESC"
	}
}

// bookFilter 把 BookQuery 编译成 WHERE 片段与参数。
func bookFilter(q BookQuery) ([]string, []any) {
	var (
		where []string
		args  []any
	)
	if s := strings.TrimSpace(q.Q); s != "" {
		pat := "%" + escapeLike(s) + "%"
		where = append(where, "(b.title LIKE ? ESCAPE '!' OR a.name LIKE ? ESCAPE '!')")
		args = append(args, pat, pat)
	}
	if q.AuthorID != 0 {
		where = append(where, "b.author_id = ?")
		args = append(args, q.AuthorID)
	}
	if t := strings.TrimSpace(q.Tag); t != "" {
		where = append(where, "EXISTS (SELECT 1 FROM book_tags bt JOIN tags t ON t.id = bt.tag_id "+
			"WHERE bt.book_id = b.id AND t.name = ?)")
		args = append(args, t)
	}
	return where, args
}

func (s *Store) countBooks(ctx context.Context, where []string, args []any) (int, error) {
	q := "SELECT COUNT(*) FROM books b LEFT JOIN authors a ON a.id = b.author_id"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count books: %w", err)
	}
	return total, nil
}

// loadBooks 执行列表查询；limit <= 0 表示不分页。
func (s *Store) loadBooks(ctx context.Context, where []string, args []any, order string, limit, offset int) ([]BookSummary, error) {
	q := "SELECT " + bookSummaryColumns + " FROM books b LEFT JOIN authors a ON a.id = b.author_id"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY " + order
	qargs := make([]any, 0, len(args)+2)
	qargs = append(qargs, args...)
	if limit > 0 {
		q += " LIMIT ? OFFSET ?"
		if offset < 0 {
			offset = 0
		}
		qargs = append(qargs, limit, offset)
	}
	rows, err := s.db.QueryContext(ctx, q, qargs...)
	if err != nil {
		return nil, fmt.Errorf("store: list books: %w", err)
	}
	defer rows.Close()

	items := make([]BookSummary, 0, 20)
	for rows.Next() {
		bs, err := scanBookSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan book: %w", err)
		}
		items = append(items, bs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list books rows: %w", err)
	}
	return items, nil
}

// ListBooks 分页列出书籍。
func (s *Store) ListBooks(ctx context.Context, q BookQuery) (Page[BookSummary], error) {
	page, size := normPage(q.Page, q.Size)
	where, args := bookFilter(q)
	total, err := s.countBooks(ctx, where, args)
	if err != nil {
		return Page[BookSummary]{}, err
	}
	items, err := s.loadBooks(ctx, where, args, bookOrder(q.Sort), size, (page-1)*size)
	if err != nil {
		return Page[BookSummary]{}, err
	}
	return Page[BookSummary]{Items: items, Total: total, Page: page, Size: size}, nil
}

// truncateRunes 按字符数截断（用于简介等展示字段）。
func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
