package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// bookCascadeTables 是删除一本书时必须清空的**主库**子表，按删除顺序排列。
//
// 这些表在 schema 上已声明 ON DELETE CASCADE（open.go 打开 foreign_keys(ON)），
// 这里仍逐条显式删除。理由：删除不可逆，级联语义藏在 schema 里，后续改表的人
// 未必记得还有这条删除路径；显式清单让「删一本书到底动了哪些表」可被审查。
// reading_sessions / collection_books / user_shelf（0014/0016）根本没有 books 外键，
// 只有这张清单会清它们——漏一行就是永远删不掉的孤儿数据。
// chapters 排第一：章节行参与压缩块寻址与检索索引维护，先清干净再动别的。
//
// chapter_blocks 不在这张清单里：它在**正文库**（另一个文件），主库事务删不掉它，
// 由 DeleteBook 在主库提交后单独清理（见该函数的注释）。
var bookCascadeTables = []string{
	"chapters",
	"volumes",
	"book_covers",
	"books_authors",
	"reading_progress",
	"bookmarks",
	"book_tags",
	"reading_sessions",
	"collection_books",
	"user_shelf",
}

// DeleteBook 单事务删除一本书及其**主库**从属数据，提交后再删正文库的块。
//
// 不存在返回 ErrNotFound（api 映射 404）；主库任一步失败整体回滚，不会留下
// 「书没了但章节还在」的半删除状态。已入库的书被删除后，同一 source_path
// 可以重新导入（UNIQUE 约束随之释放）。
//
// 跨库顺序：主库先提交，再删正文库的块（best-effort）。反过来先删块而主库失败，
// 残留的章节会指向不存在的块；现在最坏只是留孤儿块，ReconcileContent 能扫出来清掉。
// 正文库清理失败**不让删除失败**：书已经从书架消失，重试删除只会 404。
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
	// 被这本书取代的版本（0021：审核期间隐藏的新版本 / 已替换掉的旧版本）恢复为当前版本：
	// 取代者已经不在了，superseded_by 指向一个不存在的 id 会让那本书永远从书架消失。
	if err := releaseSupersededBooks(ctx, tx, id); err != nil {
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
	s.purgeBookContent(ctx, id)
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

// releaseSupersededBooks 把「被 id 取代」的书恢复成当前版本（superseded_by = 0）。
//
// 删除是唯一会让 superseded_by 指向悬空 id 的路径：审核记录本身留着（历史），
// 但那本书必须重新可见——否则用户既看不到它，也没有任何界面能把它捞回来。
func releaseSupersededBooks(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, "UPDATE books SET superseded_by = 0 WHERE superseded_by = ?", id); err != nil {
		return fmt.Errorf("store: release superseded books of %d: %w", id, err)
	}
	return nil
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
