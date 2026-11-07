package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// 本文件是「库级总量」预计算（0026）的读取侧与回填：
// 书数（全量 / 当前版本）、全库书目字数、已算质量分的书数。
//
// 为什么要有它：这四个值都是 books 的一遍扫描，却被最热的两个接口每次请求都算一遍。
// 生产 182,000 本实测（GET /api/metrics）：
//
//	health/report 里那条 COUNT(*) + SUM(CASE quality…)   p95 5500 ms / max 6760 ms
//	stats 里的 SUM(word_count) 与 COUNT(*) WHERE superseded_by = 0   p95 261 ms
//
// 维护方式继承 0025：值由 SQL 触发器在**所有写路径**上维持（见 migrate_0026.go），
// 读取侧在回填完成前回退到实时 SQL（慢但正确），绝不返回错数字，也不 500。

// 四个 kind 的取值（与 0026_book_totals.sql 的 CHECK 约束一致）。
const (
	totalsKindBooksAll        = "books_all"
	totalsKindWordSum         = "word_sum"
	totalsKindQualityComputed = "quality_computed"
)

// bookTotals 是一份库级总量快照。
type bookTotals struct {
	BooksAll        int   // books 表行数（含已被替换的旧版）
	BooksCurrent    int   // 当前版本书数（superseded_by = 0）
	WordSum         int64 // 书目字数之和（books.word_count，与 /api/stats 同口径）
	QualityComputed int   // 已算质量分的书数
}

// BookTotalsReport 是一次回填的汇总（日志与运维命令共用）。
type BookTotalsReport struct {
	bookTotals
	Elapsed time.Duration
}

// bookTotalsTimeout 是单次回填的上限（books 一遍扫描，正常是亚秒级）。
const bookTotalsTimeout = 10 * time.Minute

// liveBookTotalsSQL 是「没有预计算」时的实时口径，**一次扫描算出四个值**。
//
// 为什么不用四条子查询：每条子查询都要把 18 万本整行（含简介）读一遍，
// 四条就是四倍 IO——而这条语句一次全表扫就能同时得到计数与三个求和。
// 三个 SUM 的表达式与触发器的增量表达式逐字同口径（见 migrate_0026.go），
// 所以「实时算」与「预计算」不会给出两个不同的数（有测试守着）。
const liveBookTotalsSQL = "SELECT COUNT(*), " +
	"COALESCE(SUM(CASE WHEN superseded_by = 0 THEN 1 ELSE 0 END), 0), " +
	"COALESCE(SUM(word_count), 0), " +
	"COALESCE(SUM(CASE WHEN quality_chapters > 0 AND quality_at <> '' THEN 1 ELSE 0 END), 0) " +
	"FROM books"

// bookTotals 读预计算值；ready=false 表示还没有可用值（未迁移 / 回填未完成），
// 调用方据此走实时 SQL。
//
// 读的是 4 行小表 + 一行状态，合计亚毫秒，与书数无关。
func (s *Store) bookTotals(ctx context.Context) (bookTotals, bool) {
	if ok, err := s.bookTotalsReady(ctx); err != nil || !ok {
		return bookTotals{}, false
	}
	out, err := s.readBookTotals(ctx)
	if err != nil {
		return bookTotals{}, false
	}
	return out, true
}

// bookTotalsReady 判断预计算是否可用：状态行 complete=1。
//
// 表不存在（0026 未应用）或状态行缺失都按「不可用」处理：读侧据此回退实时 SQL。
func (s *Store) bookTotalsReady(ctx context.Context) (bool, error) {
	var complete int
	err := s.db.QueryRowContext(ctx,
		"SELECT complete FROM book_totals_state WHERE id = 1").Scan(&complete)
	if err != nil {
		return false, nil //nolint:nilerr // 未迁移 / 状态缺失都走降级，不当作错误上报
	}
	return complete != 0, nil
}

// readBookTotals 读出四个 kind（缺行按 0，等价于「库里没有」）。
func (s *Store) readBookTotals(ctx context.Context) (bookTotals, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT kind, n FROM book_totals")
	if err != nil {
		return bookTotals{}, fmt.Errorf("store: read book_totals: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	var out bookTotals
	for rows.Next() {
		var (
			kind string
			n    int64
		)
		if err := rows.Scan(&kind, &n); err != nil {
			return bookTotals{}, fmt.Errorf("store: scan book_totals: %w", err)
		}
		switch kind {
		case totalsKindBooksAll:
			out.BooksAll = int(n)
		case totalsKindCurrent:
			out.BooksCurrent = int(n)
		case totalsKindWordSum:
			out.WordSum = n
		case totalsKindQualityComputed:
			out.QualityComputed = int(n)
		}
	}
	if err := rows.Err(); err != nil {
		return bookTotals{}, fmt.Errorf("store: iterate book_totals: %w", err)
	}
	return out, nil
}

// liveBookTotals 用实时 SQL 算同一份快照（降级路径：预计算未迁移或回填未完成）。
func (s *Store) liveBookTotals(ctx context.Context) (bookTotals, error) {
	var out bookTotals
	err := s.db.QueryRowContext(ctx, liveBookTotalsSQL).Scan(
		&out.BooksAll, &out.BooksCurrent, &out.WordSum, &out.QualityComputed)
	if err != nil {
		return bookTotals{}, fmt.Errorf("store: live book totals: %w", err)
	}
	return out, nil
}

// EnsureBookTotals 在回填未完成时启动后台回填（启动路径与读取降级路径都可调）。
//
// 非阻塞、可重复调用：真正的去重在 StartBookTotalsBackfill 的 atomic CAS 里。
func (s *Store) EnsureBookTotals() {
	if ok, err := s.bookTotalsReady(context.Background()); err != nil || ok {
		return
	}
	s.StartBookTotalsBackfill()
}

// StartBookTotalsBackfill 启动后台回填（非阻塞、atomic 去重）。
func (s *Store) StartBookTotalsBackfill() {
	if backgroundDisabled() {
		return
	}
	b := &s.reconcile
	if !b.totalsRunning.CompareAndSwap(false, true) {
		return
	}
	base := s.backgroundContext()
	go func() {
		defer b.totalsRunning.Store(false)
		rctx, cancel := context.WithTimeout(base, bookTotalsTimeout)
		defer cancel()
		rep, err := s.RebuildBookTotals(rctx)
		if err != nil {
			log.Printf("WARN: 库级总量回填中断（下次启动接着补）: %v", err)
			return
		}
		log.Printf("库级总量回填完成：书 %d / 当前版本 %d / 书目字数 %d / 已算质量分 %d（耗时 %v）",
			rep.BooksAll, rep.BooksCurrent, rep.WordSum, rep.QualityComputed,
			rep.Elapsed.Round(time.Millisecond))
	}()
}

// RebuildBookTotals 全量重算库级总量（后台回填与运维命令共用，可重入）。
//
// 口径与实时 SQL 逐字一致：四个值都来自 liveBookTotalsSQL 的同一次扫描，
// 因此「回填出来的值」与「降级路径算出的值」必然相等——这是触发器正确性的判据。
func (s *Store) RebuildBookTotals(ctx context.Context) (BookTotalsReport, error) {
	start := time.Now()
	var rep BookTotalsReport
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return rep, fmt.Errorf("store: book totals begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作
	if err := rebuildBookTotalsTx(ctx, tx); err != nil {
		return rep, err
	}
	if err := tx.Commit(); err != nil {
		return rep, fmt.Errorf("store: book totals commit: %w", err)
	}
	out, ok := s.bookTotals(ctx)
	if !ok {
		return rep, fmt.Errorf("store: 库级总量回填后仍不可读（检查 0026 是否已应用）")
	}
	rep.bookTotals, rep.Elapsed = out, time.Since(start)
	return rep, nil
}

// rebuildBookTotalsTx 在事务里重算四个值并置状态行 complete=1。
//
// 一条 SQL 完成「算 + 写」：WITH 先做一遍扫描，再把四个结果拆成四行写进表，
// 避免「Go 读一遍、Go 写四行」这段中间态（失败会留下半份值）。
func rebuildBookTotalsTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_totals"); err != nil {
		return fmt.Errorf("store: clear book_totals: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO book_totals(kind, n) "+
			"WITH t AS (SELECT COUNT(*) AS books_all, "+
			"COALESCE(SUM(CASE WHEN superseded_by = 0 THEN 1 ELSE 0 END), 0) AS books_current, "+
			"COALESCE(SUM(word_count), 0) AS word_sum, "+
			"COALESCE(SUM(CASE WHEN quality_chapters > 0 AND quality_at <> '' THEN 1 ELSE 0 END), 0) "+
			"AS quality_computed FROM books) "+
			"SELECT '"+totalsKindBooksAll+"', books_all FROM t "+
			"UNION ALL SELECT '"+totalsKindCurrent+"', books_current FROM t "+
			"UNION ALL SELECT '"+totalsKindWordSum+"', word_sum FROM t "+
			"UNION ALL SELECT '"+totalsKindQualityComputed+"', quality_computed FROM t"); err != nil {
		return fmt.Errorf("store: rebuild book_totals: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE book_totals_state SET complete = 1, updated_at = ? WHERE id = 1",
		formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: mark book_totals_state complete: %w", err)
	}
	return nil
}

// rebuildBookCountsTx 顺手重算 0025 / 0026 的两张计数表（表不存在的老库直接跳过）。
//
// 与 rebuildDashboardTables 同一条理由：refresh-metrics 是「怀疑缓存漂了」时的兜底入口，
// 必须一步到位——只重建直方图而让书级计数 / 库级总量保持漂移，是最坏的一种「成功」。
// 两张表都是 books 一遍扫描，运维命令不在请求路径上，代价可接受。
func rebuildBookCountsTx(ctx context.Context, tx *sql.Tx) error {
	steps := []struct {
		table string
		fn    func(context.Context, *sql.Tx) error
	}{
		{"book_health_counts", rebuildBookHealthCountsTx},
		{"book_totals", rebuildBookTotalsTx},
	}
	for _, step := range steps {
		ok, err := tableExistsInTx(ctx, tx, step.table)
		if err != nil {
			return err
		}
		if !ok {
			continue // 老库没跑 0025 / 0026：没有可重算的对象
		}
		if err := step.fn(ctx, tx); err != nil {
			return err
		}
	}
	return nil
}
