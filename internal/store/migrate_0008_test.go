package store

import (
	"context"
	"testing"
)

// TestMigrate0008AddsBookMetadata 锁死 0008 的结构口径与幂等性。
//
// 场景一：迁移到最新后 books 有 8 个新列、book_covers 表存在。
// 场景二（回归）：把版本记录退回 7 再迁移，0008 必须能重跑——ALTER TABLE
// 没有 IF NOT EXISTS，重复执行会 duplicate column（0005/0006 都踩过同类陷阱）。
func TestMigrate0008AddsBookMetadata(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	for _, col := range []string{"language", "publisher", "isbn", "pub_date",
		"series", "series_index", "source_format", "has_cover"} {
		if !columnExists(t, ctx, st, "books", col) {
			t.Errorf("books 缺列 %q", col)
		}
	}
	exists, err := st.tableExists(ctx, "book_covers")
	if err != nil || !exists {
		t.Fatalf("book_covers 表缺失: exists=%v err=%v", exists, err)
	}

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version >= 8"); err != nil {
		t.Fatalf("回退版本: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0008 失败（应为幂等）: %v", err)
	}
}
