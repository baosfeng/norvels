package store

import (
	"context"
	"testing"
)

// 0025 的口径：书级健康计数（缺简介 / 缺作者 / 超薄书 / 章节数分桶）由 SQL 触发器维护，
// 报告读预计算、明细读实时 SQL，两条路径必须恒等；迁移只建结构、可重入；
// 旧库（未跑 0025 或回填未完成）降级到实时 SQL，数字仍然正确。

// TestMigrate0025Idempotent 重跑迁移不再应用，触发器不重复建，状态行不被重置。
func TestMigrate0025Idempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("第二次 MigrateTo: %v", err)
	}
	var triggers int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND "+
			"(name LIKE 'books_health_%' OR name LIKE 'authors_health_%')").Scan(&triggers); err != nil {
		t.Fatalf("数触发器: %v", err)
	}
	if triggers != 7 {
		t.Errorf("维护触发器 = %d 个，期望 7", triggers)
	}
	var complete int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT complete FROM book_health_state WHERE id = 1").Scan(&complete); err != nil {
		t.Fatalf("读状态行: %v", err)
	}
	if complete != 1 {
		t.Errorf("空库迁移后状态 complete = %d，期望 1（无需回填）", complete)
	}
}

// TestMigrate0025SkipsLegacySchema 老库（没有 books 表）上步骤不报错（只是什么都不做）。
func TestMigrate0025SkipsLegacySchema(t *testing.T) {
	ctx := context.Background()
	st := openBareOnly(t)
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // 只读探测
	if err := createBookHealthCountStep(ctx, tx); err != nil {
		t.Fatalf("无 books 表时步骤应直接返回：%v", err)
	}
}

// TestBookHealthCountsMatchesLiveSQL 触发器维护值与实时 SQL 恒等（插入 / 改列 / 删除）。
func TestBookHealthCountsMatchesLiveSQL(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	assertBookHealthCountsMatchLive(t, ctx, st)

	// 改简介 / 改作者 / 改章节数 / 删书：每一步之后都必须仍然恒等。
	for _, stmt := range []string{
		"UPDATE books SET intro = '  ' WHERE id = 1",             // 只剩空白 → 变成缺简介
		"UPDATE books SET author_id = 0 WHERE id = 2",            // 0 → 缺作者
		"UPDATE books SET chapter_count = 1 WHERE id = 3",        // 掉进超薄 + 换桶
		"UPDATE books SET chapter_count = 2000 WHERE id = 1",     // 换桶
		"DELETE FROM books WHERE id = 4",                         // 删书
		"INSERT INTO authors(id, name) VALUES (900001, '示例作者补')", // 不存在的作者出现
		"UPDATE books SET author_id = 900001 WHERE id = 2",
		"DELETE FROM authors WHERE id = 900001", // 作者又消失 → 变回缺作者
	} {
		if _, err := st.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("执行 %q: %v", stmt, err)
		}
		assertBookHealthCountsMatchLive(t, ctx, st)
	}
}

// TestReportMatchesDetailTotals 报告的三个书级指标与明细接口 total 恒等。
func TestReportMatchesDetailTotals(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	rep := st.HealthReport(ctx)
	for kind, want := range map[string]*int{
		HealthIssueMissingIntro:  rep.MissingIntro,
		HealthIssueMissingAuthor: rep.MissingAuthor,
		HealthIssueThinBook:      rep.ThinBooks,
	} {
		issues, err := st.HealthIssues(ctx, kind, 1, 20)
		if err != nil {
			t.Fatalf("HealthIssues(%s): %v", kind, err)
		}
		if want == nil {
			t.Fatalf("%s 报告值为 nil", kind)
		}
		if issues.Total != *want {
			t.Errorf("%s：报告 %d、明细 total %d（口径漂移）", kind, *want, issues.Total)
		}
	}
}

// TestReportDegradesWithoutCountsTable 未迁移 / 回填未完成时：回退实时 SQL，数字仍对。
func TestReportDegradesWithoutCountsTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedHealthBooks(t, ctx, st)
	stale, ok := st.bookHealthCounts(ctx)
	if !ok || stale.MissingIntro < 0 {
		t.Fatalf("预计算读数异常: %+v ok=%v", stale, ok)
	}
	// 标记未完成：读取侧必须走实时 SQL（慢但正确），且不报错。
	if _, err := st.DB().ExecContext(ctx, "UPDATE book_health_state SET complete = 0"); err != nil {
		t.Fatalf("标记未完成: %v", err)
	}
	if _, ok := st.bookHealthCounts(ctx); ok {
		t.Fatal("状态未完成时不该返回预计算值")
	}
	rep := st.HealthReport(ctx)
	if len(rep.Warnings) != 0 {
		t.Fatalf("降级路径不该有告警: %v", rep.Warnings)
	}
	if rep.MissingIntro == nil || rep.MissingAuthor == nil || rep.ThinBooks == nil {
		t.Fatalf("降级后指标缺失: missingIntro=%v missingAuthor=%v thin=%v",
			rep.MissingIntro, rep.MissingAuthor, rep.ThinBooks)
	}
}

// seedHealthBooks 造 5 本覆盖各种判据的书（简介空 / 空白 / 有内容，作者 0 / 悬空 / 正常，章节数跨桶）。
func seedHealthBooks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	for i := 1; i <= 5; i++ {
		b := syntheticBook("健康之书"+string(rune('0'+i)), "/path/to/txt/health-"+string(rune('0'+i))+".txt",
			"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"+string(rune('0'+i)),
			syntheticChapters(2))
		if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
			t.Fatalf("SaveBook: %v", err)
		}
	}
	for _, stmt := range []string{
		"UPDATE books SET intro = '' WHERE id = 1",
		"UPDATE books SET intro = '   ' WHERE id = 2",
		"UPDATE books SET author_id = 0 WHERE id = 3",
		"UPDATE books SET author_id = 999999 WHERE id = 4",
		"UPDATE books SET chapter_count = 1 WHERE id = 5",
	} {
		if _, err := st.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("执行 %q: %v", stmt, err)
		}
	}
}

// assertBookHealthCountsMatchLive 断言预计算的 9 个数字与实时 SQL 逐一相等。
func assertBookHealthCountsMatchLive(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	counts, ok := st.bookHealthCounts(ctx)
	if !ok {
		t.Fatal("预计算值不可用（状态应为 complete）")
	}
	live := map[string]string{
		"missing_intro":  "SELECT COUNT(*) FROM books WHERE TRIM(intro) = ''",
		"missing_author": "SELECT COUNT(*) FROM books WHERE " + whereMissingAuthor,
		"thin_books":     "SELECT COUNT(*) FROM books WHERE " + thinBookWhere("chapter_count"),
	}
	got := map[string]int{
		"missing_intro": counts.MissingIntro, "missing_author": counts.MissingAuthor,
		"thin_books": counts.ThinBooks,
	}
	for kind, query := range live {
		var want int
		if err := st.DB().QueryRowContext(ctx, query).Scan(&want); err != nil {
			t.Fatalf("实时 %s: %v", kind, err)
		}
		if got[kind] != want {
			t.Fatalf("%s：预计算 %d、实时 %d（触发器口径漂移）", kind, got[kind], want)
		}
	}
	liveBuckets, err := bucketCountsOn(ctx, st.DB(), "chapter_count", "books", chapterBucketEdges)
	if err != nil {
		t.Fatalf("实时分桶: %v", err)
	}
	for i := range liveBuckets {
		if counts.Buckets[i] != liveBuckets[i] {
			t.Fatalf("第 %d 桶：预计算 %d、实时 %d", i, counts.Buckets[i], liveBuckets[i])
		}
	}
}

// openBareOnly 打开一个只有 SQLite 文件、没有任何业务表的老库（迁移步骤要能在它上面安全跑）。
func openBareOnly(t *testing.T) *Store {
	t.Helper()
	st, _ := openBare(t, "bare-0025.db")
	return st
}
