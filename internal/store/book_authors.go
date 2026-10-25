package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"norvels/internal/model"
)

// 本文件是「一书多作者」的唯一写点与读点。
//
// 两条不变量：
//   - books.author_id 永远等于 books_authors 里 position 最小的那个作者；
//   - 存量书可以没有 books_authors 行，读取侧按 books.author_id 兜底主作者。
//
// 为什么要两处存：列表/筛选/聚合（/api/authors 的作品数）都按 books.author_id
// 单值走，那是既有口径且索引齐全；books_authors 只服务「这本书还有哪些作者」的展示。

// NormalizeAuthors 归一化作者列表：去首尾空白、丢空名、保序去重。
//
// 导出给 api 层复用：跨信任边界的校验（条数上限、单名长度）在 api 做，
// 「什么算同一个作者」这件事必须只有一份实现，否则 PATCH 与导入会写出两种口径。
func NormalizeAuthors(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		n := strings.TrimSpace(name)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// bookAuthorNames 取本次写入要落库的有序作者名。
//
// Authors 为空时视作 []string{Author}（spec §2.2）；Author 也空则落「佚名」，
// 与单作者导入同一口径，避免 author_id=0 的书从作者筛选里消失。
func bookAuthorNames(b *model.ParsedBook) []string {
	names := NormalizeAuthors(b.Authors)
	if len(names) == 0 {
		names = []string{bookAuthor(b)}
	}
	return names
}

// saveBookAuthorsInTx 全量替换一本书的作者关联行，并把 books.author_id 同步成主作者。
// 返回主作者的 author id（供作者聚合列重算用）。
func saveBookAuthorsInTx(ctx context.Context, tx *sql.Tx, bookID int64, names []string) (int64, error) {
	names = NormalizeAuthors(names)
	if len(names) == 0 {
		// 空名单 = 显式清空作者（P1-B §2.5）：删关联行并把主作者置 0，
		// 读取侧据此返回空 authors、authorName 空串；旧作者的聚合列由调用方重算。
		return clearBookAuthorsInTx(ctx, tx, bookID)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM books_authors WHERE book_id = ?", bookID); err != nil {
		return 0, fmt.Errorf("store: clear book authors %d: %w", bookID, err)
	}
	primaryID := int64(0)
	for pos, name := range names {
		authorID, err := ensureAuthor(ctx, tx, name)
		if err != nil {
			return 0, err
		}
		if authorID == 0 {
			continue
		}
		if pos == 0 {
			primaryID = authorID
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO books_authors(book_id, author_id, position) VALUES (?, ?, ?)",
			bookID, authorID, pos); err != nil {
			return 0, fmt.Errorf("store: save book author %d/%q: %w", bookID, name, err)
		}
	}
	if primaryID == 0 {
		return 0, fmt.Errorf("%w: book %d has no usable author", ErrInvalidArgument, bookID)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE books SET author_id = ? WHERE id = ?", primaryID, bookID); err != nil {
		return 0, fmt.Errorf("store: sync book primary author %d: %w", bookID, err)
	}
	return primaryID, nil
}

// clearBookAuthorsInTx 清空一本书的作者：删关联行并把 books.author_id 置 0。
func clearBookAuthorsInTx(ctx context.Context, tx *sql.Tx, bookID int64) (int64, error) {
	if _, err := tx.ExecContext(ctx, "DELETE FROM books_authors WHERE book_id = ?", bookID); err != nil {
		return 0, fmt.Errorf("store: clear book authors %d: %w", bookID, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE books SET author_id = 0 WHERE id = ?", bookID); err != nil {
		return 0, fmt.Errorf("store: clear primary author %d: %w", bookID, err)
	}
	return 0, nil
}

// loadBookAuthors 读有序作者名；没有 books_authors 行（存量书）时按主作者兜底。
func (s *Store) loadBookAuthors(ctx context.Context, bookID int64, primary string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT COALESCE(a.name, '') FROM books_authors ba "+
			"JOIN authors a ON a.id = ba.author_id WHERE ba.book_id = ? ORDER BY ba.position, ba.author_id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: load book authors %d: %w", bookID, err)
	}
	defer rows.Close()

	names := make([]string, 0, 2)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan book author: %w", err)
		}
		if n := strings.TrimSpace(name); n != "" {
			names = append(names, n)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: load book authors rows: %w", err)
	}
	if len(names) > 0 {
		return names, nil
	}
	if strings.TrimSpace(primary) == "" {
		// 作者已被显式清空（author_id=0）：返回空数组，不再兜底成「佚名」。
		return []string{}, nil
	}
	return []string{fallbackAuthorName(primary)}, nil
}

// fallbackAuthorName 是兜底口径：主作者名为空时落「佚名」，与导入路径一致。
func fallbackAuthorName(primary string) string {
	if n := strings.TrimSpace(primary); n != "" {
		return n
	}
	return unknownAuthor
}

// FindByTitleAuthor 按「书名 + 主作者名」找书（追更的第三级匹配、去重校验用）。
//
// 主作者名取 books.author_id → authors.name（不是 books_authors 的 position 0），
// 因此对没有 books_authors 行的存量书同样成立。
func (s *Store) FindByTitleAuthor(ctx context.Context, title, author string) (int64, bool, error) {
	id, err := findBookIDByTitleAuthorTx(ctx, s.db, title, author)
	if err != nil {
		return 0, false, err
	}
	return id, id != 0, nil
}

// queryer 抽象 *sql.DB / *sql.Tx，让「按书名作者找书」在事务内外共用一份 SQL。
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// findBookIDByTitleAuthorTx 是 FindByTitleAuthor 的实现体；未命中返回 (0, nil)。
func findBookIDByTitleAuthorTx(ctx context.Context, q queryer, title, author string) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, fmt.Errorf("%w: empty title", ErrInvalidArgument)
	}
	var id int64
	err := q.QueryRowContext(ctx,
		"SELECT b.id FROM books b LEFT JOIN authors a ON a.id = b.author_id "+
			"WHERE b.title = ? AND COALESCE(a.name, '') = ? ORDER BY b.id LIMIT 1",
		title, fallbackAuthorName(author)).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("store: find book by title/author: %w", err)
	}
	return id, nil
}
