package store

import (
	"context"
	"testing"
)

// 本文件锁死 0021「AI 审核表 + books.superseded_by」的迁移契约：
//   - 表与索引都在，列默认值符合 reviews.go 的扫描顺序（status 默认 pending）；
//   - books.superseded_by 是 NOT NULL DEFAULT 0（存量书一律按当前版本处理）；
//   - settings 预置 ai.enabled = false；
//   - 幂等：删掉版本记录重跑不报 duplicate column / table already exists，也不改已有数据。

// TestMigrate0021CreatesReviewTable 表、索引、列与预置开关的形状。
func TestMigrate0021CreatesReviewTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, col := range []string{"id", "job_id", "new_book_id", "old_book_id", "hit_ratio",
		"status", "ai_winner", "ai_confidence", "ai_reasons", "rule_reasons",
		"created_at", "decided_at", "decided_by"} {
		if !columnExistsAt(t, ctx, st, "import_reviews", col) {
			t.Fatalf("import_reviews 缺列 %s", col)
		}
	}
	if !columnExistsAt(t, ctx, st, "books", "superseded_by") {
		t.Fatalf("books 缺 superseded_by")
	}
	if n := scalarInt(t, ctx, st,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_import_reviews_status_created'"); n != 1 {
		t.Errorf("状态索引不存在（找到 %d 条）", n)
	}
	if got := scalarText(t, ctx, st, "SELECT value FROM settings WHERE key = 'ai.enabled'"); got != "false" {
		t.Errorf("ai.enabled = %q，期望预置 false", got)
	}
	// 默认值：status 默认 pending、superseded_by 默认 0。
	if got := scalarText(t, ctx, st,
		"SELECT COALESCE((SELECT dflt_value FROM pragma_table_info('import_reviews') WHERE name='status'), '')"); got != "'pending'" {
		t.Errorf("status 默认值 = %q，期望 'pending'", got)
	}
}

// TestMigrate0021Idempotent 重放 0021 不改已有数据、不报错。
func TestMigrate0021Idempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	oldID, newID := reviewPair(t, ctx, st)
	reviewID := mustCreateReview(t, ctx, st, oldID, newID)
	// 用户手工打开过开关：重放不该把它打回 false。
	if err := st.PutSetting(ctx, SettingKeyAIEnabled, "true"); err != nil {
		t.Fatalf("打开开关: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 21"); err != nil {
		t.Fatalf("清版本记录: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("重跑迁移: %v", err)
	}
	if !st.AIEnabled(ctx) {
		t.Errorf("重放后 ai.enabled 被打回 false")
	}
	if got := supersededOf(t, ctx, st, newID); got != oldID {
		t.Errorf("重放后新书 superseded_by = %d，期望保留 %d", got, oldID)
	}
	if _, err := st.GetImportReview(ctx, reviewID); err != nil {
		t.Errorf("重放后审核记录丢失: %v", err)
	}
}

// columnExistsAt 探测某表是否有某列（读 pragma_table_info，不开事务：_txlock=immediate
// 会让并发开的第二个事务直接等锁）。
func columnExistsAt(t *testing.T, ctx context.Context, st *Store, table, column string) bool {
	t.Helper()
	var n int
	if err := st.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&n); err != nil {
		t.Fatalf("探测列 %s.%s: %v", table, column, err)
	}
	return n > 0
}
