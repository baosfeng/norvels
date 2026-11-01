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
	err := s.db.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM books), "+
			"(SELECT COUNT(*) FROM chapters), "+
			"(SELECT COUNT(*) FROM authors), "+
			"(SELECT COALESCE(SUM(word_count), 0) FROM books), "+
			"(SELECT COALESCE(MAX(imported_at), '') FROM books)").Scan(
		&st.Books, &st.Chapters, &st.Authors, &st.Words, &last)
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
		st.CompositionEstimated = comp.Estimated
	}
	return st, nil
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
