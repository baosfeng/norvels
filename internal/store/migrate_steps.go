package store

import (
	"context"
	"database/sql"
	"fmt"
)

// dropSegColumnsStep 去掉 chapters 的 title_seg / content_seg 两列（幂等）。
//
// SQLite 没有「列存在才删」的条件 DDL，只能先探测再决定执行哪条 ALTER；
// 因此这段放在 Go 里、跑在 0006 的事务内。
//
// 两列都是 NOT NULL DEFAULT 空串，且从 0006 起没有任何写入方（分词镜像随正文检索一起下线），
// 所以 ALTER TABLE DROP COLUMN 的原地重建不会丢失业务数据。
func dropSegColumnsStep(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "chapters")
	if err != nil || !exists {
		return err
	}
	for _, col := range []string{"title_seg", "content_seg"} {
		has, err := columnExistsInTx(ctx, tx, "chapters", col)
		if err != nil {
			return err
		}
		if !has {
			continue
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE chapters DROP COLUMN "+col); err != nil {
			return fmt.Errorf("drop column %s: %w", col, err)
		}
	}
	return nil
}

// addBookMetadataColumnsStep 给 books 补齐 0008 的元数据列（幂等）。
//
// 与 0006 同理：SQLite 没有「列存在才加」的条件 DDL，只能先探测再决定执行哪条
// ALTER。版本回退重跑、或开发期已手工加过列的库，都必须能安全通过。
func addBookMetadataColumnsStep(ctx context.Context, tx *sql.Tx) error {
	cols := []struct{ name, ddl string }{
		{"language", "language TEXT NOT NULL DEFAULT ''"},
		{"publisher", "publisher TEXT NOT NULL DEFAULT ''"},
		{"isbn", "isbn TEXT NOT NULL DEFAULT ''"},
		{"pub_date", "pub_date TEXT NOT NULL DEFAULT ''"},
		{"series", "series TEXT NOT NULL DEFAULT ''"},
		{"series_index", "series_index INTEGER NOT NULL DEFAULT 0"},
		{"source_format", "source_format TEXT NOT NULL DEFAULT 'txt'"},
		{"has_cover", "has_cover INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range cols {
		has, err := columnExistsInTx(ctx, tx, "books", c.name)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE books ADD COLUMN "+c.ddl); err != nil {
			return fmt.Errorf("add column books.%s: %w", c.name, err)
		}
	}
	return nil
}

// tableExistsInTx 在事务内判断表/视图是否存在（迁移步骤专用，不改动连接级状态）。
func tableExistsInTx(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name = ?", name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check table %s in tx: %w", name, err)
	}
	return n > 0, nil
}

// columnExistsInTx 在事务内判断某表是否有某列（走 pragma_table_info 表函数）。
func columnExistsInTx(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check column %s.%s in tx: %w", table, column, err)
	}
	return n > 0, nil
}
