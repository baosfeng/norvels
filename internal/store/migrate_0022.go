package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// 0022 的主库步骤：给三张预计算表灌入当前库的精确值。
//
// 为什么回填不能写在 0022_dashboard_precompute.sql 里：回填要「先查业务表、再写
// 预计算表」，是跨表的数据搬运，SQL 文件里做不到（也不是能一条语句写完的聚合）。
// 但它**必须与建表同事务**：中途失败时若表已提交而值是 0，读接口会一直报 stale，
// 后台补算又要再扫一遍全表；放在同一事务里则是「要么都有，要么都没有」。
//
// 回填口径与增量维护（dashboard_precompute.go）严格一致，否则历史库与增量库
// 会给出两种数字——有测试守着这条（TestPrecomputeIncrementalMatchesFull）。
func populateDashboardPrecomputeStep(ctx context.Context, tx *sql.Tx) error {
	t0 := time.Now()
	// 老库升级路径上业务表可能还不全（v1 老库只有 books，没有 chapters），
	// 也可能缺某个列。回填按「表在才回填」处理：缺的表留空，等增量维护或
	// 运维跑 refresh-metrics 补齐——读接口对空表是降级为实时算，不会给出错数字。
	if err := rebuildDashboardTables(ctx, tx); err != nil {
		return err
	}
	days, err := countTableRows(ctx, tx, tableImportDaily)
	if err != nil {
		return err
	}
	authors, err := countTableRows(ctx, tx, tableAuthorAgg)
	if err != nil {
		return err
	}
	// 口径版本与时区偏移随回填一起落库：读接口据此判断「表里的值是不是当前口径」，
	// 以及「按本地日归并的 key 还能不能用」（换了时区就必须重算）。
	for _, kv := range []struct {
		key   string
		value int64
	}{
		{metricPrecomputeVersion, precomputeVersion},
		{metricPrecomputeZone, int64(currentZoneOffset())},
	} {
		if err := setMetric(ctx, tx, kv.key, kv.value); err != nil {
			return err
		}
	}
	log.Printf("迁移 0022：驾驶舱预计算已回填 %d 天导入趋势 / %d 个长度分桶 / %d 位作者聚合，耗时 %v",
		days, len(lengthBucketEdges), authors, time.Since(t0).Round(time.Millisecond))
	return nil
}

// rebuildDashboardTables 在一个事务里重建三张预计算表（迁移回填与 RefreshMetrics 共用）。
//
// 迁移路径与运维路径必须是**同一份实现**：两份代码各写一遍聚合，迟早有一边漏了
// 某个 WHERE（比如「被取代的书算不算」），而这类漂移不会报错，只会让驾驶舱的
// 数字慢慢和书架不一致。
func rebuildDashboardTables(ctx context.Context, tx *sql.Tx) error {
	hasBooks, err := tableExistsInTx(ctx, tx, "books")
	if err != nil {
		return err
	}
	if hasBooks {
		if _, err := rebuildImportDaily(ctx, tx); err != nil {
			return err
		}
	}
	hasChapters, err := tableExistsInTx(ctx, tx, "chapters")
	if err != nil {
		return err
	}
	if hasChapters {
		if _, err := rebuildChapterLengthHist(ctx, tx); err != nil {
			return err
		}
	}
	hasAuthors, err := tableExistsInTx(ctx, tx, "authors")
	if err != nil {
		return err
	}
	if !hasAuthors {
		return nil
	}
	_, err = rebuildAuthorAgg(ctx, tx)
	return err
}

// rebuildImportDaily 用 books 重建 import_daily（一天一行）。
//
// 口径：按 **本地日** 归并，与增量维护（localDayKey）和降级路径（countByDayFromBooks）
// 逐字一致。归并在 Go 里做而不是写一句 substr(imported_at, 1, 10)：库里存的是 UTC
// 文本，直接截前 10 位得到的是 UTC 日，而北京时间凌晨导入的书会被算到前一天——
// 这正是原实现要 parseTime → t.Local() 的原因，口径不能在这里悄悄改回 UTC。
//
// 章节数直接取 books.chapter_count：章节数在 books 上本来就有，
// 不必再 JOIN 一次 2,469 万行的 chapters（那正是本次要消灭的查询）。
func rebuildImportDaily(ctx context.Context, tx *sql.Tx) (int, error) {
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+tableImportDaily); err != nil {
		return 0, fmt.Errorf("store: clear import_daily: %w", err)
	}
	books, chapters, err := readBookImportDays(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := writeImportDaily(ctx, tx, books, chapters); err != nil {
		return 0, err
	}
	return len(books), nil
}

// readBookImportDays 扫 books 并按本地日聚合书数与章节数。
func readBookImportDays(ctx context.Context, tx *sql.Tx) (map[string]int, map[string]int, error) {
	// 老库可能还没有 chapter_count 列（0008 之前）：用 COALESCE 也挡不住「列不存在」，
	// 因此先探测列，缺了就按 0 章算——那时的章节表本来也未必存在。
	// 两条列都按「可能有、可能没有」处理：v1 老库的 books 只有 title。
	// 缺 imported_at 就没有趋势可言（全 0），缺 chapter_count 就按 0 章算。
	hasDay, err := columnExistsInTx(ctx, tx, "books", "imported_at")
	if err != nil {
		return nil, nil, err
	}
	if !hasDay {
		return map[string]int{}, map[string]int{}, nil
	}
	query := "SELECT imported_at, chapter_count FROM books WHERE imported_at <> ''"
	hasCount, err := columnExistsInTx(ctx, tx, "books", "chapter_count")
	if err != nil {
		return nil, nil, err
	}
	if !hasCount {
		query = "SELECT imported_at, 0 FROM books WHERE imported_at <> ''"
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("store: read books for import_daily: %w", err)
	}
	defer rows.Close()

	books := make(map[string]int, 512)
	chapters := make(map[string]int, 512)
	for rows.Next() {
		var (
			raw          string
			chapterCount int
		)
		if err := rows.Scan(&raw, &chapterCount); err != nil {
			return nil, nil, fmt.Errorf("store: scan book import day: %w", err)
		}
		day, ok := localDayKey(raw)
		if !ok {
			continue // 脏时间不进趋势表（与降级路径 continue 同一口径）
		}
		books[day]++
		chapters[day] += chapterCount
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("store: read books rows for import_daily: %w", err)
	}
	return books, chapters, nil
}

// writeImportDaily 把「本地日 → 计数」写进 import_daily。
func writeImportDaily(ctx context.Context, tx *sql.Tx, books, chapters map[string]int) error {
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO "+tableImportDaily+"(day, book_count, chapter_count) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("store: prepare import_daily: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for day, n := range books {
		if _, err := stmt.ExecContext(ctx, day, n, chapters[day]); err != nil {
			return fmt.Errorf("store: write import_daily %s: %w", day, err)
		}
	}
	return nil
}

// rebuildChapterLengthHist 用 chapters.word_count 重建长度直方图。
//
// 这里仍然要扫一遍 chapters，但那是**迁移时的一次**，不是每次请求；
// 真实库（2,469 万章）实测约 0.6 s，而原先的五段区间 COUNT 每次请求都要重扫。
func rebuildChapterLengthHist(ctx context.Context, tx *sql.Tx) (int, error) {
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+tableChapterLengthHist); err != nil {
		return 0, fmt.Errorf("store: clear chapter_length_hist: %w", err)
	}
	counts, err := bucketCountsOn(ctx, tx, "word_count", "chapters", lengthBucketEdges)
	if err != nil {
		return 0, fmt.Errorf("store: count length buckets: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO "+tableChapterLengthHist+"(bucket, count) VALUES (?, ?)")
	if err != nil {
		return 0, fmt.Errorf("store: prepare hist: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for i, c := range counts {
		if _, err := stmt.ExecContext(ctx, i, c); err != nil {
			return 0, fmt.Errorf("store: write hist bucket %d: %w", i, err)
		}
	}
	return len(counts), nil
}

// rebuildAuthorAgg 用 authorWorks 重建作者聚合（作品数与字数）。
//
// 口径与 refreshAuthorAgg 完全一致：主作者（books.author_id）+ 副作者
// （books_authors）UNION 去重后按书聚合，因此「详情页能看到的多作者」
// 在作者页也数得到，不会出现挂着书却查不到的数据孤儿。
func rebuildAuthorAgg(ctx context.Context, tx *sql.Tx) (int, error) {
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+tableAuthorAgg); err != nil {
		return 0, fmt.Errorf("store: clear author_agg: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+tableAuthorAgg+"(author_id, book_count, word_count) "+
			"SELECT a.id, COUNT(b.id), COALESCE(SUM(b.word_count), 0) "+
			"FROM authors a LEFT JOIN "+authorWorks+" x ON x.aid = a.id "+
			"LEFT JOIN books b ON b.id = x.book_id GROUP BY a.id"); err != nil {
		return 0, fmt.Errorf("store: rebuild author_agg: %w", err)
	}
	return countTableRows(ctx, tx, tableAuthorAgg)
}

// countTableRows 数一张表的行数（迁移日志用）。
func countTableRows(ctx context.Context, tx *sql.Tx, table string) (int, error) {
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count %s: %w", table, err)
	}
	return n, nil
}
