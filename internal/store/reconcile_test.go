package store

import (
	"context"
	"strings"
	"testing"
)

// 本文件锁死正文对账（ReconcileContent）：两库之间没有事务，漂移只有这两类——
// 孤儿块（主库没这本书）与悬空引用（章节指着不存在的块）。对账必须能分别数出来，
// 并且只在 fix=true 时删孤儿块（悬空引用无法凭空补出正文，只能报告）。

// TestReconcileContentFindsOrphanBlocks 孤儿块：主库提交成功、正文库清理失败的那一半。
func TestReconcileContentFindsOrphanBlocks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	keep, err := st.SaveBook(ctx, sampleBook("留下的书", "作者甲", "/books/keep.txt", strings.Repeat("k", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook keep: %v", err)
	}
	orphan, err := st.SaveBook(ctx, sampleBook("孤儿书", "作者乙", "/books/orphan.txt", strings.Repeat("o", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook orphan: %v", err)
	}
	orphanBlocks := contentBlocksOf(t, ctx, st, orphan.BookID)
	if orphanBlocks == 0 {
		t.Fatal("孤儿书在正文库没有块，用例样本不成立")
	}
	// 只删主库那一侧，模拟「主库提交成功、正文库清理失败」。
	dropMainBookRows(t, ctx, st, orphan.BookID)

	assertOrphanReport(t, mustReconcile(t, st, false), orphanBlocks)

	fixed := mustReconcile(t, st, true)
	if fixed.RemovedBlocks != orphanBlocks {
		t.Errorf("删除块数 = %d，期望 %d", fixed.RemovedBlocks, orphanBlocks)
	}
	if n := contentBlocksOf(t, ctx, st, orphan.BookID); n != 0 {
		t.Errorf("清理后仍残留 %d 个孤儿块", n)
	}
	if n := contentBlocksOf(t, ctx, st, keep.BookID); n == 0 {
		t.Error("清理孤儿块时误删了正常书的块")
	}
	if again := mustReconcile(t, st, false); again.OrphanBlocks != 0 {
		t.Errorf("清理后再对账仍有 %d 个孤儿块", again.OrphanBlocks)
	}
}

// mustReconcile 跑一次对账，出错即失败。
func mustReconcile(t *testing.T, st *Store, fix bool) ContentReconcileReport {
	t.Helper()
	rep, err := st.ReconcileContent(context.Background(), fix)
	if err != nil {
		t.Fatalf("ReconcileContent(fix=%v): %v", fix, err)
	}
	return rep
}

// assertOrphanReport 断言「1 本书的孤儿块、没有悬空引用、且 fix=false 不删东西」。
func assertOrphanReport(t *testing.T, rep ContentReconcileReport, orphanBlocks int) {
	t.Helper()
	if rep.OrphanBooks != 1 || rep.OrphanBlocks != orphanBlocks || rep.DanglingRefs != 0 {
		t.Fatalf("对账结果 = %+v，期望 1 本 / %d 块 / 0 悬空", rep, orphanBlocks)
	}
	if rep.RemovedBlocks != 0 {
		t.Errorf("fix=false 不该删任何块，实际删除 %d", rep.RemovedBlocks)
	}
}

// TestReconcileContentFindsDanglingRef 悬空引用：章节指向正文库里不存在的块。
func TestReconcileContentFindsDanglingRef(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, err := st.SaveBook(ctx, sampleBook("断链的书", "作者丙", "/books/broken.txt", strings.Repeat("b", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	chapterID := firstChapterID(t, ctx, st, res.BookID)
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE chapters SET block_id = 987654 WHERE id = ?", chapterID); err != nil {
		t.Fatalf("制造悬空引用: %v", err)
	}

	rep, err := st.ReconcileContent(ctx, false)
	if err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	if rep.DanglingRefs != 1 {
		t.Errorf("悬空引用 = %d，期望 1", rep.DanglingRefs)
	}
	if rep.OrphanBlocks != 0 {
		t.Errorf("孤儿块 = %d，期望 0（这次只制造了悬空引用）", rep.OrphanBlocks)
	}
	// fix 只删孤儿块，悬空引用不在修复范围内（缺的正文无法凭空造出来）。
	fixed, err := st.ReconcileContent(ctx, true)
	if err != nil {
		t.Fatalf("ReconcileContent(true): %v", err)
	}
	if fixed.DanglingRefs != 1 || fixed.RemovedBlocks != 0 {
		t.Errorf("fix 后 = %+v，期望仍报 1 条悬空且不删块", fixed)
	}
}

// TestHealthReportIncludesContent 健康报告带上正文库对账摘要。
func TestHealthReportIncludesContent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.SaveBook(ctx, sampleBook("体检的书", "作者丁", "/books/h.txt", strings.Repeat("h", 64)), SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	// 同步跑一次对账把快照填上（请求路径只读快照，见 health_refresh.go）。
	if _, err := st.ReconcileContent(ctx, false); err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	r := st.HealthReport(ctx)
	if len(r.Warnings) != 0 {
		t.Fatalf("健康报告有告警: %v", r.Warnings)
	}
	if r.Content == nil {
		t.Fatal("健康报告缺少正文库摘要")
	}
	if r.Content.Stale || r.Content.CheckedAt == "" {
		t.Errorf("对账之后快照应新鲜且带 CheckedAt: %+v", r.Content)
	}
	if r.Content.OrphanBlocks != 0 || r.Content.DanglingRefs != 0 || r.Content.MissingTable {
		t.Errorf("正文库摘要异常: %+v", r.Content)
	}
}

// dropMainBookRows 只删主库那一侧的书记录（正文库的块不动），模拟跨库半失败。
func dropMainBookRows(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	for _, stmt := range []string{
		"DELETE FROM chapters WHERE book_id = ?",
		"DELETE FROM volumes WHERE book_id = ?",
		"DELETE FROM books WHERE id = ?",
	} {
		if _, err := st.db.ExecContext(ctx, stmt, bookID); err != nil {
			t.Fatalf("删主库记录（%s）: %v", stmt, err)
		}
	}
}
