package store

import (
	"context"
	"strings"
	"testing"
)

// 本文件锁死 0019「正文库建章级指纹表 + 预置去重阈值」的迁移契约：
//   - 表在**正文库**、WITHOUT ROWID、主键 (hash, book_id)；
//   - 按书删的索引存在；按 hash 查走主键；
//   - 幂等：删掉版本记录重跑不改数据，也不会把用户改过的阈值打回缺省。

// TestMigrate0019CreatesFingerprintTable 迁移后正文库有指纹表，主库一行都不该有。
func TestMigrate0019CreatesFingerprintTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if ok, err := st.fingerprintTableExists(ctx); err != nil || !ok {
		t.Fatalf("正文库应有 chapter_fingerprints（ok=%v err=%v）", ok, err)
	}
	var ddl string
	if err := st.cdb.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'chapter_fingerprints'").Scan(&ddl); err != nil {
		t.Fatalf("读建表语句: %v", err)
	}
	for _, want := range []string{"WITHOUT ROWID", "PRIMARY KEY (hash, book_id)", "bytes   INTEGER NOT NULL"} {
		if !strings.Contains(ddl, want) {
			t.Errorf("建表语句缺少 %q：\n%s", want, ddl)
		}
	}
	// 主库侧不该有这张表（跨库 DDL 只落在正文库）。
	if exists, err := st.tableExists(ctx, "chapter_fingerprints"); err != nil || exists {
		t.Errorf("主库不该有 chapter_fingerprints（exists=%v err=%v）", exists, err)
	}
}

// assertThresholdSetting 断言阈值设置项的值（设置项不存在也算失败）。
func assertThresholdSetting(t *testing.T, st *Store, want string) {
	t.Helper()
	raw, ok, err := st.GetSetting(context.Background(), SettingKeyDedupeThreshold)
	if err != nil || !ok || raw != want {
		t.Fatalf("阈值设置 = (%q,%v,%v)，期望 %q", raw, ok, err, want)
	}
}

// TestMigrate0019SeedsThresholdAndIsIdempotent 阈值预置一次，重跑不覆盖用户改过的值。
func TestMigrate0019SeedsThresholdAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	assertThresholdSetting(t, st, "0.95")

	if err := st.PutSetting(ctx, SettingKeyDedupeThreshold, "0.5"); err != nil {
		t.Fatalf("改阈值: %v", err)
	}
	// 重放 0019（模拟「删掉版本记录重跑」）。
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 19"); err != nil {
		t.Fatalf("清版本记录: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("重跑迁移: %v", err)
	}
	assertThresholdSetting(t, st, "0.5")
	if ok, err := st.fingerprintTableExists(ctx); err != nil || !ok {
		t.Errorf("重跑后指纹表应仍在（ok=%v err=%v）", ok, err)
	}
}

// TestDedupeThresholdSetting 阈值读取的口径：缺省 0.95，越界与非法值报错并回落缺省。
func TestDedupeThresholdSetting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		value   string
		remove  bool
		want    float64
		wantErr bool
	}{
		{name: "未配置", remove: true, want: DefaultDedupeThreshold},
		{name: "合法值", value: "0.5", want: 0.5},
		{name: "边界 1", value: "1", want: 1},
		{name: "百分数写法", value: "95", wantErr: true, want: DefaultDedupeThreshold},
		{name: "零", value: "0", wantErr: true, want: DefaultDedupeThreshold},
		{name: "非数字", value: "high", wantErr: true, want: DefaultDedupeThreshold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			if tc.remove {
				if _, err := st.DB().ExecContext(ctx, "DELETE FROM settings WHERE key = ?", SettingKeyDedupeThreshold); err != nil {
					t.Fatalf("清设置项: %v", err)
				}
			} else if err := st.PutSetting(ctx, SettingKeyDedupeThreshold, tc.value); err != nil {
				t.Fatalf("写设置项: %v", err)
			}
			got, err := st.DedupeThreshold(ctx)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DedupeThreshold err = %v，期望报错=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("DedupeThreshold = %v，期望 %v", got, tc.want)
			}
		})
	}
}
