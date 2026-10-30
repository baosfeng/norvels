package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 我的书架（迁移 0016 user_shelf）：用户主动加入书架的书。
//
// ---- 与书单（collections）的分工 ----
// 书架是「一本书只属于一个书架」的收藏开关（加入 / 移出），书单是「一本书可属多个」
// 的自定义分组。两者的写入频率与展示位置都不同，因此是两张表而不是复用同一张。
//
// ---- 与书库的关系 ----
// 全部书仍然可浏览：书架只是「我挑出来的那几本」，不影响列表与检索。
// 所有读写都走 ScopeUserID(ctx)：漏一处就是串号。

// ShelfItem 是书架列表里的一行：书摘要 + 加入时间。
type ShelfItem struct {
	BookBrief
	AddedAt string `json:"addedAt"`
}

// shelfBookColumns 是书架列表的取数列（别名 b=books、a=authors）。
//
// 与 bookBriefColumns 同一份口径：书架列表与系列列表展示的是同一组字段，
// 两处分别维护列清单迟早会漂。
const shelfBookColumns = bookBriefColumns

// ListShelf 分页返回当前账号的书架，按加入时间倒序（同秒用 book_id 兜底）。
func (s *Store) ListShelf(ctx context.Context, page, size int) ([]ShelfItem, int, error) {
	page, size = normPage(page, size)
	userID := ScopeUserID(ctx)
	total, err := s.countShelf(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+shelfBookColumns+", sh.added_at FROM user_shelf sh "+
			"JOIN books b ON b.id = sh.book_id "+
			"LEFT JOIN authors a ON a.id = b.author_id "+
			"WHERE sh.user_id = ? "+
			"ORDER BY sh.added_at DESC, sh.book_id DESC LIMIT ? OFFSET ?",
		userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list shelf: %w", err)
	}
	defer rows.Close()

	items, err := scanShelfItems(rows, size)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// scanShelfItems 扫描书架列表行（BookBrief + added_at）。
func scanShelfItems(rows *sql.Rows, size int) ([]ShelfItem, error) {
	items := make([]ShelfItem, 0, size)
	for rows.Next() {
		var (
			item    ShelfItem
			updated string
		)
		if err := rows.Scan(&item.ID, &item.Title, &item.AuthorID, &item.AuthorName,
			&item.ChapterCount, &item.WordCount, &item.HasCover, &item.Rating, &item.NewChapters,
			&item.ReadStatus, &item.Series, &item.SeriesIndex, &updated, &item.AddedAt); err != nil {
			return nil, fmt.Errorf("store: scan shelf item: %w", err)
		}
		item.UpdatedAt, _ = parseTime(updated)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list shelf rows: %w", err)
	}
	return items, nil
}

// countShelf 数当前账号书架里的书。
func (s *Store) countShelf(ctx context.Context, userID int64) (int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_shelf WHERE user_id = ?", userID).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count shelf: %w", err)
	}
	return total, nil
}

// ShelfBookIDs 返回当前账号书架里的全部 book_id（按加入时间倒序）。
//
// 一次返回全量而不是分页：前端要拿它给书库列表的每一行打「已加入」标记，
// 一次拉齐比翻页期间反复增量查询更简单，量级也就是「一个人收藏的书」。
func (s *Store) ShelfBookIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT book_id FROM user_shelf WHERE user_id = ? ORDER BY added_at DESC, book_id DESC",
		ScopeUserID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: list shelf ids: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0, 64)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan shelf id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list shelf ids rows: %w", err)
	}
	return ids, nil
}

// AddToShelf 把一本书加入当前账号的书架（幂等：已经在架上不报错、不刷新加入时间）。
//
// 书不存在返回 ErrNotFound（api 映射 404）：给一个不存在的 id 加书架是调用方的错误，
// 静默成功会让前端以为加上了。
func (s *Store) AddToShelf(ctx context.Context, bookID int64) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if err := s.assertBookExists(ctx, bookID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO user_shelf(user_id, book_id, added_at) VALUES (?, ?, ?)",
		ScopeUserID(ctx), bookID, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: add shelf book %d: %w", bookID, err)
	}
	return nil
}

// RemoveFromShelf 把一本书移出当前账号的书架；本来不在架上也算成功（幂等）。
func (s *Store) RemoveFromShelf(ctx context.Context, bookID int64) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM user_shelf WHERE user_id = ? AND book_id = ?",
		ScopeUserID(ctx), bookID); err != nil {
		return fmt.Errorf("store: remove shelf book %d: %w", bookID, err)
	}
	return nil
}
