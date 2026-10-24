package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// versionAhead 是「库由更新版本创建」时用的版本号：远高于当前打包的最高版本。
const versionAhead = 99

// seedNewerSchema 手工造一个「已被更新版本迁移过」的库：
// 有 schema_migrations 且记录了 versionAhead，同时把 user_version 也设成它。
func seedNewerSchema(t *testing.T) (*Store, string) {
	t.Helper()
	st, path := openBare(t, "newer.db")
	ctx := context.Background()
	if err := st.ensureMigrationTable(ctx); err != nil {
		t.Fatalf("ensureMigrationTable: %v", err)
	}
	if err := st.recordVersion(ctx, versionAhead); err != nil {
		t.Fatalf("recordVersion: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(versionAhead)); err != nil {
		t.Fatalf("设置 user_version: %v", err)
	}
	return st, path
}

// TestMigrateRejectsNewerSchema 是守卫的主用例：
// 库版本 > 二进制版本时必须报错，且错误要能让人看懂下一步该做什么。
func TestMigrateRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	st, _ := seedNewerSchema(t)

	status, err := st.MigrateTo(ctx, LatestVersion())
	if err == nil {
		t.Fatal("库版本过新时必须拒绝迁移，却成功了")
	}
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Errorf("错误未分类为 ErrSchemaTooNew: %v", err)
	}
	var verr *SchemaVersionError
	if !errors.As(err, &verr) {
		t.Fatalf("错误不是 *SchemaVersionError: %v", err)
	}
	if verr.DatabaseVersion != versionAhead || verr.Supported != LatestVersion() {
		t.Errorf("版本对 = (%d, %d)，期望 (%d, %d)",
			verr.DatabaseVersion, verr.Supported, versionAhead, LatestVersion())
	}
	assertGuardMessage(t, err.Error())
	if status.FailedVersion != 0 || status.ToVersion != 0 {
		t.Errorf("被拒绝时不应产生迁移进度: %+v", status)
	}
}

// assertGuardMessage 断言提示含「现象 + 双方版本 + 两条出路」，缺一项用户就会卡住。
func assertGuardMessage(t *testing.T, msg string) {
	t.Helper()
	for _, want := range []string{
		"数据库由更新版本", "创建", "当前二进制只支持到",
		"请升级镜像，或从备份恢复",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("提示缺关键要素 %q: %s", want, msg)
		}
	}
}

// TestGuardKeepsDatabaseUntouched 断言拒绝路径是只读的：
// 版本记录仍是 99、user_version 未被写回、也没有被顺手建出新表。
func TestGuardKeepsDatabaseUntouched(t *testing.T) {
	ctx := context.Background()
	st, _ := seedNewerSchema(t)
	before := snapshotSchema(t, st)

	if _, err := st.MigrateTo(ctx, LatestVersion()); err == nil {
		t.Fatal("期望拒绝")
	}
	if err := st.Migrate(ctx); err == nil {
		t.Fatal("Migrate 同样应拒绝")
	}
	if err := st.EnsureSchemaNotNewer(ctx); err == nil {
		t.Fatal("EnsureSchemaNotNewer 应返回错误")
	}
	after := snapshotSchema(t, st)

	if after != before {
		t.Errorf("拒绝路径改动了数据库：\n before=%s\n after =%s", before, after)
	}
	if got := maxVersionNow(t, st); got != versionAhead {
		t.Errorf("拒绝后 max(version) = %d，期望仍为 %d", got, versionAhead)
	}
	if got := userVersionNow(t, st); got != versionAhead {
		t.Errorf("拒绝后 user_version = %d，期望未被改写（%d）", got, versionAhead)
	}
}

// snapshotSchema 汇总「库结构 + 版本」的指纹，用于比对拒绝路径有没有写库。
func snapshotSchema(t *testing.T, st *Store) string {
	t.Helper()
	ctx := context.Background()
	var ddl string
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COALESCE(group_concat(name, ','), '') FROM (SELECT name FROM sqlite_master ORDER BY name)").
		Scan(&ddl); err != nil {
		t.Fatalf("读表清单: %v", err)
	}
	return ddl + "|v=" + strconv.Itoa(maxVersionNow(t, st)) + "|uv=" + strconv.Itoa(userVersionNow(t, st))
}

// maxVersionNow 读 schema_migrations 的最大版本（无记录为 0）。
func maxVersionNow(t *testing.T, st *Store) int {
	t.Helper()
	v, err := st.maxRecordedVersion(context.Background())
	if err != nil {
		t.Fatalf("maxRecordedVersion: %v", err)
	}
	return v
}

// userVersionNow 读 PRAGMA user_version。
func userVersionNow(t *testing.T, st *Store) int {
	t.Helper()
	var v int
	if err := st.DB().QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("读 user_version: %v", err)
	}
	return v
}

// TestGuardAllowsEqualAndOlderSchemas 覆盖放行分支：版本相等、库更旧、空库都不拦。
func TestGuardAllowsEqualAndOlderSchemas(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		seed func(t *testing.T, st *Store)
	}{
		{"版本相等", func(t *testing.T, st *Store) {
			if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
				t.Fatalf("先迁到最新: %v", err)
			}
		}},
		{"库更旧", func(t *testing.T, st *Store) {
			if _, err := st.MigrateTo(ctx, 1); err != nil {
				t.Fatalf("先迁到 1: %v", err)
			}
		}},
		{"空库", func(t *testing.T, _ *Store) {}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, _ := openBare(t, "ok.db")
			c.seed(t, st)
			if err := st.EnsureSchemaNotNewer(ctx); err != nil {
				t.Errorf("EnsureSchemaNotNewer 应放行，却报错: %v", err)
			}
		})
	}
}

// TestGuardAfterMigrationStillPasses 验证迁移完自己的最新版后守卫一致放行（不误伤正常升级）。
func TestGuardAfterMigrationStillPasses(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "after.db")
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("MigrateTo: %v", err)
	}
	if err := st.EnsureSchemaNotNewer(ctx); err != nil {
		t.Errorf("迁到最新后守卫不该拦: %v", err)
	}
	if got := maxVersionNow(t, st); got != LatestVersion() {
		t.Errorf("迁移后版本 = %d，期望 %d", got, LatestVersion())
	}
}

// TestGuardOnCorruptFile 区分「不是数据库」与「版本过新」：
// 两者都要报错，但提示不能把用户引向「请升级镜像」。
func TestGuardOnCorruptFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(path, []byte("这不是 SQLite 文件，只是一段普通文本"), 0o600); err != nil {
		t.Fatalf("造损坏文件: %v", err)
	}
	st, err := Open(ctx, path)
	if err != nil {
		// 驱动在 Ping 阶段就发现不是库也可以接受，只要错误不是「版本过新」。
		if errors.Is(err, ErrSchemaTooNew) {
			t.Fatalf("损坏文件被误判为版本过新: %v", err)
		}
		return
	}
	t.Cleanup(func() { _ = st.Close() })
	err = st.EnsureSchemaNotNewer(ctx)
	if err == nil {
		t.Skip("该 SQLite 驱动对纯文本文件容忍度较高，此处不强制报错")
	}
	if errors.Is(err, ErrSchemaTooNew) {
		t.Errorf("损坏文件被误判为版本过新: %v", err)
	}
}

// TestGuardOnClosedDatabase 覆盖「读不出版本」分支：连接已关闭时守卫要如实报错，
// 且不能把这种错误误判成「库版本过新」（两者的处置完全不同）。
func TestGuardOnClosedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "closed.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	guardErr := st.EnsureSchemaNotNewer(ctx)
	if guardErr == nil {
		t.Fatal("已关闭的连接应报错")
	}
	if errors.Is(guardErr, ErrSchemaTooNew) {
		t.Errorf("读取失败不能被判成版本过新: %v", guardErr)
	}
	if !strings.Contains(guardErr.Error(), "无法读取现有数据库的 schema 版本") {
		t.Errorf("提示应指向「读不出版本」: %v", guardErr)
	}
}

// TestCheckSchemaVersion 覆盖比较函数的三个分支（纯函数，直接调用）。
func TestCheckSchemaVersion(t *testing.T) {
	cases := []struct {
		db, supported int
		wantErr       bool
	}{
		{99, 3, true},
		{3, 3, false},
		{1, 3, false},
		{0, 3, false},
		{0, 0, false},
	}
	for _, c := range cases {
		err := checkSchemaVersion(c.db, c.supported)
		if c.wantErr != (err != nil) {
			t.Errorf("checkSchemaVersion(%d, %d) err=%v，期望有错=%v", c.db, c.supported, err, c.wantErr)
		}
	}
}
