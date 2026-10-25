package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
}

// ---------- 0013：同书卷名唯一索引 ----------

// volumeTitleUniqueIndex 是 0013 建的同书卷名唯一索引名（表达式索引，TRIM(title)）。
//
// 同一个名字有两个用途：迁移里建它；写入口把「撞这个索引」的错误翻成
// ErrVolumeNameTaken（索引是并发竞态的兜底，正常路径仍走事务内查重）。
const volumeTitleUniqueIndex = "uq_volumes_book_title"

// volumeRow 是去重规划需要的一行卷：id、所属书、库里 TRIM 后的标题。
type volumeRow struct {
	ID    int64
	Book  int64
	Title string
}

// volumeRename 是一条待执行的改名。
type volumeRename struct {
	ID    int64
	Title string
}

// dedupeVolumeTitlesStep 是 0013 的全部内容：先把同书重名卷改名去重，再建唯一索引。
//
// 为什么整段放在 Go 步骤里而不写进迁移 SQL：补序号要先知道本书已有哪些卷名，
// 才能保证新名字不会又撞上真实存在的「原标题 (2)」；纯 SQL 没有有界探测，
// 而改名撞上唯一约束会让整条迁移回滚、服务起不来。
//
// 改名条数走 log.Printf 打进迁移日志：与 cmd 的「已应用 0013 …」同一个 sink，
// 运维升级完直接能看到这台机器改了几条，不需要再去查库。
func dedupeVolumeTitlesStep(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "volumes")
	if err != nil || !exists {
		return err // 老库升级路径上 volumes 可能还不存在
	}
	rows, err := loadVolumeRows(ctx, tx)
	if err != nil {
		return err
	}
	renames := planVolumeRenames(rows)
	if err := applyVolumeRenames(ctx, tx, renames); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"CREATE UNIQUE INDEX IF NOT EXISTS "+volumeTitleUniqueIndex+
			" ON volumes(book_id, TRIM(title))"); err != nil {
		return fmt.Errorf("create unique index %s: %w", volumeTitleUniqueIndex, err)
	}
	log.Printf("迁移 0013：同书卷名去重改名 %d 条，已建唯一索引 %s",
		len(renames), volumeTitleUniqueIndex)
	return nil
}

// loadVolumeRows 按 (book_id, idx, id) 读出全部卷 id 与 TRIM 后标题。
//
// 排序口径写死 (idx, id)：去重时「保留首个」必须可复现，不能随行的物理顺序变。
// 标题取 SQL 的 TRIM 值而不是 Go 的 TrimSpace——唯一索引按 SQL TRIM 建，
// 两边口径必须完全一致。
func loadVolumeRows(ctx context.Context, tx *sql.Tx) ([]volumeRow, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, book_id, TRIM(title) FROM volumes ORDER BY book_id, idx, id")
	if err != nil {
		return nil, fmt.Errorf("list volume titles: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	out := make([]volumeRow, 0, 16)
	for rows.Next() {
		var r volumeRow
		if err := rows.Scan(&r.ID, &r.Book, &r.Title); err != nil {
			return nil, fmt.Errorf("scan volume title: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list volume titles: %w", err)
	}
	return out, nil
}

// planVolumeRenames 为整批卷规划改名：按书分组，组内按 (idx, id) 顺序。
func planVolumeRenames(rows []volumeRow) []volumeRename {
	out := make([]volumeRename, 0, 4)
	for start := 0; start < len(rows); {
		end := start
		for end < len(rows) && rows[end].Book == rows[start].Book {
			end++
		}
		out = append(out, planBookVolumeRenames(rows[start:end])...)
		start = end
	}
	return out
}

// planBookVolumeRenames 规划一本书的改名：每组同 (TRIM(title)) 保留首个，
// 其余改成「原标题 (2)」「原标题 (3)」…。
//
// 先占位全书「本来就唯一」的名字，再给重复项找空位：否则重名卷可能被改成
// 本书另一个真实存在的「原标题 (2)」，撞上紧接着要建的唯一索引。
func planBookVolumeRenames(rows []volumeRow) []volumeRename {
	reserved := make(map[string]struct{}, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if _, dup := seen[r.Title]; dup {
			continue
		}
		seen[r.Title] = struct{}{}
		reserved[r.Title] = struct{}{}
	}
	out := make([]volumeRename, 0, len(rows))
	kept := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if _, dup := kept[r.Title]; !dup {
			kept[r.Title] = struct{}{} // 首个保留原名
			continue
		}
		title := nextFreeVolumeTitle(r.Title, reserved)
		reserved[title] = struct{}{}
		out = append(out, volumeRename{ID: r.ID, Title: title})
	}
	return out
}

// nextFreeVolumeTitle 从「原标题 (2)」起找第一个本书还没占用的名字。
func nextFreeVolumeTitle(base string, reserved map[string]struct{}) string {
	for k := 2; ; k++ {
		candidate := fmt.Sprintf("%s (%d)", base, k)
		if _, taken := reserved[candidate]; !taken {
			return candidate
		}
	}
}

// applyVolumeRenames 逐条改标题（条数由调用方写进迁移日志）。
//
// 不动 books.updated_at：迁移只修数据形状，不改「这本书最后被改过」的业务时间。
func applyVolumeRenames(ctx context.Context, tx *sql.Tx, renames []volumeRename) error {
	for _, r := range renames {
		if _, err := tx.ExecContext(ctx,
			"UPDATE volumes SET title = ? WHERE id = ?", r.Title, r.ID); err != nil {
			return fmt.Errorf("rename volume %d: %w", r.ID, err)
		}
	}
	return nil
}
