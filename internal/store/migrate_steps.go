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

// addBookSerialStatusStep 给 books 补 0010 的连载状态列（幂等）。
//
// 为什么不用迁移 SQL 里的 ALTER TABLE：SQLite 没有 ADD COLUMN IF NOT EXISTS，
// 而「删掉版本号重跑」是排查迁移问题的常规手段，撞 duplicate column 会让整次迁移失败。
func addBookSerialStatusStep(ctx context.Context, tx *sql.Tx) error {
	has, err := columnExistsInTx(ctx, tx, "books", "serial_status")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"ALTER TABLE books ADD COLUMN serial_status TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add column books.serial_status: %w", err)
	}
	return nil
}

// addBookReadStatusStep 给 books 补 0011 的阅读状态列（幂等）。
//
// 与 0008/0010 同理：SQLite 没有 ADD COLUMN IF NOT EXISTS，版本回退重跑会撞
// "duplicate column name"，整次迁移失败；探测放在 Go 里、跑在 0011 的事务内。
func addBookReadStatusStep(ctx context.Context, tx *sql.Tx) error {
	has, err := columnExistsInTx(ctx, tx, "books", "read_status")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"ALTER TABLE books ADD COLUMN read_status TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add column books.read_status: %w", err)
	}
	return nil
}

// addBookTagsSourceStep 给 book_tags 补 0012 的标签来源列（幂等）。
//
// 存量行保持空串（手动 / 历史），不回填：猜不出旧标签是手加的还是导入的。
func addBookTagsSourceStep(ctx context.Context, tx *sql.Tx) error {
	// 老库升级路径上 book_tags 可能还不存在（与 0006 的 chapters 同一个坑）：
	// 表都没有就没什么可加的，直接放行；0001 的建表语句不在这条路径上重跑。
	exists, err := tableExistsInTx(ctx, tx, "book_tags")
	if err != nil || !exists {
		return err
	}
	has, err := columnExistsInTx(ctx, tx, "book_tags", "source")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"ALTER TABLE book_tags ADD COLUMN source TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add column book_tags.source: %w", err)
	}
	return nil
}

// addP3ColumnsStep 给 books / reading_progress 补 0014 的列（幂等）。
//
// 与 0008/0010/0011 同理：SQLite 没有 ADD COLUMN IF NOT EXISTS，版本回退重跑会撞
// "duplicate column name"。旧库升级路径上 reading_progress 可能还不存在（0003 之前
// 的畸形库），表都没有就没什么可加的，直接跳过。
func addP3ColumnsStep(ctx context.Context, tx *sql.Tx) error {
	cols := []struct{ table, name, ddl string }{
		{"books", "rating", "rating INTEGER NOT NULL DEFAULT 0"},
		{"books", "new_chapters", "new_chapters INTEGER NOT NULL DEFAULT 0"},
		{"books", "last_chapter_at", "last_chapter_at TEXT NOT NULL DEFAULT ''"},
		{"reading_progress", "para_index", "para_index INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range cols {
		if err := addColumnIfMissing(ctx, tx, c.table, c.name, c.ddl); err != nil {
			return err
		}
	}
	return nil
}

// addColumnIfMissing 探测表/列后再 ALTER：表不存在（老库升级路径）或列已在都跳过。
func addColumnIfMissing(ctx context.Context, tx *sql.Tx, table, column, ddl string) error {
	exists, err := tableExistsInTx(ctx, tx, table)
	if err != nil || !exists {
		return err
	}
	has, err := columnExistsInTx(ctx, tx, table, column)
	if err != nil || has {
		return err
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+ddl); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
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

// ---------- 迁移步骤登记表 ----------

// migrationStepsByVersion 登记带 Go 步骤的迁移（按版本号索引）。
//
// 步骤闭包只捕获版本号，状态全部走事务内的探测（表/列是否存在），因此对
// 「空库」「老库」「已部分迁移的库」都走同一条代码路径，不需要额外分支。
// 登记表与步骤实现放在同一个文件：新增迁移只改一处，migrate.go 也不会再涨。
var migrationStepsByVersion = map[int][]migrationStep{
	6: {{
		Name: "drop chapter segmented columns",
		Run:  dropSegColumnsStep,
	}},
	7: {{
		Name: "pack chapter content into blocks",
		Run:  rebuildChaptersForBlocks,
	}},
	8: {{
		Name: "add book metadata columns",
		Run:  addBookMetadataColumnsStep,
	}},
	10: {{
		Name: "add book serial status column",
		Run:  addBookSerialStatusStep,
	}},
	11: {{
		Name: "add book read status column",
		Run:  addBookReadStatusStep,
	}},
	12: {{
		Name: "add book tags source column",
		Run:  addBookTagsSourceStep,
	}},
	13: {{
		Name: "dedupe volume titles and add unique index",
		Run:  dedupeVolumeTitlesStep,
	}},
	14: {{
		Name: "add p3 rating and progress columns",
		Run:  addP3ColumnsStep,
	}},
	15: {{
		Name: "add book category columns and backfill legacy",
		Run:  addCategoryColumnsStep,
	}},
	16: {{
		Name: "scope personal data to users and create default user",
		Run:  addUserScopedTablesStep,
	}},
	17: {{
		Name: "rebuild blocks to 8 MB codebook format",
		Run:  rebuildBlocksForCodebook,
	}},
	20: {{
		Name: "add book quality columns",
		Run:  addBookQualityColumnsStep,
	}},
	21: {{Name: "create import reviews and add superseded_by", Run: createImportReviewsStep}},
	22: {{Name: "populate dashboard precompute tables", Run: populateDashboardPrecomputeStep}},
	// 0025：书级健康计数（缺简介 / 缺作者 / 超薄书 / 章节数分桶）的预计算结构与维护触发器。
	25: {{Name: "create book health counts and triggers", Run: createBookHealthCountStep}},
	// 0026：库级总量（书数 / 书目字数 / 已算质量分书数）的预计算结构与维护触发器。
	26: {{Name: "create book totals and triggers", Run: createBookTotalsStep}},
}

// storeStepsByVersion 登记「需要 Store」的迁移步骤（跨库写入，拿不到连接就做不了）。
//
// 与上面的 migrationStepsByVersion 分开：那张表的签名是 func(ctx, tx)，
// 是不依赖连接池的纯主库步骤；0018 要把块写进正文库（另一个文件）、0019 要在
// 正文库建指纹表，都只能在 applyMigration 里把 Store 传进来。
var storeStepsByVersion = map[int]func(ctx context.Context, tx *sql.Tx, s *Store) error{
	18: splitContentBlocksStep,
	19: createFingerprintTableStep,
	// 0023：正文库建书级指纹覆盖表并回填（实现见 migrate_0023.go）。
	// 加项时把这一行复制给下一个跨库迁移即可。
	23: createFingerprintCoverageStep,
}
