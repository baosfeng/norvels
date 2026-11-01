package store

import (
	"context"
	"testing"
)

// 本文件锁死 0018「正文块搬进独立库」的迁移契约：
//   - 搬完之后块只在正文库，主库的表被 DROP；
//   - 逐本正文指纹不变（块 id 与字节都原样保留，chapters.block_id 仍然指得着）；
//   - 幂等：中途失败留下的半份拷贝会被覆盖而不是翻倍，重跑整条迁移不改任何块。

// TestMigrate0018MovesBlocksOutOfMain 迁移后块只在正文库，正文逐本不变。
func TestMigrate0018MovesBlocksOutOfMain(t *testing.T) {
	st, ctx, books := seedLegacyBlocks(t, true)
	// 先停在 0017：0018 之前块还在主库，这是「搬迁的输入形态」。
	if _, err := st.MigrateTo(ctx, 17); err != nil {
		t.Fatalf("迁移到 0017: %v", err)
	}
	before := digestsByBook(t, ctx, st, books)
	counts := blocksPerBook(t, ctx, st)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("迁移到最新版本: %v", err)
	}
	assertContentOwnsBlocks(t, ctx, st)
	assertBlockCountsUnchanged(t, ctx, st, counts)
	assertDigestsEqual(t, ctx, st, before)
}

// assertContentOwnsBlocks 断言主库没有块表、正文库有块表。
func assertContentOwnsBlocks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if exists, err := st.tableExists(ctx, "chapter_blocks"); err != nil || exists {
		t.Fatalf("主库应已 DROP chapter_blocks（exists=%v err=%v）", exists, err)
	}
	if ok, err := st.contentTableExists(ctx); err != nil || !ok {
		t.Fatalf("正文库应有 chapter_blocks（ok=%v err=%v）", ok, err)
	}
}

// assertBlockCountsUnchanged 逐本比对块数（块 id 原样保留，章节引用才不会断）。
func assertBlockCountsUnchanged(t *testing.T, ctx context.Context, st *Store, want map[int64]int) {
	t.Helper()
	got := blocksPerBook(t, ctx, st)
	if len(got) != len(want) {
		t.Fatalf("有块的书数变了：%d → %d", len(want), len(got))
	}
	for id, n := range want {
		if got[id] != n {
			t.Errorf("书 %d 块数 %d → %d", id, n, got[id])
		}
	}
}

// TestMigrate0018Idempotent 重复执行不重复搬：已搬完（主库无表）再跑一次是空操作。
func TestMigrate0018Idempotent(t *testing.T) {
	st, ctx, _ := seedLegacyBlocks(t, true)
	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("首次迁移: %v", err)
	}
	first := blockSnapshot(t, ctx, st)
	if len(first) == 0 {
		t.Fatal("迁移后正文库块表为空，断言会退化为恒真")
	}

	// 重放 0018（模拟「删掉版本记录重跑」）：主库已无块表 → 步骤直接返回。
	err := st.applyMigration(ctx, migration{
		Version: 99,
		Name:    "0099_replay_0018.sql",
		SQL:     mustMigrationSQL(t, "0018_content_db.sql"),
		Steps:   nil,
	})
	if err != nil {
		t.Fatalf("重放 0018: %v", err)
	}
	assertSnapshotEqual(t, blockSnapshot(t, ctx, st), first, "重放 0018")
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 99"); err != nil {
		t.Fatalf("清掉重放版本记录: %v", err)
	}
}

// TestMigrate0018ResumesAfterPartialCopy 半路中断（正文库已有半份拷贝）后重跑不翻倍。
//
// 模拟方式：手工把前若干块先写进正文库，再跑整条迁移。插入用 INSERT OR REPLACE
// 按 id 覆盖，所以结果是「补齐剩余」而不是「同一批块出现两份」。
func TestMigrate0018ResumesAfterPartialCopy(t *testing.T) {
	st, ctx, books := seedLegacyBlocks(t, true)
	before := digestsByBook(t, ctx, st, books)
	if _, err := st.MigrateTo(ctx, 17); err != nil {
		t.Fatalf("迁移到 0017: %v", err)
	}
	counts := blocksPerBook(t, ctx, st)
	total := 0
	for _, n := range counts {
		total += n
	}
	if total < 4 {
		t.Fatalf("0017 之后只有 %d 个块，半份拷贝的用例造不出来", total)
	}
	copied := copyBlocksToContent(t, ctx, st, 3)
	if copied == 0 || copied >= total {
		t.Fatalf("半份拷贝行数 %d 不成立（总块 %d）", copied, total)
	}

	if _, err := st.MigrateTo(ctx, LatestVersion()); err != nil {
		t.Fatalf("迁移到最新版本: %v", err)
	}
	after := blockSnapshot(t, ctx, st)
	if len(after) != total {
		t.Errorf("块数 = %d，期望 %d（半份拷贝被重复搬了）", len(after), total)
	}
	assertDigestsEqual(t, ctx, st, before)
}

// copyBlocksToContent 手工把主库前 n 个块抄进正文库（模拟迁移中途被杀）。
func copyBlocksToContent(t *testing.T, ctx context.Context, st *Store, n int) int {
	t.Helper()
	if err := st.ensureContentSchema(ctx); err != nil {
		t.Fatalf("建正文库表: %v", err)
	}
	rows, err := st.db.QueryContext(ctx,
		"SELECT id, book_id, seq, first_idx, last_idx, raw_bytes, packed FROM chapter_blocks "+
			"ORDER BY id LIMIT ?", n)
	if err != nil {
		t.Fatalf("读主库块: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	count := 0
	for rows.Next() {
		var (
			id, bookID             int64
			seq, firstIdx, lastIdx int
			raw                    int
			packed                 []byte
		)
		if err := rows.Scan(&id, &bookID, &seq, &firstIdx, &lastIdx, &raw, &packed); err != nil {
			t.Fatalf("扫描主库块: %v", err)
		}
		if _, err := st.cdb.ExecContext(ctx,
			"INSERT OR REPLACE INTO chapter_blocks(id, book_id, seq, first_idx, last_idx, raw_bytes, packed) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?)", id, bookID, seq, firstIdx, lastIdx, raw, packed); err != nil {
			t.Fatalf("写正文库块 %d: %v", id, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历主库块: %v", err)
	}
	return count
}
