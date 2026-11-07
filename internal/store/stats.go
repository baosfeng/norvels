package store

import (
	"context"
	"fmt"
	"os"
)

// Stats 返回库级统计。DBBytes 是**两个库文件**（主库 + 正文库）与各自 -wal 的合计，
// 即用户真正占用的磁盘量。
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	// 章节数/书数/书目字数三个整型聚合都曾绑在请求路径上：
	//   - COUNT(*) FROM chapters 每次数 2,469 万行（实测 152 ms），0022 起读 store_metrics；
	//   - COUNT(*) FROM books WHERE superseded_by = 0 与 SUM(word_count) FROM books
	//     是 books 的一次全表扫（生产 18.2 万本 p95 261 ms），0026 起读 book_totals。
	// 两者都保留实时降级：预计算不可用时数字仍正确，只是这一次慢。
	chapters, estimated, err := s.chapterTotal(ctx)
	if err != nil {
		return Stats{}, err
	}
	st.Chapters = chapters
	books, words, err := s.bookTotalsForStats(ctx)
	if err != nil {
		return Stats{}, err
	}
	st.Books, st.Words = books, words
	// 作者数是千级行、最近导入时间走 idx_books_imported 的倒序扫描：两条都毫秒级，
	// 不进预计算（多一张表就要多一套触发器，收益为零）。
	var last string
	err = s.db.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM authors), (SELECT COALESCE(MAX(imported_at), '') FROM books)").Scan(
		&st.Authors, &last)
	if err != nil {
		return Stats{}, fmt.Errorf("store: stats: %w", err)
	}
	if last != "" {
		if t, perr := parseTime(last); perr == nil {
			st.LastImportAt = &t
		}
	}
	st.DBBytes = s.diskUsage()
	// 体积构成：优先读缓存（精确、毫秒级），缺失时退化为抽样估算。
	// 估算失败不影响主统计（前端仍能画「未知」退化）。
	if comp, cerr := s.Composition(ctx); cerr == nil {
		fillComposition(&st, comp.TextBytes, comp.IndexBytes)
		st.TextBytesRaw = comp.TextBytesRaw
		st.CompositionEstimated = comp.Estimated || estimated
	}
	return st, nil
}

// bookTotalsForStats 取 /api/stats 的书数与书目字数：优先 0026 预计算，不可用时实时扫。
//
// 实时路径就是 0026 之前的原语句（一条 SQL 里的两个子查询，同一份 books 扫描）。
func (s *Store) bookTotalsForStats(ctx context.Context) (int, int64, error) {
	if totals, ok := s.bookTotals(ctx); ok {
		return totals.BooksCurrent, totals.WordSum, nil
	}
	// 降级：拉一次后台回填，本次请求仍给出正确数字（慢一点而已）。
	s.EnsureBookTotals()
	var books int
	var words int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM books WHERE superseded_by = 0), "+
			"(SELECT COALESCE(SUM(word_count), 0) FROM books)").Scan(&books, &words); err != nil {
		return 0, 0, fmt.Errorf("store: stats books: %w", err)
	}
	return books, words, nil
}

// chapterTotal 取全库章节数：优先读 store_metrics 的预计算值，缺失时退回实时 COUNT。
//
// 返回 estimated=true 表示这次是实时数的（缓存缺失或口径落后），调用方据此把
// /api/stats 标成估算——数字是对的，只是这次走了慢路径，运维需要看见。
func (s *Store) chapterTotal(ctx context.Context) (int, bool, error) {
	if snap, err := s.Metrics(ctx); err == nil && !snap.Stale {
		return int(snap.ChapterCount), false, nil
	}
	n, err := s.scalarInt64(ctx, chapterCountSQL)
	if err != nil {
		// 实时兜底也失败才算真错（例如库被删）：此时宁可 500 也不要编一个 0 出来。
		return 0, false, fmt.Errorf("store: stats: chapter count: %w", err)
	}
	return int(n), true, nil
}

// diskUsage 统计两个库文件与各自 WAL 的字节数（不存在按 0 计）。
func (s *Store) diskUsage() int64 {
	return dbFileBytes(s.path) + dbFileBytes(s.contentPath)
}

// mainBytes 只统计主库（含 WAL）的字节数，用于「索引类」体积口径。
func (s *Store) mainBytes() int64 { return dbFileBytes(s.path) }

// dbFileBytes 返回一个 SQLite 文件与其 -wal 的合计字节数。
func dbFileBytes(path string) int64 {
	var total int64
	for _, p := range []string{path, path + "-wal"} {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	return total
}
