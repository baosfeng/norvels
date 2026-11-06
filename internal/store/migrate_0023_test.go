package store

import (
	"context"
	"testing"
)

// 0023 的口径（部署安全版）：迁移**只建表 + 落状态行**，历史指纹的回填由启动后的
// 后台任务分批补——生产的 chapter_fingerprints 有 5125 万行，把它塞进迁移等于让升级多停服几分钟。
// 回填未完成期间，健康报告要给「可用的数字 + stale + backfilling」，不能 500、
// 也不能把「还没补到」读成「这些书都没有指纹」。

// TestMigrate0023CreatesTableOnly 迁移步骤本身不扫指纹表：只建表、只落「待回填」状态。
func TestMigrate0023CreatesTableOnly(t *testing.T) {
	ctx := context.Background()
	st := newTestStoreTo(t, 22)
	backfillBook(t, ctx, st, 1)
	backfillBook(t, ctx, st, 2)
	// 模拟「0023 之前那版代码建出来的库」：指纹行齐全，但没有覆盖表、也没有状态行。
	for _, stmt := range []string{
		"DROP TABLE " + fingerprintBooksTable,
		"DELETE FROM " + coverageStateTable,
	} {
		if _, err := st.cdb.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("制造「未迁移」现场（%s）: %v", stmt, err)
		}
	}

	// 直接跑 0023 的步骤（tx 参数本步骤不用，跨库 DDL 在正文库）。
	if err := createFingerprintCoverageStep(ctx, nil, st); err != nil {
		t.Fatalf("createFingerprintCoverageStep: %v", err)
	}
	cover, err := st.fingerprintCoverageCount(ctx)
	if err != nil {
		t.Fatalf("fingerprintCoverageCount: %v", err)
	}
	if cover != 0 {
		t.Errorf("迁移步骤不该回填（覆盖表 %d 行）；5125 万行的扫描必须留给后台任务", cover)
	}
	complete, lastID, err := st.coverageState(ctx)
	if err != nil {
		t.Fatalf("coverageState: %v", err)
	}
	if complete || lastID != 0 {
		t.Errorf("状态行 = (complete=%v, last=%d)，期望待回填", complete, lastID)
	}
	// 指纹行一行都不能被迁移动过。
	if got := fingerprintRows(t, ctx, st, 1); got != 4 {
		t.Errorf("书 1 指纹行 = %d，期望 4", got)
	}
}

// TestBackfillFingerprintCoverageFills 后台回填把历史指纹补进覆盖表，并把状态置完成。
func TestBackfillFingerprintCoverageFills(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := []int64{backfillBook(t, ctx, st, 1), backfillBook(t, ctx, st, 2), backfillBook(t, ctx, st, 3)}
	// 制造「历史库」现场：指纹在、覆盖表空、状态待回填。
	if _, err := st.cdb.ExecContext(ctx, "DELETE FROM "+fingerprintBooksTable); err != nil {
		t.Fatalf("清覆盖表: %v", err)
	}
	if err := st.setCoverageState(ctx, false, 0); err != nil {
		t.Fatalf("置待回填: %v", err)
	}

	rep, err := st.BackfillFingerprintCoverage(ctx)
	if err != nil {
		t.Fatalf("BackfillFingerprintCoverage: %v", err)
	}
	if rep.Added != len(ids) || rep.Covered != len(ids) {
		t.Errorf("回填 = %+v，期望补 %d 本", rep, len(ids))
	}
	assertCoverageMatchesFull(t, ctx, st)
	if complete, lastID, err := st.coverageState(ctx); err != nil || !complete {
		t.Errorf("回填后状态 = (complete=%v, last=%d, err=%v)，期望完成", complete, lastID, err)
	}
	// 再跑一次：幂等，不重复插入。
	again, err := st.BackfillFingerprintCoverage(ctx)
	if err != nil {
		t.Fatalf("第二次 BackfillFingerprintCoverage: %v", err)
	}
	if again.Added != 0 {
		t.Errorf("重跑又补了 %d 本（应为 0）", again.Added)
	}
}

// TestHealthReportReportsBackfilling 回填未完成时：数字可用、stale、backfilling 说清状态。
func TestHealthReportReportsBackfilling(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	backfillBook(t, ctx, st, 1)
	if err := st.setCoverageState(ctx, false, 0); err != nil {
		t.Fatalf("置待回填: %v", err)
	}
	if _, err := st.cdb.ExecContext(ctx, "DELETE FROM "+fingerprintBooksTable); err != nil {
		t.Fatalf("清覆盖表: %v", err)
	}

	fp := st.HealthReport(ctx).Fingerprints
	if fp == nil {
		t.Fatal("缺少 fingerprints 段")
	}
	if !fp.Stale {
		t.Error("回填未完成时必须标 stale（否则缺失数会被读成真实缺口）")
	}
	if fp.Hint == "" {
		t.Error("回填未完成时应给出 hint，说明这是后台在补、无需人工重跑")
	}

	// 后台补完之后：不再 stale，缺口归零。
	if _, err := st.BackfillFingerprintCoverage(ctx); err != nil {
		t.Fatalf("BackfillFingerprintCoverage: %v", err)
	}
	fp = st.HealthReport(ctx).Fingerprints
	if fp.Stale || fp.Missing != 0 || fp.Fingerprinted != 1 {
		t.Errorf("回填完成后 = %+v，期望 1 本全覆盖且不 stale", fp)
	}
}

// TestMigrate0023Idempotent 重跑迁移不再应用；重算也不该需要修补。
func TestMigrate0023Idempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	backfillBook(t, ctx, st, 1)

	status, err := st.MigrateTo(ctx, LatestVersion())
	if err != nil {
		t.Fatalf("MigrateTo: %v", err)
	}
	if len(status.Applied) != 0 || !status.AlreadyAtMax {
		t.Errorf("第二次迁移应用了 %d 个（AlreadyAtMax=%v），期望 0", len(status.Applied), status.AlreadyAtMax)
	}
	rep, err := st.RepairFingerprintCoverage(ctx)
	if err != nil {
		t.Fatalf("RepairFingerprintCoverage: %v", err)
	}
	if rep.Added != 0 || rep.Pruned != 0 || rep.Covered != 1 {
		t.Errorf("重算 = %+v，期望覆盖 1 本且无需修补", rep)
	}
}

// TestHealthReportDegradesWithoutCoverageTable 旧库（未跑 0023）上健康报告降级：有数、标 stale、无告警。
func TestHealthReportDegradesWithoutCoverageTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	backfillBook(t, ctx, st, 1)
	if _, err := st.cdb.ExecContext(ctx, "DROP TABLE "+fingerprintBooksTable); err != nil {
		t.Fatalf("删覆盖表（模拟未迁移）: %v", err)
	}

	rep := st.HealthReport(ctx)
	if len(rep.Warnings) != 0 {
		t.Fatalf("降级路径不该有告警: %v", rep.Warnings)
	}
	fp := rep.Fingerprints
	if fp == nil {
		t.Fatal("缺少 fingerprints 段")
	}
	if fp.Fingerprinted != 1 || fp.Missing != 0 || fp.Books != 1 {
		t.Errorf("降级覆盖数 = %+v，期望 1 本全覆盖", fp)
	}
	if !fp.Stale {
		t.Error("降级用全表 COUNT(DISTINCT) 时必须标 stale，否则这一次 20 秒的扫描没人看得见")
	}
}
