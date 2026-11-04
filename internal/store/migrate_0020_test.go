package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 本文件锁死 0020「books 七列质量分 + 聚合口径 +1」的迁移契约：
//   - 七列都在、NOT NULL、默认 0 / ''（存量行天然是「未计算」）；
//   - metrics_version 前进（口径变更，旧缓存必然过期）；
//   - 幂等：删掉版本记录重跑不报 duplicate column、不改已有质量分。

// TestMigrate0020AddsQualityColumns 七列的形状与默认值。
func TestMigrate0020AddsQualityColumns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, col := range bookQualityColumns {
		var dflt string
		var notNull int
		if err := st.db.QueryRowContext(ctx,
			"SELECT COALESCE(dflt_value, ''), \"notnull\" FROM pragma_table_info('books') WHERE name = ?",
			col.name).Scan(&dflt, &notNull); err != nil {
			t.Fatalf("读列 %s: %v", col.name, err)
		}
		if notNull != 1 {
			t.Errorf("列 %s 不是 NOT NULL", col.name)
		}
		want := "0"
		if strings.HasSuffix(col.name, "_at") {
			want = "''"
		}
		if dflt != want {
			t.Errorf("列 %s 的默认值 = %q，期望 %q", col.name, dflt, want)
		}
	}
	// 断言的是「迁移跑完后的最终口径版本」：0020 把它推到 5，0022 又推到 6。
	// 断言 5 会让这条测试在每次口径 +1 时误报——它真正要看的是七列的形状，
	// 版本号只是「口径标志位确实前进了」的旁证，跟着 metricsVersion 走即可。
	if v := scalarInt(t, ctx, st, "SELECT value FROM store_metrics WHERE key = 'metrics_version'"); v != int64(metricsVersion) {
		t.Errorf("metrics_version = %d，期望 %d", v, metricsVersion)
	}
}

// TestMigrate0020Idempotent 重放 0020 不改数据、不报错。
func TestMigrate0020Idempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := withQuality(syntheticBook("重放之书", "/books/rep.txt", strings.Repeat("r", 64), qualityChapters(2)),
		model.QualityMetrics{ChapterCount: 2, TotalChars: 600, NoiseRatio: 0.2, StructureScore: 0.7})
	res := saveOrFail(t, ctx, st, book)

	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 20"); err != nil {
		t.Fatalf("清版本记录: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("重跑迁移: %v", err)
	}
	got, err := st.bookQualityByID(ctx, res.BookID)
	if err != nil {
		t.Fatalf("bookQualityByID: %v", err)
	}
	if got.NoiseRatio != 0.2 || !got.Computed() {
		t.Errorf("重放后质量分 = %+v，期望保留原值", got)
	}
	if ok, err := st.qualityColumnsReady(ctx); err != nil || !ok {
		t.Errorf("重放后质量列应仍在（ok=%v err=%v）", ok, err)
	}
}
