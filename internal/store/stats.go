package store

import (
	"context"
	"fmt"
	"os"
)

// Stats 返回库级统计。DBBytes 是**两个库文件**（主库 + 正文库）与各自 -wal 的合计，
// 即用户真正占用的磁盘量。
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var (
		st   Stats
		last string
	)
	// 书数/作者数是千级行，实时算就是毫秒级，不进缓存（缓存反而多一次读表）。
	// 章节数另有门道：原先这里还有一句 (SELECT COUNT(*) FROM chapters)，
	// NAS 生产库 24,697,647 章时它每次请求都要数一遍整个 btree（实测 152 ms/次）。
	// 0022 起改读 store_metrics 的 chapter_count（导入时同事务增量维护）；
	// 缓存缺失时退回实时 COUNT，并把 CompositionEstimated 置 true（接口照常 200）。
	chapters, estimated, err := s.chapterTotal(ctx)
	if err != nil {
		return Stats{}, err
	}
	st.Chapters = chapters
	err = s.db.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM books WHERE superseded_by = 0), "+
			"(SELECT COUNT(*) FROM authors), "+
			"(SELECT COALESCE(SUM(word_count), 0) FROM books), "+
			"(SELECT COALESCE(MAX(imported_at), '') FROM books)").Scan(
		&st.Books, &st.Authors, &st.Words, &last)
	if err != nil {
		return Stats{}, fmt.Errorf("store: stats: %w", err)
	}
	if last != "" {
		if t, err := parseTime(last); err == nil {
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
