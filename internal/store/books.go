package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// unknownAuthor 是作者缺失时的统一占位名。
const unknownAuthor = "佚名"

// bookSummaryColumns 是 BookSummary 的通用取数列（别名 b=books、a=authors）。
//
// 末段 8 列是 0008 加的书籍身份（language…source_format）与 has_cover：
// 列表与详情共用同一份口径，避免两个 SELECT 漂移。
const bookSummaryColumns = "b.id, b.title, b.author_id, COALESCE(a.name, ''), " +
	"b.chapter_count, b.word_count, b.volume_count, b.intro, b.status, b.serial_status, " +
	"b.read_status, b.imported_at, b.updated_at, " +
	"b.language, b.publisher, b.isbn, b.pub_date, b.series, b.series_index, b.has_cover, b.source_format, " +
	"b.rating, b.new_chapters, b.category_l1, b.category_l2"

// rowScanner 抽象 *sql.Row / *sql.Rows 的 Scan。
type rowScanner interface {
	Scan(dest ...any) error
}

// summaryScanTargets 按 bookSummaryColumns 的顺序返回扫描目标。
//
// 抽成函数是因为详情读取要在同一份列后面追加自己的列：两处共用一套目标，
// 以后加字段只改一处（19 个字段手抄两遍，迟早漏一个）。
func summaryScanTargets(bs *BookSummary, imported, updated *string) []any {
	return []any{
		&bs.ID, &bs.Title, &bs.AuthorID, &bs.AuthorName, &bs.ChapterCount, &bs.WordCount,
		&bs.VolumeCount, &bs.Intro, &bs.Status, &bs.SerialStatus, &bs.ReadStatus, imported, updated,
		&bs.Language, &bs.Publisher, &bs.ISBN, &bs.PubDate, &bs.Series, &bs.SeriesIndex,
		&bs.HasCover, &bs.Format, &bs.Rating, &bs.NewChapters,
		&bs.CategoryL1, &bs.CategoryL2,
	}
}

// scanBookSummary 把一行扫描成 BookSummary（时间列为文本，解析失败时留零值）。
func scanBookSummary(sc rowScanner) (BookSummary, error) {
	var (
		bs       BookSummary
		imported string
		updated  string
	)
	if err := sc.Scan(summaryScanTargets(&bs, &imported, &updated)...); err != nil {
		return bs, err
	}
	applySummaryTimes(&bs, imported, updated)
	return bs, nil
}

// applySummaryTimes 回填两条时间列（解析失败保持零值，不让一条脏数据废掉整页）。
func applySummaryTimes(bs *BookSummary, imported, updated string) {
	bs.ImportedAt, _ = parseTime(imported)
	bs.UpdatedAt, _ = parseTime(updated)
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
	case "rating":
		return "b.rating DESC, b.updated_at DESC, b.id DESC"
	default: // updated
		return "b.updated_at DESC, b.id DESC"
	}
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

// ensureBookExistsInTx 是写子表（book_covers/book_tags）前的存在性守卫：
// 子表外键只保证「book_id 有效」，调用方需要的是「这本书在不在」这个明确答案。
func ensureBookExistsInTx(ctx context.Context, tx *sql.Tx, bookID int64) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM books WHERE id = ?)", bookID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check book %d: %w", bookID, err)
	}
	if !exists {
		return fmt.Errorf("%w: book %d", ErrNotFound, bookID)
	}
	return nil
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

// ---------- 追更角标（0014 起） ----------

// AckNewChapters 清零一本书的追更角标（读者已经看过最新章节）。
//
// 不刷新 updated_at：确认角标不是内容变更，不该让这本书在「最近更新」里跳位
// （左侧栏与列表都按 updated_at 排序）。
func (s *Store) AckNewChapters(ctx context.Context, bookID int64) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	res, err := s.db.ExecContext(ctx, "UPDATE books SET new_chapters = 0 WHERE id = ?", bookID)
	if err != nil {
		return fmt.Errorf("store: ack new chapters %d: %w", bookID, err)
	}
	return requireOneRow(res, bookID)
}

// AddNewChapters 手工累加追更章数（追更主链路已在 updateBookAfterAppend 的事务内自增，
// 这里留给测试与运维脚本手工修正角标）。
//
// n 为负一律拒绝：累加本身是 +，负值只会来自调用方把「删掉了几章」误当追加，
// 而 new_chapters 是角标不是账本，写进负数没有任何可解释的含义。
func (s *Store) AddNewChapters(ctx context.Context, bookID int64, n int) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if n < 0 {
		return fmt.Errorf("%w: 追更章数 %d 不能为负", ErrInvalidArgument, n)
	}
	if n == 0 {
		return nil // 没有追加：不碰库，也不要求书存在（追更未命中时上游会跳过调用）
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE books SET new_chapters = new_chapters + ? WHERE id = ?", n, bookID)
	if err != nil {
		return fmt.Errorf("store: add new chapters %d: %w", bookID, err)
	}
	return requireOneRow(res, bookID)
}
