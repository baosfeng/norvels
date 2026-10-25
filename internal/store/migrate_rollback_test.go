package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// 本文件收拢「迁移失败必须整体回滚」的验证。
//
// 两种失败面各一条：
//   - SQL 本身有语法错误（迁移文件写坏）→ 前面的 DDL 不能留下；
//   - SQL 成功但 Go 步骤失败（0006/0007 的条件 DDL 与数据搬迁）→ 建表与版本记录一起回滚。

// TestMigrationRollsBackWhenStepFails 验证「SQL 已执行、Go 步骤失败」也整体回滚。
//
// 这是 0006 引入 migration.Steps 后新增的风险点：SQLite 没有条件 DDL，
// 删列只能放在 Go 步骤里。若步骤与 SQL 不在同一事务，就会出现
// 「SQL 提交成功、版本号已前进、列却没删掉」的半迁移库。
func TestMigrationRollsBackWhenStepFails(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "stepfail.db")
	if err := st.ensureMigrationTable(ctx); err != nil {
		t.Fatalf("ensureMigrationTable: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "CREATE TABLE probe_keepers (id INTEGER)"); err != nil {
		t.Fatalf("建前置表: %v", err)
	}

	bad := migration{
		Version: 98,
		Name:    "0098_step_broken.sql",
		SQL:     "DROP TABLE probe_keepers;",
		Steps: []migrationStep{{
			Name: "intentionally broken step",
			Run: func(context.Context, *sql.Tx) error {
				return errors.New("步骤故意失败")
			},
		}},
	}
	err := st.applyMigration(ctx, bad)
	if err == nil {
		t.Fatal("步骤失败应返回错误")
	}
	if !strings.Contains(err.Error(), "intentionally broken step") {
		t.Errorf("错误信息应点名失败的步骤: %v", err)
	}

	// SQL 里的 DROP 必须被回滚：表还在。
	if exists, _ := st.tableExists(ctx, "probe_keepers"); !exists {
		t.Error("步骤失败后 SQL 的 DROP TABLE 未回滚")
	}
	v, err := st.maxRecordedVersion(ctx)
	if err != nil {
		t.Fatalf("maxRecordedVersion: %v", err)
	}
	if v != 0 {
		t.Errorf("失败后版本 = %d，期望 0（版本不应前进）", v)
	}
}

// TestMigrate0007RollsBackOnCorruptBlock 验证 0007 搬迁失败时整体回滚。
//
// 0007 是唯一一条「改数据」的迁移：它把正文搬进压缩块再删 content 列。
// 若搬迁中途失败却留下半迁移状态（有的书进了块、有的还是原文），库既读不出
// 完整正文，也回不到旧结构。本测试用一个会失败的压缩器模拟这一场景。
func TestMigrate0007RollsBackOnCorruptBlock(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "v6-broken.db")
	if _, err := st.MigrateTo(ctx, 6); err != nil {
		t.Fatalf("迁到版本 6: %v", err)
	}
	insertLegacyBook(t, ctx, st)
	insertPreBlockChapters(t, ctx, st)

	beforeCount := countRow(t, ctx, st, "SELECT COUNT(*) FROM chapters")
	beforeFirst := scalarText(t, ctx, st, "SELECT content FROM chapters ORDER BY id LIMIT 1")

	// 注入一个必然失败的块压缩：让 splitBlocks 之后的压缩步骤报错。
	restore := stubCompressFailure(t)

	err := st.applyMigration(ctx, migration{
		Version: 7,
		Name:    "0007_block_storage.sql",
		SQL:     mustMigrationSQL(t, "0007_block_storage.sql"),
		Steps:   migrationStepsByVersion[7],
	})
	restore()
	if err == nil {
		t.Fatal("搬迁失败应返回错误")
	}

	// 库必须停在版本 6 的样子：有 content 列、章节数与内容不变、无块数据。
	if !columnExists(t, ctx, st, "chapters", "content") {
		t.Error("回滚后 chapters.content 列不应被删除")
	}
	assertCountRow(t, ctx, st, "SELECT COUNT(*) FROM chapters", beforeCount, "章节数")
	if got := scalarText(t, ctx, st, "SELECT content FROM chapters ORDER BY id LIMIT 1"); got != beforeFirst {
		t.Errorf("回滚后首章正文变了：%q", truncateForLog(got))
	}
	// 连 chapter_blocks 的建表都要回滚（它在 0007 的 SQL 里创建）。
	exists, err := st.tableExists(ctx, "chapter_blocks")
	if err != nil {
		t.Fatalf("查块表: %v", err)
	}
	if exists {
		t.Errorf("回滚后 chapter_blocks 表仍存在（建表未被回滚）")
	}
	v, err := st.maxRecordedVersion(ctx)
	if err != nil {
		t.Fatalf("读版本: %v", err)
	}
	if v != 6 {
		t.Errorf("回滚后版本 = %d，期望 6（版本不应前进）", v)
	}
}

// scalarText 执行单值文本查询。
func scalarText(t *testing.T, ctx context.Context, st *Store, query string) string {
	t.Helper()
	var v string
	if err := st.DB().QueryRowContext(ctx, query).Scan(&v); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return v
}

// mustMigrationSQL 取出打包的迁移 SQL 原文（测试要手工重放某条迁移）。
func mustMigrationSQL(t *testing.T, name string) string {
	t.Helper()
	for _, m := range mustLoad(t) {
		if m.Name == name {
			return m.SQL
		}
	}
	t.Fatalf("打包里没有迁移 %q", name)
	return ""
}
