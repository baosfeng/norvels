package store

import (
	"context"
	"testing"
)

// TestMigrateRebuildsLegacyStoreMetrics 复现并锁死「CREATE TABLE IF NOT EXISTS 无法改表结构」的陷阱。
//
// 历史事故：开发期存在过一版 store_metrics (id, text_bytes, index_bytes, chapters_at, computed_at)。
// 0004 早期版本的建表语句用了 IF NOT EXISTS，遇到同名旧表直接跳过，新结构从未建上，
// 于是每次写 metrics 都报 "table store_metrics has no column named key"，
// /api/stats 的正文/索引体积恒为 0。
//
// 修复方式：0005 迁移先 DROP 再 CREATE（store_metrics 全是可重算的派生值，删了无害）。
// 本测试保证这个修复不会被后续改动悄悄退回去。
func TestMigrateRebuildsLegacyStoreMetrics(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st) // 有数据才能验证重算出的正文非 0
	// 换成开发期的旧结构，并把版本退回到 4（模拟「已应用过 0004 旧版」的库）。
	installLegacyStoreMetrics(t, ctx, st)
	regressToVersion(t, ctx, st, 4)

	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("升级: %v", err)
	}
	assertMetricsTableShape(t, ctx, st)
	assertMetricsWritable(t, ctx, st)
}

// installLegacyStoreMetrics 把 store_metrics 换成开发期的旧结构并塞一行旧数据。
func installLegacyStoreMetrics(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	ddl := []string{
		"DROP TABLE IF EXISTS store_metrics",
		"CREATE TABLE store_metrics (" +
			"id INTEGER PRIMARY KEY CHECK (id = 1), " +
			"text_bytes INTEGER NOT NULL DEFAULT 0, " +
			"index_bytes INTEGER NOT NULL DEFAULT 0, " +
			"chapters_at INTEGER NOT NULL DEFAULT 0, " +
			"computed_at TEXT NOT NULL DEFAULT '')",
		"INSERT INTO store_metrics(id, text_bytes, index_bytes, chapters_at, computed_at) " +
			"VALUES (1, 999, 888, 0, '')",
	}
	for _, q := range ddl {
		if _, err := st.DB().ExecContext(ctx, q); err != nil {
			t.Fatalf("构造旧结构: %v", err)
		}
	}
}

// regressToVersion 把库版本退回 target（模拟旧代码迁移过的库）。
func regressToVersion(t *testing.T, ctx context.Context, st *Store, target int) {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE version > ?", target); err != nil {
		t.Fatalf("回退版本记录: %v", err)
	}
}

// assertMetricsTableShape 校验升级后 store_metrics 是新结构（有 key 列、没有旧的 id 列）。
func assertMetricsTableShape(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	hasKey := columnExists(t, ctx, st, "store_metrics", "key")
	if !hasKey {
		t.Fatalf("升级后 store_metrics 仍缺 key 列（IF NOT EXISTS 陷阱未修复）")
	}
	if columnExists(t, ctx, st, "store_metrics", "chapters_at") {
		t.Errorf("升级后仍残留旧的 chapters_at 列")
	}
}

// assertMetricsWritable 校验升级后能正常写 metrics（旧结构下这一步必然失败）。
func assertMetricsWritable(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("升级后写 metrics: %v", err)
	}
	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if snap.TextBytes <= 0 {
		t.Errorf("重算后 TextBytes = %d，应为正", snap.TextBytes)
	}
}

// columnExists 判断表上是否有某列。
func columnExists(t *testing.T, ctx context.Context, st *Store, table, column string) bool {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("扫描列名: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

// TestMetricsRebuiltTableTriggersRecompute 校验表重建后 stale 判据能让后台重算重新触发。
//
// 事故二：重建表时迁移会写入 metrics_version=1，与当前口径版本相等。
// 若 stale 只看版本号，重建后的库会被永远判为「有效」，text_bytes 恒为 0 且永不重算。
func TestMetricsRebuiltTableTriggersRecompute(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	// 模拟迁移重建：清空表后只写 metrics_version（聚合值还没算）。
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM store_metrics"); err != nil {
		t.Fatalf("清空: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO store_metrics(key, value, updated_at) VALUES ('metrics_version', 1, '')"); err != nil {
		t.Fatalf("写版本: %v", err)
	}

	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if !snap.Stale {
		t.Errorf("只写版本号、未算聚合值时应判为 stale")
	}
	comp, err := st.Composition(ctx)
	if err != nil {
		t.Fatalf("Composition: %v", err)
	}
	if !comp.Estimated {
		t.Errorf("stale 时 Composition.Estimated 应为 true")
	}
}

// TestMetricsEmptyDatabaseNotStale 校验真正的空库（无章节）不算 stale。
func TestMetricsEmptyDatabaseNotStale(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if snap.Stale {
		t.Errorf("空库重算后不应 stale")
	}
}
