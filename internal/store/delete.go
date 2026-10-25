package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// bookCascadeTables 是删除一本书时必须清空的子表，按删除顺序排列。
//
// 这些表在 schema 上已声明 ON DELETE CASCADE（open.go 打开 foreign_keys(ON)），
// 这里仍逐条显式删除。理由：删除不可逆，级联语义藏在 schema 里，后续改表的人
// 未必记得还有这条删除路径；显式清单让「删一本书到底动了哪些表」可被审查。
// reading_sessions / collection_books（0014）根本没有 books 外键，只有这张清单会清
// 它们——漏一行就是永远删不掉的孤儿数据。
// chapters 排第一：章节行参与压缩块寻址与检索索引维护，先清干净再动别的。
var bookCascadeTables = []string{
	"chapters",
	"chapter_blocks",
	"volumes",
	"book_covers",
	"books_authors",
	"reading_progress",
	"bookmarks",
	"book_tags",
	"reading_sessions",
	"collection_books",
}

// DeleteBook 单事务删除一本书及其全部从属数据，并刷新作者聚合列。
//
// 不存在返回 ErrNotFound（api 映射 404）；任一步失败整体回滚，不会留下
// 「书没了但章节还在」的半删除状态。已入库的书被删除后，同一 source_path
// 可以重新导入（UNIQUE 约束随之释放）。
func (s *Store) DeleteBook(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, id)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete book begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	// 先确认书存在并记住要重算的作者：books 本体的删除放在最后一步，
	// 这样「书不存在」由最后一步的 RowsAffected 判定，不会与「子表本来就是空的」混淆。
	authorIDs, err := bookAuthorIDsForDelete(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, table := range bookCascadeTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE book_id = ?", id); err != nil {
			return fmt.Errorf("store: delete book %d from %s: %w", id, table, err)
		}
	}
	if err := deleteBookRow(ctx, tx, id); err != nil {
		return err
	}
	for _, authorID := range authorIDs {
		if err := refreshAuthorStats(ctx, tx, authorID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete book commit: %w", err)
	}
	return nil
}

// bookAuthorIDsForDelete 确认书存在，并返回删除后需要重算聚合列的作者 id。
//
// 取主作者（books.author_id）与 books_authors 里的全部关联作者：作者的作品数
// 与总字数按 books.author_id 聚合，但多作者书的主作者可能不是 position 0 之外的
// 那几位，一并重算是为了不让作者页出现「挂着书却数不到」的漂移。
func bookAuthorIDsForDelete(ctx context.Context, tx *sql.Tx, id int64) ([]int64, error) {
	var primary int64
	err := tx.QueryRowContext(ctx, "SELECT author_id FROM books WHERE id = ?", id).Scan(&primary)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: book %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: book author lookup %d: %w", id, err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT author_id FROM books_authors WHERE book_id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("store: list book authors %d: %w", id, err)
	}
	defer rows.Close()

	ids := []int64{primary}
	for rows.Next() {
		var authorID int64
		if err := rows.Scan(&authorID); err != nil {
			return nil, fmt.Errorf("store: scan book author %d: %w", id, err)
		}
		ids = append(ids, authorID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list book authors rows %d: %w", id, err)
	}
	return ids, nil
}

// deleteBookRow 删除 books 本体；行数为 0（书不存在）返回 ErrNotFound。
func deleteBookRow(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM books WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("store: delete book %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete book %d rows: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: book %d", ErrNotFound, id)
	}
	return nil
}
