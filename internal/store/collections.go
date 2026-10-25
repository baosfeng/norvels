package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// 书单（P3 §自定义书单）：扁平收藏夹，一本书可属多个书单，不做嵌套与排序。
//
// 两张表（0014）：collections.name 全库唯一；collection_books 对 collections 有
// ON DELETE CASCADE、对 books 没有外键——删书由 delete.go 的级联清单显式清理
// （契约 §迁移 0014）。名字比较口径全库只有一份：去首尾空白后精确匹配。

// maxCollectionNameRunes 是书单名上限：展示字段，超长只会在书单列表里撑破布局。
const maxCollectionNameRunes = 80

// maxCollectionBooks 是一次全量替换的书数上限：请求体大小与写放大的边界，
// 远超「一个书单能装多少本」的实际使用场景。
const maxCollectionBooks = 2000

// Collection 是一个书单及其实时的书籍数。
type Collection struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
}

// ListCollections 列出全部书单（按名字升序）并带书目数。
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT c.id, c.name, "+
			"(SELECT COUNT(*) FROM collection_books cb WHERE cb.collection_id = c.id) "+
			"FROM collections c ORDER BY c.name ASC, c.id ASC")
	if err != nil {
		return nil, fmt.Errorf("store: list collections: %w", err)
	}
	defer rows.Close()

	items := make([]Collection, 0, 8)
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.BookCount); err != nil {
			return nil, fmt.Errorf("store: scan collection: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list collections rows: %w", err)
	}
	return items, nil
}

// CreateCollection 新建书单；重名返回 ErrConflict（409，用户该换个名字）。
func (s *Store) CreateCollection(ctx context.Context, name string) (Collection, error) {
	clean, err := cleanCollectionName(name)
	if err != nil {
		return Collection{}, err
	}
	c := Collection{Name: clean}
	err = s.inTx(ctx, "create collection", func(tx *sql.Tx) error {
		if err := ensureCollectionNameFree(ctx, tx, clean, 0); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO collections(name, created_at) VALUES (?, ?)", clean, formatTime(nowUTC()))
		if err != nil {
			return wrapCollectionNameError(err, clean)
		}
		if c.ID, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("store: create collection id: %w", err)
		}
		return nil
	})
	if err != nil {
		return Collection{}, err
	}
	return c, nil
}

// RenameCollection 改名；重名 ErrConflict，书单不存在 ErrNotFound。
func (s *Store) RenameCollection(ctx context.Context, id int64, name string) error {
	if id <= 0 {
		return fmt.Errorf("%w: 书单 id %d", ErrInvalidArgument, id)
	}
	clean, err := cleanCollectionName(name)
	if err != nil {
		return err
	}
	return s.inTx(ctx, "rename collection", func(tx *sql.Tx) error {
		if err := collectionExistsInTx(ctx, tx, id); err != nil {
			return err
		}
		if err := ensureCollectionNameFree(ctx, tx, clean, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE collections SET name = ? WHERE id = ?", clean, id); err != nil {
			return wrapCollectionNameError(err, clean)
		}
		return nil
	})
}

// DeleteCollection 删除书单；书目关联随 ON DELETE CASCADE 一起清（书本身不动）。
func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: 书单 id %d", ErrInvalidArgument, id)
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM collections WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("store: delete collection %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete collection %d rows: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: 书单 %d", ErrNotFound, id)
	}
	return nil
}

// ReplaceCollectionBooks 全量替换书单里的书（空列表 = 清空）。
//
// 书单不存在 ErrNotFound；书 id 不存在/非法是请求错误（ErrInvalidArgument），
// 与「书单没了」区分开——前者该修请求，后者该刷新列表。
func (s *Store) ReplaceCollectionBooks(ctx context.Context, id int64, bookIDs []int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: 书单 id %d", ErrInvalidArgument, id)
	}
	ids, err := normalizeCollectionBookIDs(bookIDs)
	if err != nil {
		return err
	}
	return s.inTx(ctx, "replace collection books", func(tx *sql.Tx) error {
		if err := collectionExistsInTx(ctx, tx, id); err != nil {
			return err
		}
		if err := requireBooksExistInTx(ctx, tx, ids); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM collection_books WHERE collection_id = ?", id); err != nil {
			return fmt.Errorf("store: clear collection %d books: %w", id, err)
		}
		now := formatTime(nowUTC())
		for _, bookID := range ids {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO collection_books(collection_id, book_id, added_at) VALUES (?, ?, ?)",
				id, bookID, now); err != nil {
				return fmt.Errorf("store: add book %d to collection %d: %w", bookID, id, err)
			}
		}
		return nil
	})
}

// cleanCollectionName 归一化并校验书单名：去首尾空白后非空、不超上限。
func cleanCollectionName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", fmt.Errorf("%w: 书单名不能为空", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(clean) > maxCollectionNameRunes {
		return "", fmt.Errorf("%w: 书单名超过 %d 字", ErrInvalidArgument, maxCollectionNameRunes)
	}
	return clean, nil
}

// collectionExistsInTx 确认书单存在；不存在返回 ErrNotFound。
func collectionExistsInTx(ctx context.Context, tx *sql.Tx, id int64) error {
	var got int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM collections WHERE id = ?", id).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: 书单 %d", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("store: check collection %d: %w", id, err)
	}
	return nil
}

// ensureCollectionNameFree 事务内查重；excludeID 用于改名时跳过自己。
func ensureCollectionNameFree(ctx context.Context, tx *sql.Tx, name string, excludeID int64) error {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM collections WHERE name = ?", name).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("store: check collection name %q: %w", name, err)
	}
	if id != excludeID {
		return fmt.Errorf("%w: 书单名 %q 已存在", ErrConflict, name)
	}
	return nil
}

// wrapCollectionNameError 把「撞 UNIQUE」的写错误翻成 ErrConflict（并发兜底），
// 其余错误原样包装保留底层链。
func wrapCollectionNameError(err error, name string) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed: collections.name") {
		return fmt.Errorf("%w: 书单名 %q 已存在", ErrConflict, name)
	}
	return fmt.Errorf("store: write collection %q: %w", name, err)
}

// normalizeCollectionBookIDs 校验书 id 列表：非正数拒绝、保序去重、有上限。
func normalizeCollectionBookIDs(bookIDs []int64) ([]int64, error) {
	if len(bookIDs) > maxCollectionBooks {
		return nil, fmt.Errorf("%w: 一个书单最多 %d 本", ErrInvalidArgument, maxCollectionBooks)
	}
	seen := make(map[int64]struct{}, len(bookIDs))
	out := make([]int64, 0, len(bookIDs))
	for _, id := range bookIDs {
		if id <= 0 {
			return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, id)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// requireBooksExistInTx 逐本确认存在；缺任何一本都返回 ErrInvalidArgument。
//
// 不用 ErrNotFound：请求里只要有一本书不存在，整个 PUT 就是一份非法载荷，
// 调用方应修请求体，而不是以为「书单被删了」。
func requireBooksExistInTx(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for _, id := range ids {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM books WHERE id = ?)", id).Scan(&exists); err != nil {
			return fmt.Errorf("store: check book %d: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("%w: book %d 不存在", ErrInvalidArgument, id)
		}
	}
	return nil
}
