package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// 0016 的口径：
//   - 空库迁到最新后有 users / user_shelf / user_sessions 三张表，且默认账号已就位；
//   - 单用户结构的存量数据（进度 / 书单 / 关联 / 会话 / 书签）全部回填到默认账号；
//   - 删掉版本记录重跑幂等，不会把数据搬丢。

// TestMigrate0016AddsUserSchema 空库迁移后有账号体系的结构，且重跑 0016 幂等。
func TestMigrate0016AddsUserSchema(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireUserSchema(t, ctx, st)

	// 重跑路径：建表与加列都靠事务内探测，删版本号重放不应报错。
	mustExec(t, ctx, st, "DELETE FROM schema_migrations WHERE version = 16")
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0016 失败（应为幂等）: %v", err)
	}
	requireUserSchema(t, ctx, st)
	// 重放不能把默认账号建出第二份。
	if n := countRows(t, ctx, st, "users"); n != 1 {
		t.Errorf("users 行数 = %d，期望 1（默认账号不该重复）", n)
	}
}

// requireUserSchema 断言 0016 的表、列与索引。
func requireUserSchema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, name := range []string{"users", "user_sessions", "user_shelf"} {
		exists, err := st.tableExists(ctx, name)
		if err != nil || !exists {
			t.Fatalf("表 %s 缺失: exists=%v err=%v", name, exists, err)
		}
	}
	for _, c := range []struct{ table, column string }{
		{"reading_progress", "user_id"},
		{"reading_sessions", "user_id"},
		{"bookmarks", "user_id"},
		{"collections", "user_id"},
		{"collection_books", "user_id"},
	} {
		if !columnExists(t, ctx, st, c.table, c.column) {
			t.Errorf("%s 缺列 %s", c.table, c.column)
		}
	}
	for _, idx := range []string{"ux_users_name", "ix_user_sessions_user", "ix_user_shelf_added"} {
		if !indexExists(t, ctx, st, idx) {
			t.Errorf("缺索引 %s", idx)
		}
	}
	// 默认账号必须存在且是固定的 id=1：存量回填、开放模式兜底都指着它。
	var name string
	if err := st.DB().QueryRowContext(ctx,
		"SELECT name FROM users WHERE id = ?", defaultUserID).Scan(&name); err != nil {
		t.Fatalf("读默认账号: %v", err)
	}
	requireEqual(t, "默认账号名", name, defaultUserName)
}

// TestMigrate0016BackfillsLegacyRows 单用户结构的存量数据全部归入默认账号。
func TestMigrate0016BackfillsLegacyRows(t *testing.T) {
	ctx := context.Background()
	st, bookID := seedLegacyPersonalData(t, ctx)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0015 → 最新失败: %v", err)
	}
	requireUserSchema(t, ctx, st)
	requireBackfilledRows(t, ctx, st, bookID)
}

// seedLegacyPersonalData 造一个 0015 的库并写入单用户结构的个人数据。
func seedLegacyPersonalData(t *testing.T, ctx context.Context) (*Store, int64) {
	t.Helper()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-0015.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateTo(ctx, 15); err != nil {
		t.Fatalf("迁到 0015: %v", err)
	}
	bookID := insertLegacyP3Book(t, ctx, st, "/books/old16.txt", "a16")
	// 单用户结构的写入：这里刻意不经过 store 方法（它们已经要求 user 维度），
	// 直接写 0015 时期的那几列，才能复现「升级前的库」。
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO reading_progress(book_id, chapter_id, chapter_idx, percent, updated_at) "+
			"VALUES (%d, 0, 0, 0.5, '2026-10-01T00:00:00.000Z')", bookID))
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO reading_sessions(book_id, chapter_id, started_at, seconds) "+
			"VALUES (%d, 7, '2026-10-01T00:00:00.000Z', 60)", bookID))
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO bookmarks(book_id, chapter_id, chapter_idx, para_index, label, created_at) "+
			"VALUES (%d, 7, 0, 3, '存量书签', '2026-10-01T00:00:00.000Z')", bookID))
	mustExec(t, ctx, st,
		"INSERT INTO collections(name, created_at) VALUES ('存量书单', '2026-10-01T00:00:00.000Z')")
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO collection_books(collection_id, book_id, added_at) "+
			"SELECT id, %d, '2026-10-01T00:00:00.000Z' FROM collections WHERE name = '存量书单'", bookID))
	return st, bookID
}

// requireBackfilledRows 断言五张表的存量行都挂到了默认账号上。
func requireBackfilledRows(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	for _, table := range []string{
		"reading_progress", "reading_sessions", "bookmarks", "collections", "collection_books",
	} {
		var n int
		if err := st.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", defaultUserID).Scan(&n); err != nil {
			t.Fatalf("查 %s 的归属: %v", table, err)
		}
		if n != 1 {
			t.Errorf("%s 回填到默认账号的行数 = %d，期望 1", table, n)
		}
		if n := countRows(t, ctx, st, table); n != 1 {
			t.Errorf("%s 总行数 = %d，期望 1（不该多出行）", table, n)
		}
	}
	var percent float64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT percent FROM reading_progress WHERE user_id = ? AND book_id = ?",
		defaultUserID, bookID).Scan(&percent); err != nil {
		t.Fatalf("读回填进度: %v", err)
	}
	requireEqual(t, "回填进度 percent", percent, 0.5)
}

// TestMigrate0016AllowsPerUserProgress 两个账号各自拥有同一本书的进度（联合主键生效）。
func TestMigrate0016AllowsPerUserProgress(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	chapterID := chapterIDsOf(t, ctx, st, bookID)[0]
	other, err := st.RegisterUser(ctx, "读者乙")
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	for _, u := range []struct {
		id      int64
		percent float64
	}{{defaultUserID, 0.2}, {other.ID, 0.8}} {
		if err := st.SaveReadingProgress(WithUserID(ctx, u.id), ReadingProgress{
			BookID: bookID, ChapterID: chapterID, Percent: u.percent,
		}); err != nil {
			t.Fatalf("SaveReadingProgress(user=%d): %v", u.id, err)
		}
	}
	// 同一本书两行进度：add user 维度的目的就是这个。
	if n := countRows(t, ctx, st, "reading_progress"); n != 2 {
		t.Errorf("reading_progress 行数 = %d，期望 2", n)
	}
	mine, err := st.ListReadingProgress(WithUserID(ctx, other.ID), 5)
	requireOK(t, "ListReadingProgress", err)
	if len(mine) != 1 || mine[0].Percent != 0.8 {
		t.Errorf("读者乙的进度 = %+v，期望只有自己那 0.8 的一条", mine)
	}
}

// indexExists 判断某个索引是否存在。
func indexExists(t *testing.T, ctx context.Context, st *Store, name string) bool {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&n); err != nil {
		t.Fatalf("查索引 %s: %v", name, err)
	}
	return n > 0
}
