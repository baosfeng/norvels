package main

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/store"
)

// tooNewVersion 是「库由更新版本迁移过」时库里的版本号：远高于当前打包最高版本。
const tooNewVersion = 99

// seedTooNewDB 造一个「已被更新版本迁移过」的库：schema_migrations 记 99、user_version=99。
func seedTooNewDB(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "toonew.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	seed := []string{
		"CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT '')",
		"INSERT INTO schema_migrations (version) VALUES (" + strconv.Itoa(tooNewVersion) + ")",
		"PRAGMA user_version = " + strconv.Itoa(tooNewVersion),
	}
	for _, q := range seed {
		if _, err := st.DB().ExecContext(ctx, q); err != nil {
			t.Fatalf("造过新库 %q: %v", q, err)
		}
	}
	return path
}

// assertGuardRejects 断言 err 是版本守卫的拒绝（可用 errors.Is 判哨兵），且提示能指路。
func assertGuardRejects(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("库版本过新时必须拒绝启动，却放行了")
	}
	if !errors.Is(err, store.ErrSchemaTooNew) {
		t.Fatalf("错误未分类为 ErrSchemaTooNew: %v", err)
	}
	for _, want := range []string{"数据库由更新版本", "当前二进制只支持到", "请升级镜像，或从备份恢复"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("提示缺关键要素 %q: %v", want, err)
		}
	}
}

// assertDBStillTooNew 断言拒绝路径没往库里写结构：版本仍 99，且没被建出业务表。
func assertDBStillTooNew(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("回读库: %v", err)
	}
	defer st.Close()
	var version int
	if err := st.DB().QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("读版本: %v", err)
	}
	if version != tooNewVersion {
		t.Errorf("schema_migrations 最大版本 = %d，期望 %d（守卫必须只读）", version, tooNewVersion)
	}
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE name = 'books'").Scan(&n); err != nil {
		t.Fatalf("读表清单: %v", err)
	}
	if n != 0 {
		t.Error("拒绝路径不该建出 books 表（守卫必须在任何 DDL 之前）")
	}
}

// TestServeStartupPathRejectsTooNewSchema 覆盖 serve 启动路径：
// cmdServe(main.go:151) → openStore(main.go:101) → migrateAndLog(main.go:117) → MigrateTo 守卫。
func TestServeStartupPathRejectsTooNewSchema(t *testing.T) {
	path := seedTooNewDB(t)
	st, err := openStore(context.Background(), path)
	if st != nil {
		st.Close()
		t.Fatal("库版本过新时 openStore 不该返回可用 Store")
	}
	assertGuardRejects(t, err)
	assertDBStillTooNew(t, path)
}

// TestMigrateStartupPathRejectsTooNewSchema 覆盖 migrate 启动路径：
// migrate_cmd.go:23 store.Open → MigrateTo 守卫；错误原样透出，不包成「迁移失败」。
func TestMigrateStartupPathRejectsTooNewSchema(t *testing.T) {
	path := seedTooNewDB(t)
	err := cmdMigrate([]string{"--db", path})
	assertGuardRejects(t, err)
	if strings.Contains(err.Error(), "迁移失败（") {
		t.Errorf("守卫拒绝被包装成迁移失败: %v", err)
	}
	assertDBStillTooNew(t, path)
}

// TestMigrateDryRunReportsGuard 覆盖 --dry-run 的只读诊断：
// 先摆出「库版本 / 二进制支持到」两个数，再按守卫口径报错。
func TestMigrateDryRunReportsGuard(t *testing.T) {
	path := seedTooNewDB(t)
	var err error
	out := captureStdout(t, func() { err = cmdMigrate([]string{"--db", path, "--dry-run"}) })
	assertGuardRejects(t, err)
	for _, want := range []string{
		"当前版本: " + strconv.Itoa(tooNewVersion),
		"当前二进制支持到: " + strconv.Itoa(store.LatestVersion()),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run 输出缺 %q:\n%s", want, out)
		}
	}
	assertDBStillTooNew(t, path)
}

// TestOpenStoreAtLatestLogsZeroMigrations 覆盖「库版本 = 二进制版本」的启动：
// 第二次启动应用 0 个迁移，日志里应出现「应用 0 个迁移」，且不逐条打印「已应用」。
func TestOpenStoreAtLatestLogsZeroMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "latest.db")
	first, err := openStore(ctx, path)
	if err != nil {
		t.Fatalf("首次启动: %v", err)
	}
	first.Close()

	logText := captureLog(t)
	second, err := openStore(ctx, path)
	if err != nil {
		t.Fatalf("已是最新版本时不该启动失败: %v", err)
	}
	defer second.Close()
	out := logText()
	if !strings.Contains(out, "应用 0 个迁移") {
		t.Errorf("日志未显示「应用 0 个迁移」: %s", out)
	}
	if strings.Contains(out, "  已应用 ") {
		t.Errorf("无迁移可应用时不该逐条打印「已应用」: %s", out)
	}
}
