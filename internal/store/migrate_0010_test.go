package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrate0010AddsSerialStatusAndSettings 锁死 0010 的结构口径与幂等性。
//
// 三个场景：
//   - 空库迁到最新：books.serial_status 列与 settings 表都在；
//   - 删掉版本号重跑：必须能通过（加列走 Go 步骤先探测，建表带 IF NOT EXISTS）；
//   - 已应用过 0010 的库再 Migrate 一次：仍是最高版本，不重复执行。
func TestMigrate0010AddsSerialStatusAndSettings(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	requireSerialAndSettingsSchema(t, ctx, st)

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 10"); err != nil {
		t.Fatalf("回退版本: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("重跑 0010 失败（应为幂等）: %v", err)
	}
	requireSerialAndSettingsSchema(t, ctx, st)
}

// TestMigrate0010From0009 从「只有 0009 的库」升到最新。
func TestMigrate0010From0009(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-0009.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.MigrateTo(ctx, 9); err != nil {
		t.Fatalf("迁到 0009 失败: %v", err)
	}
	if columnExists(t, ctx, st, "books", "serial_status") {
		t.Error("0009 不该有 books.serial_status")
	}
	if exists, err := st.tableExists(ctx, "settings"); err != nil || exists {
		t.Fatalf("0009 不该有 settings: exists=%v err=%v", exists, err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0009 → 最新失败: %v", err)
	}
	requireSerialAndSettingsSchema(t, ctx, st)
}

// TestMigrate0010ExistingBooksGetUnknownSerial 存量书的 serial_status 回填为空串。
func TestMigrate0010ExistingBooksGetUnknownSerial(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "norvels-old.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateTo(ctx, 9); err != nil {
		t.Fatalf("迁到 0009 失败: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('旧书', '/books/old.txt', 'aa')"); err != nil {
		t.Fatalf("写入存量书: %v", err)
	}
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("0009 → 最新失败: %v", err)
	}
	var serial string
	if err := st.DB().QueryRowContext(ctx,
		"SELECT serial_status FROM books WHERE source_path = '/books/old.txt'").Scan(&serial); err != nil {
		t.Fatalf("读 serial_status: %v", err)
	}
	if serial != SerialStatusUnknown {
		t.Errorf("存量书 serial_status = %q，期望空串（不猜）", serial)
	}
}

// requireSerialAndSettingsSchema 断言 0010 的结构（列 + 表 + 三列）。
func requireSerialAndSettingsSchema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if !columnExists(t, ctx, st, "books", "serial_status") {
		t.Error("books 缺列 serial_status")
	}
	exists, err := st.tableExists(ctx, "settings")
	if err != nil || !exists {
		t.Fatalf("settings 表缺失: exists=%v err=%v", exists, err)
	}
	for _, col := range []string{"key", "value", "updated_at"} {
		if !columnExists(t, ctx, st, "settings", col) {
			t.Errorf("settings 缺列 %q", col)
		}
	}
}
