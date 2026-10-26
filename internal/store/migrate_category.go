package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// unclassifiedCategory 是存量书回填的默认分类名。
//
// 为什么不在代码里写真实分类名：仓库的隐私红线不允许出现真实书目/分类词
// （真实值登记在本机 .privacy-denylist.txt）。部署方用 NORVELS_LEGACY_CATEGORY
// 指定自己的回填分类名；不设时落到这个中性默认值。
const unclassifiedCategory = "未分类"

// legacyCategoryL1 返回存量书回填用的分类名（环境变量优先）。
func legacyCategoryL1() string {
	if v := strings.TrimSpace(os.Getenv("NORVELS_LEGACY_CATEGORY")); v != "" {
		return v
	}
	return unclassifiedCategory
}

// addCategoryColumnsStep 给 books 补 0015 的两列分类（幂等）。
//
// 与 0008/0010/0011/0014 同一条路径：SQLite 没有 ADD COLUMN IF NOT EXISTS，
// 先探测列是否存在再 ALTER。存量回填只在「本次真的补上了 category_l1」时执行：
// 迁移重放（删版本号重跑）时列已存在，不会把所有空分类的书再刷一遍。
func addCategoryColumnsStep(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "books")
	if err != nil || !exists {
		return err
	}
	hadColumn, err := columnExistsInTx(ctx, tx, "books", "category_l1")
	if err != nil {
		return err
	}
	cols := []struct{ table, name, ddl string }{
		{"books", "category_l1", "category_l1 TEXT NOT NULL DEFAULT ''"},
		{"books", "category_l2", "category_l2 TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range cols {
		if err := addColumnIfMissing(ctx, tx, c.table, c.name, c.ddl); err != nil {
			return err
		}
	}
	// 索引依赖新列，必须排在加列之后（它不能写在 0015 的 SQL 里：SQL 先于步骤执行）。
	if _, err := tx.ExecContext(ctx,
		"CREATE INDEX IF NOT EXISTS idx_books_category ON books(category_l1, category_l2)"); err != nil {
		return fmt.Errorf("store: create book category index: %w", err)
	}
	if hadColumn {
		return nil
	}
	return backfillLegacyCategory(ctx, tx)
}

// backfillLegacyCategory 把所有空分类的存量书标成部署方指定的回填分类。
func backfillLegacyCategory(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx,
		"UPDATE books SET category_l1 = ? WHERE category_l1 = ''", legacyCategoryL1()); err != nil {
		return fmt.Errorf("store: backfill book category: %w", err)
	}
	return nil
}
