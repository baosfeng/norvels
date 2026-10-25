package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrate0009AddsBookAuthors 锁死 0009 的结构口径与幂等性。
//
// 三个场景：
//   - 空库迁到最新：books_authors 表、三列与反向索引都在；
//   - 已有 0008 的库升级：0008 阶段不该有这张表，升到最新后必须出现；
//   - 重复执行：退回版本 9 再迁移必须能通过（CREATE TABLE/INDEX 都带 IF NOT EXISTS）。
func TestMigrate0009AddsBookAuthors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	requireBookAuthorsSchema(t, ctx, st)

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 9"); err != nil {
		t.Fatalf("回退版本: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0009 失败（应为幂等）: %v", err)
	}
	requireBookAuthorsSchema(t, ctx, st)
}

// TestMigrate0009From0008 从「只有 0008 的库」升到最新。
func TestMigrate0009From0008(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-0008.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.MigrateTo(ctx, 8); err != nil {
		t.Fatalf("迁到 0008 失败: %v", err)
	}
	if exists, err := st.tableExists(ctx, "books_authors"); err != nil || exists {
		t.Fatalf("0008 不该有 books_authors: exists=%v err=%v", exists, err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0008 → 最新失败: %v", err)
	}
	requireBookAuthorsSchema(t, ctx, st)
}

// requireBookAuthorsSchema 断言 0009 的结构（表 + 三列 + 反向索引）。
func requireBookAuthorsSchema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	exists, err := st.tableExists(ctx, "books_authors")
	if err != nil || !exists {
		t.Fatalf("books_authors 表缺失: exists=%v err=%v", exists, err)
	}
	for _, col := range []string{"book_id", "author_id", "position"} {
		if !columnExists(t, ctx, st, "books_authors", col) {
			t.Errorf("books_authors 缺列 %q", col)
		}
	}
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_books_authors_author'").Scan(&n); err != nil {
		t.Fatalf("查索引: %v", err)
	}
	if n == 0 {
		t.Error("缺索引 idx_books_authors_author")
	}
}
