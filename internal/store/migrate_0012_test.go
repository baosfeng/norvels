package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrate0012AddsJobFilesAndTagSource 锁死 0012 的结构口径与幂等性。
func TestMigrate0012AddsJobFilesAndTagSource(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireImportJobFilesSchema(t, ctx, st)

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version >= 12"); err != nil {
		t.Fatalf("回退版本: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0012 失败（应为幂等）: %v", err)
	}
	requireImportJobFilesSchema(t, ctx, st)
}

// TestMigrate0012From0011 从「只到 0011 的库」升到最新，存量标签来源保持空串。
func TestMigrate0012From0011(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-0011.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateTo(ctx, 11); err != nil {
		t.Fatalf("迁到 0011 失败: %v", err)
	}
	if exists, err := st.tableExists(ctx, "import_job_files"); err != nil || exists {
		t.Fatalf("0011 不该有 import_job_files: exists=%v err=%v", exists, err)
	}
	bookID := seedLegacyBooksAndTag(t, ctx, st)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0011 → 最新失败: %v", err)
	}
	requireImportJobFilesSchema(t, ctx, st)
	requireTagSource(t, ctx, st, bookID, "旧标签", TagSourceManual)
}

// seedLegacyBooksAndTag 按 0011 的结构写入存量书、标签与关联（source 列还不存在）。
func seedLegacyBooksAndTag(t *testing.T, ctx context.Context, st *Store) int64 {
	t.Helper()
	mustExec(t, ctx, st,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('旧书', '/books/old12.txt', 'cc')")
	mustExec(t, ctx, st, "INSERT INTO tags(name) VALUES ('旧标签')")
	mustExec(t, ctx, st,
		"INSERT INTO book_tags(book_id, tag_id) SELECT b.id, t.id FROM books b, tags t WHERE b.source_path = '/books/old12.txt'")
	var bookID int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT id FROM books WHERE source_path = '/books/old12.txt'").Scan(&bookID); err != nil {
		t.Fatalf("读 book id: %v", err)
	}
	return bookID
}

// mustExec 执行一条无返回的 SQL，失败即 Fatal。
func mustExec(t *testing.T, ctx context.Context, st *Store, query string) {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx, query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// requireImportJobFilesSchema 断言 0012 的结构：表、唯一索引与 book_tags.source 列。
func requireImportJobFilesSchema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	exists, err := st.tableExists(ctx, "import_job_files")
	if err != nil || !exists {
		t.Fatalf("import_job_files 表缺失: exists=%v err=%v", exists, err)
	}
	for _, col := range []string{"id", "job_id", "source", "display_path", "status", "reason", "book_id", "updated_at"} {
		if !columnExists(t, ctx, st, "import_job_files", col) {
			t.Errorf("import_job_files 缺列 %q", col)
		}
	}
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'ux_import_job_files'").Scan(&n); err != nil || n != 1 {
		t.Errorf("唯一索引 ux_import_job_files 缺失: n=%d err=%v", n, err)
	}
	if !columnExists(t, ctx, st, "book_tags", "source") {
		t.Error("book_tags 缺列 source")
	}
}
