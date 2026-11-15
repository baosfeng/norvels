package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// 对账的失败分支：两库任一侧读不动时必须报错，绝不返回一份「0 孤儿 / 0 悬空」的
// 干净报告。正常库里这些分支走不到，它们对应真实运维场景——只恢复了主库、正文库
// 文件被单独替换、或恢复过程中某张表的结构不完整。此时谎报干净会让健康卡片全绿，
// 而「明明缺正文却没人发现」正是从这里开始。
//
// 造这类现场的统一手法：把块表换成视图（读得动、写不动），或换成缺列的视图
// （表名还在，但某条查询的列取不到）。

// TestReconcileFailsWhenContentUnreadable 正文库读不动时对账报错。
func TestReconcileFailsWhenContentUnreadable(t *testing.T) {
	t.Run("正文库连接关闭", func(t *testing.T) {
		ctx := context.Background()
		st := newTestStore(t)
		requireOK(t, "关闭正文库", st.cdb.Close())
		if _, err := st.reconcileContent(ctx, false); err == nil {
			t.Fatal("正文库不可读时对账应报错")
		}
	})

	// 表名还在、但缺 id 列：孤儿侧数得出来，悬空侧那条读 id 的查询失败。
	t.Run("块表缺 id 列", func(t *testing.T) {
		ctx := context.Background()
		st := newTestStore(t)
		requireOK(t, "删块表", execOn(t, st.cdb, "DROP TABLE chapter_blocks"))
		requireOK(t, "建缺列视图", execOn(t, st.cdb,
			"CREATE VIEW chapter_blocks AS SELECT 1 AS book_id"))
		if _, err := st.reconcileContent(ctx, false); err == nil {
			t.Fatal("块表缺列时对账应报错")
		}
	})
}

// TestReconcileFailsWhenMainTablesMissing 主库缺表时对账报错（不把「读不到」当「没有」）。
func TestReconcileFailsWhenMainTablesMissing(t *testing.T) {
	for _, table := range []string{"books", "chapters"} {
		t.Run("缺 "+table, func(t *testing.T) {
			ctx := context.Background()
			st := newTestStore(t)
			saveOrFail(t, ctx, st, sampleBook("对账断链之书", "作者己", "/books/recon2.txt",
				"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"))
			requireOK(t, "删表 "+table, execOn(t, st.DB(), "DROP TABLE "+table))
			if _, err := st.reconcileContent(ctx, false); err == nil {
				t.Fatalf("主库缺 %s 时对账应报错", table)
			}
		})
	}
}

// TestReconcileRemoveOrphanFails 孤儿块数得出来却删不掉（块表是只读视图）→ 报错且原样保留。
//
// 这条对应「进程以只读方式挂载正文库」或「表被换成视图」：修复动作失败必须冒泡，
// 否则运维会以为已经清干净了，而下一次对账又报同样的数字。
func TestReconcileRemoveOrphanFails(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	orphan, err := st.SaveBook(ctx, sampleBook("待清孤儿", "作者辛", "/books/orphan2.txt",
		strings.Repeat("c", 64)), SaveOptions{})
	requireOK(t, "SaveBook", err)
	blocks := contentBlocksOf(t, ctx, st, orphan.BookID)
	if blocks == 0 {
		t.Fatal("样本不成立：孤儿书在正文库没有块")
	}
	dropMainBookRows(t, ctx, st, orphan.BookID)
	requireOK(t, "改写块表结构", makeBlocksReadOnly(t, st))

	if _, err := st.reconcileContent(ctx, true); err == nil {
		t.Fatal("块表不可写时 fix=true 应报错")
	}
	if n := contentBlocksOf(t, ctx, st, orphan.BookID); n != blocks {
		t.Errorf("删除失败后块数 = %d，期望原样保留 %d", n, blocks)
	}
}

// TestReconcileFailureMarksSnapshotStale 失败要记账：快照带出错误并标 stale。
//
// 驾驶舱据「快照有错误」把健康卡片标成 stale 并给出告警；失败若被静默吞掉，
// 运维看到的就是一份「从没对过账但一切正常」的数字。
func TestReconcileFailureMarksSnapshotStale(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	saveOrFail(t, ctx, st, sampleBook("对账失败之书", "作者庚", "/books/recon3.txt",
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
	requireOK(t, "Close", st.Close())

	if _, err := st.ReconcileContent(ctx, false); err == nil {
		t.Fatal("库关闭后对账应返回错误")
	}
	if _, stale, err := st.reconcileSnapshot(); err == nil || !stale {
		t.Fatalf("对账失败后快照 = stale:%v err:%v，期望 stale 且带错误", stale, err)
	}
	if content := st.HealthReport(ctx).Content; content == nil || !content.Stale {
		t.Errorf("对账失败后 content 段应标 stale：%+v", content)
	}
}

// makeBlocksReadOnly 把块表换成指向真实表的视图：读得动、写不动。
func makeBlocksReadOnly(t *testing.T, st *Store) error {
	t.Helper()
	if err := execOn(t, st.cdb, "ALTER TABLE chapter_blocks RENAME TO chapter_blocks_real"); err != nil {
		return err
	}
	return execOn(t, st.cdb,
		"CREATE VIEW chapter_blocks AS SELECT * FROM chapter_blocks_real")
}

// execOn 在一个库上执行一条 DDL（测试用；出错即失败）。
func execOn(t *testing.T, db *sql.DB, query string) error {
	t.Helper()
	_, err := db.ExecContext(context.Background(), query)
	return err
}
