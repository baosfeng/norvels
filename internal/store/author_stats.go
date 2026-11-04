package store

import (
	"context"
	"database/sql"
	"fmt"
)

// refreshBookAuthors 刷新「这本书涉及到的全部作者」的聚合列。
//
// 为什么不是「只刷新新旧主作者」：聚合口径是 authorWorks（主作者 ∪ 副作者），
// 一本书可能有 3 个作者而主作者从没变过——只刷主作者，副作者的 book_count 就永远
// 停在它们第一次被写进去时的值。api 层有测试盯着这条（TestPatchBookAuthors）。
//
// extra 用来带进「已经不在本书上、但刚才还在」的作者 id（作者被换掉的那个）：
// 它们已经查不到了，只能由调用方传进来。
//
// 两个写点一起刷：authors 表的两列是既有接口契约（列表里要能直接看到），
// author_agg 是 0022 起 /api/authors 的实际读路径。漏掉前者会让旧字段漂，
// 漏掉后者会让驾驶舱的作者 Top 榜停在导入前的状态。
func refreshBookAuthors(ctx context.Context, tx *sql.Tx, bookID int64, extra ...int64) error {
	ids, err := authorIDsOfBook(ctx, tx, bookID)
	if err != nil {
		return err
	}
	for _, id := range extra {
		ids = append(ids, id)
	}
	return refreshAuthorIDs(ctx, tx, ids)
}

// authorIDsOfBook 读出一本书当前关联的全部作者 id（主作者 + 副作者）。
func authorIDsOfBook(ctx context.Context, tx *sql.Tx, bookID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT author_id FROM books WHERE id = ? AND author_id <> 0 "+
			"UNION SELECT author_id FROM books_authors WHERE book_id = ?", bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list authors of book %d: %w", bookID, err)
	}
	defer rows.Close()

	ids := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan author of book %d: %w", bookID, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list authors of book rows %d: %w", bookID, err)
	}
	return ids, nil
}

// refreshAuthorIDs 去重后逐个刷新作者的两个聚合写点。
func refreshAuthorIDs(ctx context.Context, tx *sql.Tx, ids []int64) error {
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if err := refreshAuthorStats(ctx, tx, id); err != nil {
			return err
		}
		if err := refreshAuthorAgg(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// ensureAuthor 按名字取作者 id，不存在则创建；空名返回 0（未知作者）。
func ensureAuthor(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	if name == "" {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO authors(name) VALUES (?) ON CONFLICT(name) DO NOTHING", name); err != nil {
		return 0, fmt.Errorf("store: ensure author: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM authors WHERE name = ?", name).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: select author: %w", err)
	}
	return id, nil
}

// refreshAuthorStats 重算单个作者的聚合列（books 表只有千级行，成本可忽略）。
func refreshAuthorStats(ctx context.Context, tx *sql.Tx, authorID int64) error {
	if authorID == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		"UPDATE authors SET "+
			"book_count = (SELECT COUNT(*) FROM "+authorWorks+" WHERE aid = authors.id), "+
			"word_count = (SELECT COALESCE(SUM(b.word_count), 0) FROM "+authorWorks+" x "+
			"JOIN books b ON b.id = x.book_id WHERE x.aid = authors.id) "+
			"WHERE id = ?", authorID)
	if err != nil {
		return fmt.Errorf("store: refresh author stats: %w", err)
	}
	return nil
}
