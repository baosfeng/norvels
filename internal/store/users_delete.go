package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// 删除账号（0016 的延伸）：删账号必须带走**它的**个人数据，且只带走它的。
//
// ---- 删什么、不删什么 ----
// 删：该账号在个人数据表里的行（书架 / 阅读进度 / 阅读会话 / 书签 / 书单及其关联）
// 以及它的会话行（否则旧 Cookie 还能换出一个已不存在的账号）。
// 不删：书库（books / authors / chapters / tags / 封面 / 正文块）——书是全库共享的，
// 不归任何账号；也不动**别的账号**在同一本书上的记录。
//
// ---- 为什么禁止删默认账号 ----
// 默认账号 id=1（reader）是存量数据归属与开放模式兜底（见 migrate_users.go）：
// 删掉它，未登录请求回落的 ScopeUserID 会指向一个不存在的账号，升级前的老数据
// 也会变成孤儿。所以这不是「用户能不能删」的问题，而是「删了库就不自洽」。
//
// ---- 为什么要事务 ----
// 六张表的删除要么全成、要么全不成：中途失败留下「账号没了但书签还在」的孤儿行，
// 下次同 id 被 AUTOINCREMENT 复用时就会串到新账号名下。

// DeleteUser 删除一个账号及其全部个人数据；默认账号返回 ErrDefaultUser。
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: 账号 id %d", ErrInvalidArgument, id)
	}
	if id == defaultUserID {
		return fmt.Errorf("%w: 默认账号 %q（id=%d）", ErrDefaultUser, defaultUserName, id)
	}
	return s.inTx(ctx, "delete user", func(tx *sql.Tx) error {
		if err := ensureUserExists(ctx, tx, id); err != nil {
			return err
		}
		if err := deleteUserRows(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id); err != nil {
			return fmt.Errorf("store: delete user %d: %w", id, err)
		}
		return nil
	})
}

// ensureUserExists 事务内确认账号还在（并发删除时给出 ErrNotFound 而不是静默成功）。
func ensureUserExists(ctx context.Context, tx *sql.Tx, id int64) error {
	var got int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE id = ?", id).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: 账号 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("store: read user %d: %w", id, err)
	}
	return nil
}

// userDataTables 是该账号个人数据所在的表，按「先子后父」的顺序删：
// collection_books 指向 collections，顺序反了会留下孤儿关联行。
var userDataTables = []string{
	"collection_books",
	"collections",
	"bookmarks",
	"reading_progress",
	"reading_sessions",
	"user_shelf",
	"user_reader_settings",
	// 会话放最后：它是「这个账号还活着」的凭据，最后再撤。
	"user_sessions",
}

// deleteUserRows 删掉该账号在各个人数据表里的行。
func deleteUserRows(ctx context.Context, tx *sql.Tx, id int64) error {
	for _, table := range userDataTables {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM "+table+" WHERE user_id = ?", id); err != nil {
			return fmt.Errorf("store: delete %s of user %d: %w", table, id, err)
		}
	}
	return nil
}
