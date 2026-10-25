package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrate0011AddsReadStatusAndBookmarks 锁死 0011 的结构口径与幂等性。
func TestMigrate0011AddsReadStatusAndBookmarks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireReadStatusSchema(t, ctx, st)

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version >= 11"); err != nil {
		t.Fatalf("回退版本: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0011 失败（应为幂等）: %v", err)
	}
	requireReadStatusSchema(t, ctx, st)
}

// TestMigrate0011From0010 从「只到 0010 的库」升到最新，存量书 read_status 保持空串。
func TestMigrate0011From0010(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-0010.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.MigrateTo(ctx, 10); err != nil {
		t.Fatalf("迁到 0010 失败: %v", err)
	}
	if exists, err := st.tableExists(ctx, "bookmarks"); err != nil || exists {
		t.Fatalf("0010 不该有 bookmarks: exists=%v err=%v", exists, err)
	}
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('存量书', '/books/old11.txt', 'bb')"); err != nil {
		t.Fatalf("写入存量书: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0010 → 最新失败: %v", err)
	}
	requireReadStatusSchema(t, ctx, st)
	var status string
	if err := st.DB().QueryRowContext(ctx,
		"SELECT read_status FROM books WHERE source_path = '/books/old11.txt'").Scan(&status); err != nil {
		t.Fatalf("读 read_status: %v", err)
	}
	if status != ReadStatusUnset {
		t.Errorf("存量书 read_status = %q，期望空串（不回填）", status)
	}
}

// requireReadStatusSchema 断言 0011 的结构：books.read_status 列、bookmarks 表与两个索引。
func requireReadStatusSchema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if !columnExists(t, ctx, st, "books", "read_status") {
		t.Error("books 缺列 read_status")
	}
	exists, err := st.tableExists(ctx, "bookmarks")
	if err != nil || !exists {
		t.Fatalf("bookmarks 表缺失: exists=%v err=%v", exists, err)
	}
	for _, col := range []string{"id", "book_id", "chapter_id", "chapter_idx", "para_index", "label", "created_at"} {
		if !columnExists(t, ctx, st, "bookmarks", col) {
			t.Errorf("bookmarks 缺列 %q", col)
		}
	}
	for _, idx := range []string{"ux_bookmarks_pos", "ix_bookmarks_book"} {
		var n int
		if err := st.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("索引 %s 缺失: n=%d err=%v", idx, n, err)
		}
	}
}
