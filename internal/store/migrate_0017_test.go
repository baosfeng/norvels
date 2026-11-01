package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// 本文件锁死 0017「存量块重压成 8 MB / 0x03」的升级路径。
//
// 造库方式刻意**手工**写入 1 MB / 0x02 的老块，不借 SaveBook：写入侧代码已经改成
// 8 MB / 0x03 了，只有照 0007 时代的口径自己造，才能证明「老块真的会被重压」。
//
// 三条不可退让的断言：
//  1. 每本书的 BookContentDigest 迁移前后逐本相等（正文一个字节都没变）；
//  2. 迁完再没有 0x01/0x02 块，多章块 raw_bytes 都在 8 MB 目标内；
//  3. 坏块必须让整次迁移失败并点名 book_id + block_seq，版本号不前进。

// TestMigrate0017RepacksLegacyBlocks 是本次迁移最关键的一条：正文逐字节不变 + 块形态换新。
func TestMigrate0017RepacksLegacyBlocks(t *testing.T) {
	st, ctx, books := seedLegacyBlocks(t, true)
	beforeDigests := digestsByBook(t, ctx, st, books)
	beforeBlocks := blocksPerBook(t, ctx, st)
	if beforeBlocks[books[0]] < 8 {
		t.Fatalf("大书迁移前只有 %d 个块，造库失败（应接近 9）", beforeBlocks[books[0]])
	}
	if got := scalarIntBlock(t, ctx, st,
		"SELECT COUNT(*) FROM chapter_blocks WHERE substr(packed,1,1) = x'02'"); got < 8 {
		t.Fatalf("老格式 0x02 块只有 %d 个，造库失败", got)
	}

	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("升级到 0017: %v", err)
	}

	assertNoLegacyAlgo(t, ctx, st)
	assertBlockShape(t, ctx, st)
	afterBlocks := blocksPerBook(t, ctx, st)
	if afterBlocks[books[0]] >= beforeBlocks[books[0]] {
		t.Errorf("大书块数没下降：迁移前 %d，迁移后 %d", beforeBlocks[books[0]], afterBlocks[books[0]])
	}
	if afterBlocks[books[0]] != 2 {
		t.Errorf("大书（8.6 MB）按 8 MB 目标应切成 2 块，实际 %d 块", afterBlocks[books[0]])
	}
	assertDigestsEqual(t, ctx, st, beforeDigests)

	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	packed, raw := assertBlockBytesMatchMetrics(t, ctx, st)
	t.Logf("大书块数 %d → %d；有块的书 %d 本；packed %d 字节 / 原文 %d 字节（%.1f%%）",
		beforeBlocks[books[0]], afterBlocks[books[0]], len(afterBlocks),
		packed, raw, 100*float64(packed)/float64(raw))
	// 0018 把块表搬进正文库后口径再 +1（v4），所以这里断言的是最终版本。
	if v := scalarInt(t, ctx, st, "SELECT value FROM store_metrics WHERE key = 'metrics_version'"); v != 4 {
		t.Errorf("metrics_version = %d，期望 4（0018 拆库后口径）", v)
	}
}

// TestMigrate0017SecondRunIsNoOp 验证幂等：整跑两次 + 直接重放步骤，块表一字节不动。
func TestMigrate0017SecondRunIsNoOp(t *testing.T) {
	st, ctx, _ := seedLegacyBlocks(t, true)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("首次迁移: %v", err)
	}
	first := blockSnapshot(t, ctx, st)
	if len(first) == 0 {
		t.Fatal("迁移后块表为空，断言会退化为恒真")
	}

	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("第二次迁移: %v", err)
	}
	assertSnapshotEqual(t, blockSnapshot(t, ctx, st), first, "第二次 Migrate")

	// 直接重放 0017（模拟「删掉版本记录重跑」）：步骤里的幂等判定应让库形态保持不变。
	err := st.applyMigration(ctx, migration{
		Version: 99,
		Name:    "0099_replay_0017.sql",
		SQL:     mustMigrationSQL(t, "0017_codebook_blocks.sql"),
		Steps:   migrationStepsByVersion[17],
	})
	if err != nil {
		t.Fatalf("重放 0017: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 99"); err != nil {
		t.Fatalf("清掉重放版本记录: %v", err)
	}
	assertSnapshotEqual(t, blockSnapshot(t, ctx, st), first, "重放 0017")
	if exists, err := st.tableExists(ctx, newBlocksTable); err != nil || exists {
		t.Errorf("重放后不应残留 %s（err=%v）", newBlocksTable, err)
	}
}

// TestMigrate0017FailsOnCorruptBlock 验证坏块路径：报出 book_id + seq，整体回滚。
func TestMigrate0017FailsOnCorruptBlock(t *testing.T) {
	st, ctx, books := seedLegacyBlocks(t, true)
	corruptBlock := scalarIntBlock(t, ctx, st,
		"SELECT id FROM chapter_blocks WHERE book_id = ? AND seq = 2", books[0])
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE chapter_blocks SET packed = x'02deadbeef' WHERE id = ?", corruptBlock); err != nil {
		t.Fatalf("注入坏块: %v", err)
	}
	// 快照取「注入坏块之后、迁移之前」的库形态：回滚后必须原样（连坏块字节都不动）。
	before := blockSnapshot(t, ctx, st)

	err := st.Migrate(ctx)
	if err == nil {
		t.Fatal("坏块必须让迁移失败")
	}
	assertBlockErrorMentions(t, err, books[0], 2)
	assertMigrationRolledBack(t, ctx, st, 16, before)
}

// TestMigrate0017FailsOnSpanMismatch 验证「章节区间越出块」同样被当成块级损坏报出来。
func TestMigrate0017FailsOnSpanMismatch(t *testing.T) {
	st, ctx, books := seedLegacyBlocks(t, false)
	before := blockSnapshot(t, ctx, st)
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE chapters SET block_len = block_len + 4096 WHERE book_id = ? AND idx = 0", books[0]); err != nil {
		t.Fatalf("篡改章节区间: %v", err)
	}

	err := st.Migrate(ctx)
	if err == nil {
		t.Fatal("区间越界必须让迁移失败")
	}
	assertBlockErrorMentions(t, err, books[0], 1)
	assertMigrationRolledBack(t, ctx, st, 16, before)
}

// TestMigrate0017FailsOnCompressError 覆盖「重压某一块失败」：整次迁移回滚，版本不前进。
func TestMigrate0017FailsOnCompressError(t *testing.T) {
	st, ctx, _ := seedLegacyBlocks(t, false)
	before := blockSnapshot(t, ctx, st)
	restore := stubCompressFailure(t)
	err := st.Migrate(ctx)
	restore()
	if err == nil {
		t.Fatal("压缩失败必须让迁移失败")
	}
	if !strings.Contains(err.Error(), "compress block seq") {
		t.Errorf("错误信息应点名压缩失败的块序号: %v", err)
	}
	assertMigrationRolledBack(t, ctx, st, 16, before)
}

// TestMigrate0017FailsOnBlocklessChapter 覆盖「章节有长度却没块归属」这种损坏形态。
func TestMigrate0017FailsOnBlocklessChapter(t *testing.T) {
	st, ctx, books := seedLegacyBlocks(t, false)
	before := blockSnapshot(t, ctx, st)
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE chapters SET block_id = 0, block_off = 0, block_len = 7 WHERE book_id = ? AND idx = 0",
		books[0]); err != nil {
		t.Fatalf("篡改章节块归属: %v", err)
	}
	err := st.Migrate(ctx)
	if err == nil {
		t.Fatal("章节无块却有长度必须让迁移失败")
	}
	if !strings.Contains(err.Error(), "no block") ||
		!strings.Contains(err.Error(), fmt.Sprintf("book %d:", books[0])) {
		t.Errorf("错误信息应点名书与章节: %v", err)
	}
	assertMigrationRolledBack(t, ctx, st, 16, before)
}

// TestRebuildStepGuards 覆盖步骤的两条前置探测：目标表缺失、现役块表缺失。
func TestRebuildStepGuards(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "guards.db")
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("开事务: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // 探测用事务，结束即回滚

	// 目标表缺失（迁移 SQL 没执行）：必须报错点名，不能静默跳过。
	if err := rebuildBlocksForCodebook(ctx, tx); err == nil || !strings.Contains(err.Error(), newBlocksTable) {
		t.Errorf("目标表缺失时 %s 应报错并点名该表，实际 %v", newBlocksTable, err)
	}
	// 目标表在、现役块表不在：没有可重编码的数据，视为无需重建并正常返回。
	if _, err := tx.ExecContext(ctx, "CREATE TABLE "+newBlocksTable+" (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("建目标表: %v", err)
	}
	if err := rebuildBlocksForCodebook(ctx, tx); err != nil {
		t.Errorf("现役块表缺失时应直接返回，实际 %v", err)
	}
	if exists, err := tableExistsInTx(ctx, tx, newBlocksTable); err != nil || exists {
		t.Errorf("无需重建时应删掉目标表（err=%v）", err)
	}
}
