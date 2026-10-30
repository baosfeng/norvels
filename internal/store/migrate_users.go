package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// 0016 的 Go 步骤：把「单用户」结构的个人数据表改造成按账号隔离，并把存量行回填给默认账号。
//
// ---- 为什么整段在 Go 里 ----
// SQLite 想换主键 / 换唯一约束，唯一途径是「建新表 → 搬数据 → 删旧表 → 改名」；
// 而「这些表是不是已经改造过」只能先探测再决定（删掉版本号重跑是排查迁移问题的
// 常规手段，重跑时又重建一次会把数据搬丢）。探测与重建必须在**同一个事务**里，
// 所以放在迁移步骤而不是迁移 SQL：SQL 在步骤之前执行，看不见探测结果。
//
// ---- 回填口径 ----
// 单用户时代只有一位读者，其全部进度 / 书签 / 书单 / 会话都归默认账号（id=1）。
// 建号与回填在同一个事务内完成：迁移提交后不存在「有记录却没有账号」的中间态。

// defaultUserName 是存量数据归属的默认账号名。
//
// 选 reader 而不是「未登录」之类：它是升级后真实存在、可登录的账号，
// 而不是一个只在本文件里出现的魔法值。
const defaultUserName = "reader"

// defaultUserID 是默认账号的固定 id。
//
// 回填、开放模式兜底、鉴权中间件都引用它，必须只有一份定义。显式写 id 而不是让
// AUTOINCREMENT 分配：存量数据的 user_id 回填值要与它在同一次迁移里对齐。
const defaultUserID int64 = 1

// readingProgressSchema 是 0016 之后 reading_progress 的结构（重建与新建共用一份）。
const readingProgressSchema = `CREATE TABLE reading_progress (
	user_id     INTEGER NOT NULL DEFAULT 0,
	book_id     INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	chapter_id  INTEGER NOT NULL,
	chapter_idx INTEGER NOT NULL,
	percent     REAL    NOT NULL DEFAULT 0,
	para_index  INTEGER NOT NULL DEFAULT 0,
	updated_at  TEXT    NOT NULL,
	PRIMARY KEY (user_id, book_id)
)`

// collectionsSchema 是 0016 之后 collections 与 collection_books 的结构。
//
// 唯一性从「全库唯一」收窄为「同一账号内唯一」：两个账号都能有一个叫「想读」的书单。
const collectionsSchema = `CREATE TABLE collections (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL DEFAULT 0,
	name       TEXT    NOT NULL,
	created_at TEXT    NOT NULL
)`

const collectionBooksSchema = `CREATE TABLE collection_books (
	user_id       INTEGER NOT NULL DEFAULT 0,
	collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
	book_id       INTEGER NOT NULL,
	added_at      TEXT    NOT NULL,
	PRIMARY KEY (user_id, collection_id, book_id)
)`

// addUserScopedTablesStep 是 0016 的全部内容。
func addUserScopedTablesStep(ctx context.Context, tx *sql.Tx) error {
	if err := ensureDefaultUser(ctx, tx); err != nil {
		return err
	}
	if err := rebuildReadingProgress(ctx, tx); err != nil {
		return err
	}
	if err := rebuildCollections(ctx, tx); err != nil {
		return err
	}
	if err := addUserScopedColumn(ctx, tx, "reading_sessions", []string{
		"CREATE INDEX IF NOT EXISTS ix_sessions_user_started " +
			"ON reading_sessions(user_id, started_at DESC, id DESC)",
	}); err != nil {
		return err
	}
	if err := addUserScopedColumn(ctx, tx, "bookmarks", bookmarkUserIndexes); err != nil {
		return err
	}
	log.Printf("迁移 0016：个人数据已按账号隔离，存量记录归入默认账号 %q（id=%d）",
		defaultUserName, defaultUserID)
	// 重建表删掉的旧数据页进了 freelist。这里刻意不在启动路径上自动 VACUUM：
	// 它会重写整个库文件并需要额外一份等效磁盘空间，把「启动失败」的风险
	// 压在最不该出问题的地方，而这次搬的只是个人记录那几张表。
	// 需要真的把文件缩回去时，按 docs/deploy.md §7.3 在停服窗口手动执行。
	log.Printf("迁移 0016：如需回收空洞页，请在停服窗口执行 VACUUM（见 docs/deploy.md §7.3）")
	return nil
}

// bookmarkUserIndexes 把「同位置唯一」从全书唯一收窄为「同一账号内唯一」：
// 两个账号可以在同一段落各存一条书签。
var bookmarkUserIndexes = []string{
	"DROP INDEX IF EXISTS ux_bookmarks_pos",
	"CREATE UNIQUE INDEX IF NOT EXISTS ux_bookmarks_pos " +
		"ON bookmarks(user_id, book_id, chapter_id, para_index)",
	"CREATE INDEX IF NOT EXISTS ix_bookmarks_user " +
		"ON bookmarks(user_id, book_id, created_at DESC, id DESC)",
}

// ensureDefaultUser 建默认账号（幂等）。
func ensureDefaultUser(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO users(id, name, created_at) VALUES (?, ?, ?)",
		defaultUserID, defaultUserName, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("create default user: %w", err)
	}
	return nil
}

// rebuildReadingProgress 把 reading_progress 改成 (user_id, book_id) 联合主键。
//
// 两种升级路径走同一条代码：表不存在就直接建新结构；表存在但没有 user_id 列
// 就是单用户结构，搬数据后重建。已经是新结构则什么也不做（重跑安全）。
func rebuildReadingProgress(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "reading_progress")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.ExecContext(ctx, readingProgressSchema); err != nil {
			return fmt.Errorf("create reading_progress: %w", err)
		}
	} else if err := rebuildProgressIfLegacy(ctx, tx); err != nil {
		return err
	}
	return rebuildTableIndexes(ctx, tx, []string{
		"CREATE INDEX IF NOT EXISTS idx_reading_progress_updated " +
			"ON reading_progress(updated_at DESC, book_id DESC)",
		"CREATE INDEX IF NOT EXISTS ix_reading_progress_user " +
			"ON reading_progress(user_id, updated_at DESC, book_id DESC)",
	})
}

// rebuildProgressIfLegacy 只在「表已存在且没有 user_id 列」时重建并搬数据。
func rebuildProgressIfLegacy(ctx context.Context, tx *sql.Tx) error {
	has, err := columnExistsInTx(ctx, tx, "reading_progress", "user_id")
	if err != nil || has {
		return err
	}
	return execAllInTx(ctx, tx, []string{
		"ALTER TABLE reading_progress RENAME TO reading_progress_old",
		readingProgressSchema,
		"INSERT INTO reading_progress(user_id, book_id, chapter_id, chapter_idx, percent, para_index, updated_at) " +
			"SELECT " + defaultUserIDLiteral + ", book_id, chapter_id, chapter_idx, percent, para_index, updated_at " +
			"FROM reading_progress_old",
		"DROP TABLE reading_progress_old",
	}, "rebuild reading_progress")
}

// rebuildCollections 重建 collections 与其关联表 collection_books。
//
// 关联表跟着书单一起重建：它按 collection_id 指过来，书单换表后旧 id 不再有效，
// 因此两张新表用同一批 id 一次搬完。
func rebuildCollections(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "collections")
	if err != nil || !exists {
		return err // 0014 之前的库没有书单表，没什么可搬的
	}
	has, err := columnExistsInTx(ctx, tx, "collections", "user_id")
	if err != nil || has {
		return err
	}
	return execAllInTx(ctx, tx, []string{
		"ALTER TABLE collections RENAME TO collections_old",
		"ALTER TABLE collection_books RENAME TO collection_books_old",
		collectionsSchema,
		collectionBooksSchema,
		"INSERT INTO collections(id, user_id, name, created_at) " +
			"SELECT id, " + defaultUserIDLiteral + ", name, created_at FROM collections_old",
		"INSERT INTO collection_books(user_id, collection_id, book_id, added_at) " +
			"SELECT " + defaultUserIDLiteral + ", collection_id, book_id, added_at FROM collection_books_old",
		"DROP TABLE collection_books_old",
		"DROP TABLE collections_old",
		"CREATE UNIQUE INDEX IF NOT EXISTS ux_collections_user_name ON collections(user_id, name)",
		"CREATE INDEX IF NOT EXISTS ix_collections_user ON collections(user_id, name, id)",
	}, "rebuild collections")
}

// defaultUserIDLiteral 是回填 SQL 里的用户 id 字面量（值来自包级常量 defaultUserID）。
//
// 迁移 SQL 全是常量、不含任何外部输入，直接内联比把同一个参数重复传多次更不容易错位。
const defaultUserIDLiteral = "1"

// addUserScopedColumn 幂等地给一张个人数据表补 user_id 列，并重建依赖它的索引。
//
// 加列这一步会把存量行的 user_id 落成列的默认值 0，因此必须紧接着把 0 回填成
// 默认账号——否则升级后「昨天的阅读记录」会挂到一个不存在的账号上，
// 用户打开历史页看到的是空的。
//
// 回填条件写成 user_id = 0 而不是「无条件 UPDATE」：迁移重放时那些已经归到
// 真实账号的行不能被改回默认账号。
func addUserScopedColumn(ctx context.Context, tx *sql.Tx, table string, indexes []string) error {
	if err := addColumnIfMissing(ctx, tx, table, "user_id",
		"user_id INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE "+table+" SET user_id = "+defaultUserIDLiteral+" WHERE user_id = 0"); err != nil {
		return fmt.Errorf("backfill %s.user_id: %w", table, err)
	}
	return rebuildTableIndexes(ctx, tx, indexes)
}

// execAllInTx 按序执行一组语句，任一失败即返回（外层事务整体回滚）。
func execAllInTx(ctx context.Context, tx *sql.Tx, stmts []string, what string) error {
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	return nil
}

// rebuildTableIndexes 依次执行索引语句（全部 IF NOT EXISTS / IF EXISTS，可重放）。
func rebuildTableIndexes(ctx context.Context, tx *sql.Tx, stmts []string) error {
	return execAllInTx(ctx, tx, stmts, "rebuild user-scoped index")
}
