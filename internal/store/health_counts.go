package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// 本文件是「书级健康计数」的读取侧与回填：缺简介 / 缺作者 / 超薄书 / 章节数六段区间。
//
// 为什么要有它：这三条 SQL 是 health/report 请求路径上最后的全表 / 全索引扫描
// （生产 169,230 本实测 4847 ms / 851 ms / 324 ms）。前两条要读 books 的页（简介在
// books 里，GB 级），第三条要把 books 的章节数索引整条扫一遍——都不是 CPU 问题，
// 而是「每个请求都把同一批页从 NAS 重新读一遍」。
//
// 设计取舍（继承 0022 的思路，维护方式更省事）：
//   - 值由 SQL 触发器在**所有写路径**上维持（见 migrate_0025.go），因此报告与明细
//     永远同一口径，不需要在 Go 的每条写路径上补一行；
//   - 迁移只建结构、不回填（升级期间不能停监听）：回填是启动后的后台任务（books 一遍扫描），
//     完成前读取侧回退到实时 SQL（慢但正确），绝不返回错的数字。

// bookHealthCounts 是一份书级健康计数的快照。
type bookHealthCounts struct {
	MissingIntro  int
	MissingAuthor int
	ThinBooks     int
	Buckets       []int // 长度 = len(chapterBucketLabels)
}

// BookHealthCountsReport 是一次回填的汇总（日志与运维命令共用）。
type BookHealthCountsReport struct {
	bookHealthCounts
	Elapsed time.Duration
}

// bookHealthCountsTimeout 是单次回填的上限（books 一遍扫描，正常是亚秒级）。
const bookHealthCountsTimeout = 10 * time.Minute

// bookHealthCounts 读预计算值；ready=false 表示还没有可用值（未迁移 / 回填未完成）。
//
// 读的是两张只有个位数行的小表：两条查询合计亚毫秒，与书数无关。
func (s *Store) bookHealthCounts(ctx context.Context) (bookHealthCounts, bool) {
	var out bookHealthCounts
	if ok, err := s.bookHealthCountsReady(ctx); err != nil || !ok {
		return out, false
	}
	if err := s.scanBookHealthCounts(ctx, &out); err != nil {
		return bookHealthCounts{}, false
	}
	buckets, err := s.bookChapterBucketCounts(ctx)
	if err != nil {
		return bookHealthCounts{}, false
	}
	out.Buckets = buckets
	return out, true
}

// scanBookHealthCounts 把三个标量读进 out。
func (s *Store) scanBookHealthCounts(ctx context.Context, out *bookHealthCounts) error {
	rows, err := s.db.QueryContext(ctx, "SELECT kind, n FROM book_health_counts")
	if err != nil {
		return fmt.Errorf("store: read book_health_counts: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return fmt.Errorf("store: scan book_health_counts: %w", err)
		}
		switch kind {
		case "missing_intro":
			out.MissingIntro = n
		case "missing_author":
			out.MissingAuthor = n
		case "thin_books":
			out.ThinBooks = n
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: iterate book_health_counts: %w", err)
	}
	return nil
}

// bookHealthCountsReady 判断预计算值是否可用：状态行 complete=1。
//
// 表不存在（0025 未应用）或状态行缺失都按「不可用」处理：读取侧据此回退到实时 SQL。
func (s *Store) bookHealthCountsReady(ctx context.Context) (bool, error) {
	var complete int
	err := s.db.QueryRowContext(ctx,
		"SELECT complete FROM book_health_state WHERE id = 1").Scan(&complete)
	if err != nil {
		return false, nil //nolint:nilerr // 未迁移 / 状态缺失都走降级，不当作错误上报
	}
	return complete != 0, nil
}

// bookChapterBucketCounts 读出六个桶的计数。
func (s *Store) bookChapterBucketCounts(ctx context.Context) ([]int, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT bucket, n FROM book_chapter_buckets ORDER BY bucket")
	if err != nil {
		return nil, fmt.Errorf("store: read book_chapter_buckets: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	out := make([]int, len(chapterBucketLabels))
	for rows.Next() {
		var bucket, n int
		if err := rows.Scan(&bucket, &n); err != nil {
			return nil, fmt.Errorf("store: scan book_chapter_buckets: %w", err)
		}
		if bucket >= 0 && bucket < len(out) {
			out[bucket] = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate book_chapter_buckets: %w", err)
	}
	return out, nil
}

// missingIntroWhere 返回「缺简介」的 WHERE 片段：优先用生成列（索引查找），旧库退回 TRIM 表达式。
//
// 两条谓词语义完全相同（生成列的表达式就是 TRIM(intro) = ”），选哪条只影响快慢：
// 0025 之前建的库没有 intro_blank 列，写死新谓词会直接报 no such column。
func (s *Store) missingIntroWhere(ctx context.Context) string {
	if has, err := s.columnExists(ctx, "books", "intro_blank"); err == nil && has {
		return "intro_blank = 1"
	}
	return whereMissingIntro
}

// columnExists 判断主库某表是否有某列（走 pragma_table_info，微秒级）。
func (s *Store) columnExists(ctx context.Context, table, column string) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("store: check column %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}

// EnsureBookHealthCounts 在回填未完成时启动后台回填（启动路径与健康读取都可调）。
//
// 非阻塞、可重复调用：真正的去重在 StartBookHealthCountsBackfill 的 atomic CAS 里。
func (s *Store) EnsureBookHealthCounts() {
	if ok, err := s.bookHealthCountsReady(context.Background()); err != nil || ok {
		return
	}
	s.StartBookHealthCountsBackfill()
}

// StartBookHealthCountsBackfill 启动后台回填（非阻塞、atomic 去重）。
func (s *Store) StartBookHealthCountsBackfill() {
	if backgroundDisabled() {
		return
	}
	b := &s.reconcile
	if !b.healthCountsRunning.CompareAndSwap(false, true) {
		return
	}
	base := s.backgroundContext()
	go func() {
		defer b.healthCountsRunning.Store(false)
		rctx, cancel := context.WithTimeout(base, bookHealthCountsTimeout)
		defer cancel()
		rep, err := s.RebuildBookHealthCounts(rctx)
		if err != nil {
			log.Printf("WARN: 书级健康计数回填中断（下次启动接着补）: %v", err)
			return
		}
		log.Printf("书级健康计数回填完成：缺简介 %d / 缺作者 %d / 超薄 %d（耗时 %v）",
			rep.MissingIntro, rep.MissingAuthor, rep.ThinBooks, rep.Elapsed.Round(time.Millisecond))
	}()
}

// RebuildBookHealthCounts 全量重算书级健康计数（后台回填与运维命令共用，可重入）。
//
// 口径与实时 SQL 逐字一致：缺简介 = intro_blank（即 TRIM(intro) = ”）、缺作者 =
// author_id = 0 或无对应作者、超薄 = chapter_count ≤ thinBookChapters、分桶 = bucketRangeSQL。
func (s *Store) RebuildBookHealthCounts(ctx context.Context) (BookHealthCountsReport, error) {
	start := time.Now()
	var rep BookHealthCountsReport
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return rep, fmt.Errorf("store: book health counts begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作
	if err := rebuildBookHealthCountsTx(ctx, tx); err != nil {
		return rep, err
	}
	if err := tx.Commit(); err != nil {
		return rep, fmt.Errorf("store: book health counts commit: %w", err)
	}
	counts, ok := s.bookHealthCounts(ctx)
	if !ok {
		return rep, fmt.Errorf("store: 书级健康计数回填后仍不可读（检查 0025 是否已应用）")
	}
	rep.bookHealthCounts, rep.Elapsed = counts, time.Since(start)
	return rep, nil
}

// rebuildBookHealthCountsTx 在事务里重算两张计数表。
func rebuildBookHealthCountsTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_health_counts"); err != nil {
		return fmt.Errorf("store: clear book_health_counts: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO book_health_counts(kind, n) VALUES "+
			"('missing_intro', (SELECT COUNT(*) FROM books WHERE TRIM(intro) = '')), "+
			"('missing_author', (SELECT COUNT(*) FROM books WHERE author_id = 0 OR NOT EXISTS "+
			"(SELECT 1 FROM authors a WHERE a.id = books.author_id))), "+
			"('thin_books', (SELECT COUNT(*) FROM books WHERE chapter_count <= ?))",
		thinBookChapters); err != nil {
		return fmt.Errorf("store: rebuild book_health_counts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_chapter_buckets"); err != nil {
		return fmt.Errorf("store: clear book_chapter_buckets: %w", err)
	}
	counts, err := bucketCountsOn(ctx, tx, "chapter_count", "books", chapterBucketEdges)
	if err != nil {
		return fmt.Errorf("store: count chapter buckets: %w", err)
	}
	for bucket, n := range counts {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO book_chapter_buckets(bucket, n) VALUES (?, ?)", bucket, n); err != nil {
			return fmt.Errorf("store: write bucket %d: %w", bucket, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE book_health_state SET complete = 1, updated_at = ? WHERE id = 1",
		formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: mark book_health_state complete: %w", err)
	}
	return nil
}
