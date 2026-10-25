package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// 0014 的口径：四列默认零值、三张新表与索引就位；
// 删掉版本记录重跑必须幂等（加列步骤靠事务内探测，不靠 SQL）。

// TestMigrate0014AddsSchema 空库迁到最新后有 P3 的结构，且重跑 0014 不报错。
func TestMigrate0014AddsSchema(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireP3Schema(t, ctx, st)

	// 重跑路径：ALTER TABLE ADD COLUMN 撞列已存在，只有 Go 步骤的探测能兜住。
	mustExec(t, ctx, st, "DELETE FROM schema_migrations WHERE version = 14")
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0014 失败（应为幂等）: %v", err)
	}
	requireP3Schema(t, ctx, st)
}

// TestMigrate0014From0013KeepsDefaults 存量书与存量进度保持零值，不回填、不猜。
func TestMigrate0014From0013KeepsDefaults(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-0013.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateTo(ctx, 13); err != nil {
		t.Fatalf("迁到 0013: %v", err)
	}

	bookID := insertLegacyP3Book(t, ctx, st, "/books/old14.txt", "e14")
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO reading_progress(book_id, chapter_id, chapter_idx, percent, updated_at) "+
			"VALUES (?, 0, 0, 0.5, ?)", bookID, formatTime(nowUTC())); err != nil {
		t.Fatalf("写入存量进度: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0013 → 最新失败: %v", err)
	}
	requireP3Schema(t, ctx, st)

	var (
		rating, newChapters int
		lastChapterAt       string
	)
	if err := st.DB().QueryRowContext(ctx,
		"SELECT rating, new_chapters, last_chapter_at FROM books WHERE id = ?", bookID).
		Scan(&rating, &newChapters, &lastChapterAt); err != nil {
		t.Fatalf("读存量书: %v", err)
	}
	if rating != 0 || newChapters != 0 || lastChapterAt != "" {
		t.Errorf("存量书 = rating %d / new_chapters %d / last_chapter_at %q，期望全零值",
			rating, newChapters, lastChapterAt)
	}
	var para int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT para_index FROM reading_progress WHERE book_id = ?", bookID).Scan(&para); err != nil {
		t.Fatalf("读存量进度: %v", err)
	}
	if para != 0 {
		t.Errorf("存量进度 para_index = %d，期望 0", para)
	}
}

// TestDeleteBookClearsP3Tables 删书要清掉没有外键的 reading_sessions / collection_books。
func TestDeleteBookClearsP3Tables(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := insertLegacyP3Book(t, ctx, st, "/books/p3-delete.txt", "f14")
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO reading_sessions(book_id, chapter_id, started_at, seconds) "+
			"VALUES (%d, 7, '2026-10-01T00:00:00.000Z', 60)", bookID))
	mustExec(t, ctx, st,
		"INSERT INTO collections(name, created_at) VALUES ('待清理书单', '2026-10-01T00:00:00.000Z')")
	var collID int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT id FROM collections WHERE name = '待清理书单'").Scan(&collID); err != nil {
		t.Fatalf("读书单 id: %v", err)
	}
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO collection_books(collection_id, book_id, added_at) "+
			"VALUES (%d, %d, '2026-10-01T00:00:00.000Z')", collID, bookID))

	if err := st.DeleteBook(ctx, bookID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	if n := countRows(t, ctx, st, "reading_sessions"); n != 0 {
		t.Errorf("reading_sessions 残留 %d 行", n)
	}
	if n := countRows(t, ctx, st, "collection_books"); n != 0 {
		t.Errorf("collection_books 残留 %d 行", n)
	}
	// 书单本体不是书的从属数据，必须保留。
	if n := countRows(t, ctx, st, "collections"); n != 1 {
		t.Errorf("collections 行数 = %d，期望 1（书单不该随书删除）", n)
	}
}

// requireP3Schema 断言 0014 的结构：四列、三表与两个会话索引。
func requireP3Schema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for _, c := range []struct{ table, column string }{
		{"books", "rating"},
		{"books", "new_chapters"},
		{"books", "last_chapter_at"},
		{"reading_progress", "para_index"},
	} {
		if !columnExists(t, ctx, st, c.table, c.column) {
			t.Errorf("%s 缺列 %s", c.table, c.column)
		}
	}
	for _, name := range []string{"reading_sessions", "collections", "collection_books"} {
		exists, err := st.tableExists(ctx, name)
		if err != nil || !exists {
			t.Fatalf("表 %s 缺失: exists=%v err=%v", name, exists, err)
		}
	}
	for _, idx := range []string{"ix_sessions_started", "ix_sessions_book"} {
		var n int
		if err := st.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("索引 %s 缺失: n=%d err=%v", idx, n, err)
		}
	}
}

// insertLegacyP3Book 直接写一行 books（绕过 SaveBook 的卷/章节写入），返回 id。
func insertLegacyP3Book(t *testing.T, ctx context.Context, st *Store, path, sha string) int64 {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('示例书名', ?, ?)", path, sha); err != nil {
		t.Fatalf("写入示例书: %v", err)
	}
	var id int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT id FROM books WHERE source_path = ?", path).Scan(&id); err != nil {
		t.Fatalf("读 book id: %v", err)
	}
	return id
}

// countRows 返回整表行数（0014 测试只关心「清没清干净」）。
func countRows(t *testing.T, ctx context.Context, st *Store, table string) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
