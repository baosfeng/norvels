package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// 0024 的步骤在**本文件里**自注册，而不是加进 migrate_steps.go 的 migrationStepsByVersion：
// 那张表同时被 0023（另一路任务）修改，集中登记会让两条并行的迁移抢同一处改动。
// 若以后要收回到集中登记，删掉下面这段 init、把同一个 migrationStep 写进
// migrationStepsByVersion[24] 即可，行为完全一样。
func init() {
	migrationStepsByVersion[24] = []migrationStep{{
		Name: "books 查询索引",
		Run:  addBookQueryIndexesStep,
	}}
}

// 0024：列表 / 搜索 / 随机 / 系列接口的查询索引。
//
// 全部索引都是**部分索引**（WHERE superseded_by = 0），而不是把 superseded_by 放进
// 复合索引首列。两种写法都能消掉 ORDER BY 的临时 B-TREE，但复合索引会让优化器对
// 「按别的列排序」的查询也挑它、再补一次全表排序——17 万本实测 sort=updated 从
// 0.6 ms 退化到 84 ms、sort=words 从 2.8 ms 退化到 117 ms（计划里出现
// USE TEMP B-TREE FOR ORDER BY）。部分索引只在查询里出现 superseded_by = 0 时参与
// 规划，且不带首列等值前缀，原有 idx_books_updated / idx_books_words /
// idx_books_chapters / idx_books_imported 仍被选中。
//
// 写入代价：导入是批量写，每本书多维护 4 条索引项（17 万本合计约 14 MB）；在 169,230
// 行上建这 4 条索引实测合计 0.32 s，相对「一章一行」的 chapters 写入可忽略。
func addBookQueryIndexesStep(ctx context.Context, tx *sql.Tx) error {
	t0 := time.Now()
	created, err := createBookQueryIndexes(ctx, tx)
	if err != nil {
		return err
	}
	log.Printf("迁移 0024：已建 %d 条书目查询索引，耗时 %v", created, time.Since(t0).Round(time.Millisecond))
	return nil
}

// bookQueryIndex 是一条索引的声明：建它需要的列 + DDL。
//
// 把「需要的列」写成数据而不是逐个 if：老库缺哪一列只影响到用到那一列的索引，
// 其余索引照建；新增索引时也只加一行。
type bookQueryIndex struct {
	name    string   // 索引名（写日志 / 出错定位用）
	columns []string // books 上必须存在的列
	ddl     string
}

// bookQueryIndexes 是 0024 要建的索引（顺序即创建顺序）。
//
// 每条索引对应的慢查询（生产 16.9 万本实测）：
//   - idx_books_active_id：COUNT(*) WHERE superseded_by = 0（列表/搜索/统计的 total）、
//     /api/stats 的书数、随机取书的 id 区间与「>= 随机 id 的第一本」。
//     没有它时是全表扫 books（约 130 MB），每次 ~0.10 s。
//   - idx_books_title_sorted：ORDER BY title COLLATE NOCASE, id（sort=title）。原有
//     idx_books_title 是 BINARY 排序，NOCASE 用不上，计划是 SCAN + TEMP B-TREE。
//     末列 author_id 让「关键词同时匹配书名与作者」的 COUNT 走覆盖索引，不必回表。
//   - idx_books_rating：ORDER BY rating DESC, updated_at DESC, id DESC（sort=rating）。
//   - idx_books_series_trim：GROUP BY TRIM(series)（/api/series）。表达式列 + 部分谓词，
//     只装「有系列名且是当前版本」的书（17 万本里只占一小部分）。
//   - idx_books_category：/api/categories 的覆盖索引。0015 的步骤已经建过，这里再声明
//     一次是给「升级路径上漏建」的库兜底；缺它时 GROUP BY 会变成全表扫 + 临时表。
var bookQueryIndexes = []bookQueryIndex{
	{
		name:    "idx_books_active_id",
		columns: []string{"superseded_by", "id"},
		ddl:     "CREATE INDEX IF NOT EXISTS idx_books_active_id ON books(id) WHERE superseded_by = 0",
	},
	{
		name:    "idx_books_title_sorted",
		columns: []string{"superseded_by", "title", "id", "author_id"},
		ddl: "CREATE INDEX IF NOT EXISTS idx_books_title_sorted " +
			"ON books(title COLLATE NOCASE, id, author_id) WHERE superseded_by = 0",
	},
	{
		name:    "idx_books_rating",
		columns: []string{"superseded_by", "rating", "updated_at", "id"},
		ddl: "CREATE INDEX IF NOT EXISTS idx_books_rating " +
			"ON books(rating DESC, updated_at DESC, id DESC) WHERE superseded_by = 0",
	},
	{
		name:    "idx_books_series_trim",
		columns: []string{"superseded_by", "series"},
		ddl: "CREATE INDEX IF NOT EXISTS idx_books_series_trim " +
			"ON books(TRIM(series)) WHERE TRIM(series) <> '' AND superseded_by = 0",
	},
	{
		name:    "idx_books_category",
		columns: []string{"category_l1", "category_l2"},
		ddl:     "CREATE INDEX IF NOT EXISTS idx_books_category ON books(category_l1, category_l2)",
	},
}

// createBookQueryIndexes 按「列齐才建」的规则建全部索引，返回实际创建（或已存在）的条数。
func createBookQueryIndexes(ctx context.Context, tx *sql.Tx) (int, error) {
	exists, err := tableExistsInTx(ctx, tx, "books")
	if err != nil || !exists {
		return 0, err // 老库连 books 都没有：没什么可建的
	}
	has, err := bookColumnSet(ctx, tx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, idx := range bookQueryIndexes {
		if !hasAllColumns(has, idx.columns) {
			continue
		}
		if _, err := tx.ExecContext(ctx, idx.ddl); err != nil {
			return n, fmt.Errorf("store: create index %s: %w", idx.name, err)
		}
		n++
	}
	return n, nil
}

// bookColumnSet 读出 books 现有的列名集合（列探测只做一次，不按索引重复查）。
func bookColumnSet(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT name FROM pragma_table_info('books')")
	if err != nil {
		return nil, fmt.Errorf("store: read books columns: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool, 64)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan books column: %w", err)
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read books columns rows: %w", err)
	}
	return out, nil
}

// hasAllColumns 判断列集合是否覆盖 required。
func hasAllColumns(have map[string]bool, required []string) bool {
	for _, c := range required {
		if !have[c] {
			return false
		}
	}
	return true
}
