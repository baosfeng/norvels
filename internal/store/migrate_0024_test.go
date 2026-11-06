package store

import (
	"context"
	"strings"
	"testing"
)

// 0024 的验收口径：
//   - 五条查询索引都要建出来（含 /api/categories 的覆盖索引兜底）；
//   - 重放（删版本号重跑）必须幂等，不报错也不重复建；
//   - 老库缺列时跳过对应索引，不能让整次迁移失败。

// queryIndexNames 是 0024 负责的索引名。
var queryIndexNames = []string{
	"idx_books_active_id",
	"idx_books_title_sorted",
	"idx_books_rating",
	"idx_books_series_trim",
	"idx_books_category",
}

func indexCount(t *testing.T, st *Store, name string) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&n); err != nil {
		t.Fatalf("查索引 %s: %v", name, err)
	}
	return n
}

// TestMigration0024CreatesQueryIndexes 迁移到最新版本后五条索引都在（且各一条）。
func TestMigration0024CreatesQueryIndexes(t *testing.T) {
	st := newTestStore(t)
	for _, name := range queryIndexNames {
		if n := indexCount(t, st, name); n != 1 {
			t.Errorf("索引 %s 数量 = %d，期望 1", name, n)
		}
	}
}

// TestMigration0024Idempotent 建索引步骤可以重放：第二次不报错，索引数量不变。
func TestMigration0024Idempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("开事务: %v", err)
	}
	n, err := createBookQueryIndexes(ctx, tx)
	if err != nil {
		t.Fatalf("重放建索引: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("提交: %v", err)
	}
	if n != len(queryIndexNames) {
		t.Errorf("重放创建 %d 条，期望 %d 条", n, len(queryIndexNames))
	}
	for _, name := range queryIndexNames {
		if got := indexCount(t, st, name); got != 1 {
			t.Errorf("重放后索引 %s 数量 = %d，期望 1", name, got)
		}
	}
}

// TestMigration0024SkipsMissingColumns 老库（只有 id/title）上缺列的索引直接跳过，不报错。
func TestMigration0024SkipsMissingColumns(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "legacy24.db")
	if _, err := st.DB().ExecContext(ctx,
		"CREATE TABLE books (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL)"); err != nil {
		t.Fatalf("建老库 books: %v", err)
	}
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("开事务: %v", err)
	}
	n, err := createBookQueryIndexes(ctx, tx)
	if err != nil {
		t.Fatalf("老库建索引不该失败: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("提交: %v", err)
	}
	if n != 0 {
		t.Errorf("老库缺列时应建 0 条索引，实际 %d 条", n)
	}
}

// TestListBooksTitleSortAvoidsTempBTree sort=title 必须走 idx_books_title_sorted，
// 不能退回「全表扫 + 临时 B-TREE 排序」（生产 16.9 万本时那是 0.5 s 级的退化）。
func TestListBooksTitleSortAvoidsTempBTree(t *testing.T) {
	st := newTestStore(t)
	q := "SELECT " + bookSummaryColumns + " FROM books b LEFT JOIN authors a ON a.id = b.author_id " +
		"WHERE b.superseded_by = 0 ORDER BY " + bookOrder("title") + " LIMIT 20"
	if plan := queryPlan(t, st, q); !strings.Contains(plan, "idx_books_title_sorted") ||
		strings.Contains(plan, "TEMP B-TREE") {
		t.Errorf("sort=title 未走索引: %s", plan)
	}
}

// TestCountBooksUsesCoveringIndex 无关键词的 total 走部分索引覆盖扫描，不再全表扫。
func TestCountBooksUsesCoveringIndex(t *testing.T) {
	st := newTestStore(t)
	plan := queryPlan(t, st, "SELECT COUNT(*) FROM books b WHERE b.superseded_by = 0")
	if !strings.Contains(plan, "idx_books_active_id") {
		t.Errorf("COUNT 未走 idx_books_active_id: %s", plan)
	}
}

// queryPlan 返回一条 SQL 的 EXPLAIN QUERY PLAN 文本（每行 detail 用 ; 连接）。
func queryPlan(t *testing.T, st *Store, sql string) string {
	t.Helper()
	rows, err := st.DB().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+sql)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var (
			id, parent, notused int
			detail              string
		)
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("扫描计划行: %v", err)
		}
		parts = append(parts, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("计划行: %v", err)
	}
	return strings.Join(parts, "; ")
}
