package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// openBare 打开一个「什么都不建」的空库。
func openBare(t *testing.T, name string) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, path
}

// TestMigrateFreshAppliesAll 验证空库一次迁移到最新版本且只应用一次。
func TestMigrateFreshAppliesAll(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "fresh.db")

	status, err := st.MigrateTo(ctx, LatestVersion())
	if err != nil {
		t.Fatalf("MigrateTo: %v", err)
	}
	if status.FromVersion != 0 {
		t.Errorf("空库 FromVersion = %d，期望 0", status.FromVersion)
	}
	if status.ToVersion != LatestVersion() {
		t.Errorf("ToVersion = %d，期望 %d", status.ToVersion, LatestVersion())
	}
	if len(status.Applied) != status.Total {
		t.Errorf("应用了 %d 个迁移，打包 %d 个", len(status.Applied), status.Total)
	}
	if status.AdoptedLegacy {
		t.Error("空库不应被判为老库")
	}
	if err := assertTables(ctx, st, "schema_migrations", "books", "chapters", "reading_progress"); err != nil {
		t.Error(err)
	}

	// 再跑一次：不应重复应用。
	again, err := st.MigrateTo(ctx, LatestVersion())
	if err != nil {
		t.Fatalf("第二次 MigrateTo: %v", err)
	}
	if len(again.Applied) != 0 {
		t.Errorf("第二次迁移应用了 %d 个（应为 0）", len(again.Applied))
	}
	if !again.AlreadyAtMax {
		t.Error("第二次迁移应标记 AlreadyAtMax")
	}
}

// TestMigrationFilesOrdered 校验打包的迁移按版本升序且版本号唯一。
func TestMigrationFilesOrdered(t *testing.T) {
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("没有打包任何迁移")
	}
	for i := 1; i < len(all); i++ {
		if all[i].Version <= all[i-1].Version {
			t.Errorf("迁移未按升序：%s 在 %s 之后", all[i].Name, all[i-1].Name)
		}
	}
	if all[0].Name != "0001_init.sql" {
		t.Errorf("首个迁移 = %q，期望 0001_init.sql", all[0].Name)
	}
	for _, m := range all {
		if strings.TrimSpace(m.SQL) == "" {
			t.Errorf("迁移 %s 内容为空", m.Name)
		}
	}
}

// TestMigrateAdoptsLegacyDatabase 复现「老库」：user_version=1、有 books 表、无 schema_migrations。
//
// 期望：直接记 0001 为已应用，不重跑建表（数据保留），后续迁移照常应用。
func TestMigrateAdoptsLegacyDatabase(t *testing.T) {
	st := seedLegacyDatabase(t)
	assertLegacyDetected(t, st)
	migrateLegacy(t, st)
	assertLegacyDataKept(t, st)
	assertBaselineRecorded(t, st)
}

// seedLegacyDatabase 手工构造 v1 老库：只建 books 表并写入一行（模拟旧版 Migrate 的产物）。
func seedLegacyDatabase(t *testing.T) *Store {
	t.Helper()
	st, _ := openBare(t, "legacy.db")
	ctx := context.Background()
	if _, err := st.DB().ExecContext(ctx, legacyBooksDDL); err != nil {
		t.Fatalf("建老库 books: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "INSERT INTO books(title) VALUES ('老书')"); err != nil {
		t.Fatalf("插入老数据: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		t.Fatalf("设置 user_version: %v", err)
	}
	return st
}

// legacyBooksDDL 是老库唯一存在的业务表结构。
const legacyBooksDDL = "CREATE TABLE books (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL);"

// assertLegacyDetected 校验 dry-run 探测把该库判为老库（版本 1，待应用 = 总数-1）。
func assertLegacyDetected(t *testing.T, st *Store) {
	t.Helper()
	pending, info, err := st.PendingMigrations(context.Background())
	if err != nil {
		t.Fatalf("PendingMigrations: %v", err)
	}
	if !info.Legacy || info.Version != legacyBaselineVersion {
		t.Errorf("老库探测 = %+v，期望 Legacy=true Version=%d", info, legacyBaselineVersion)
	}
	if want := len(mustLoad(t)) - 1; len(pending) != want {
		t.Errorf("老库待应用 %d 个，期望 %d 个", len(pending), want)
	}
}

// migrateLegacy 执行老库升级并校验「记为 0001 + 只应用后续迁移」。
func migrateLegacy(t *testing.T, st *Store) {
	t.Helper()
	status, err := st.MigrateTo(context.Background(), LatestVersion())
	if err != nil {
		t.Fatalf("老库迁移: %v", err)
	}
	if !status.AdoptedLegacy {
		t.Error("应标记 AdoptedLegacy")
	}
	if status.FromVersion != legacyBaselineVersion {
		t.Errorf("FromVersion = %d，期望 %d", status.FromVersion, legacyBaselineVersion)
	}
	if want := len(mustLoad(t)) - 1; len(status.Applied) != want {
		t.Errorf("应用 %d 个迁移，期望 %d 个（不含 0001）", len(status.Applied), want)
	}
}

// assertLegacyDataKept 校验老数据没被重跑建表删掉。
func assertLegacyDataKept(t *testing.T, st *Store) {
	t.Helper()
	var title string
	if err := st.DB().QueryRowContext(context.Background(),
		"SELECT title FROM books WHERE id = 1").Scan(&title); err != nil {
		t.Fatalf("读老数据: %v", err)
	}
	if title != "老书" {
		t.Errorf("老数据 title = %q，期望 老书", title)
	}
}

// assertBaselineRecorded 校验版本表里有版本 1 的记录且只有一条。
func assertBaselineRecorded(t *testing.T, st *Store) {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&n); err != nil {
		t.Fatalf("查版本记录: %v", err)
	}
	if n != 1 {
		t.Errorf("版本 1 记录数 = %d，期望 1", n)
	}
}

// TestMigrateRollsBackFailedMigration 验证失败迁移整体回滚、版本不前进。
func TestMigrateRollsBackFailedMigration(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "broken.db")

	bad := migration{
		Version: 99,
		Name:    "0099_broken.sql",
		SQL:     "CREATE TABLE probe_should_vanish (id INTEGER);\nTHIS IS NOT SQL;\n",
	}
	if err := st.ensureMigrationTable(ctx); err != nil {
		t.Fatalf("ensureMigrationTable: %v", err)
	}
	err := st.applyMigration(ctx, bad)
	if err == nil {
		t.Fatal("坏迁移应返回错误")
	}
	if !strings.Contains(err.Error(), "0099_broken.sql") {
		t.Errorf("错误信息应包含迁移名: %v", err)
	}
	// 事务回滚：半截建表不能残留。
	if exists, _ := st.tableExists(ctx, "probe_should_vanish"); exists {
		t.Error("坏迁移里的建表未被回滚")
	}
	// 版本不前进。
	v, err := st.maxRecordedVersion(ctx)
	if err != nil {
		t.Fatalf("maxRecordedVersion: %v", err)
	}
	if v != 0 {
		t.Errorf("失败后版本 = %d，期望 0", v)
	}
}

// TestMigrateToTarget 验证 --to 只迁移到指定版本。
func TestMigrateToTarget(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "target.db")
	status, err := st.MigrateTo(ctx, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1): %v", err)
	}
	if status.ToVersion != 1 {
		t.Errorf("ToVersion = %d，期望 1", status.ToVersion)
	}
	if exists, _ := st.tableExists(ctx, "reading_progress"); exists {
		t.Error("只迁到 1 却建了 reading_progress")
	}
	if !status.AlreadyAtMax {
		// 打包版本高于 1，所以不是「已是最高」。
		t.Log("AlreadyAtMax 语义按打包最高版本判定，允许为 false")
	}
}

// TestParseMigrationVersion 覆盖文件名校验。
func TestParseMigrationVersion(t *testing.T) {
	ok := map[string]int{"0001_init.sql": 1, "0042_add_x.sql": 42}
	for name, want := range ok {
		got, err := parseMigrationVersion(name)
		if err != nil || got != want {
			t.Errorf("parseMigrationVersion(%q) = %d, %v（期望 %d）", name, got, err, want)
		}
	}
	bad := []string{"1_init.sql", "0001init.sql", "abcd_init.sql", "0000_zero.sql", "_x.sql"}
	for _, name := range bad {
		if _, err := parseMigrationVersion(name); err == nil {
			t.Errorf("parseMigrationVersion(%q) 应报错", name)
		}
	}
}

// mustLoad 读取打包迁移（测试辅助）。
func mustLoad(t *testing.T) []migration {
	t.Helper()
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	return all
}

// assertTables 断言若干表存在。
func assertTables(ctx context.Context, st *Store, names ...string) error {
	for _, n := range names {
		ok, err := st.tableExists(ctx, n)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("缺表: " + n)
		}
	}
	return nil
}

var (
	_ = sql.ErrNoRows
	_ = context.Background
)
