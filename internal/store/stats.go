package store

import (
	"context"
	"fmt"
	"os"
)

// Stats 返回库级统计。DBBytes 含主库与 -wal 文件的实际占用。
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
	// 体积构成是估算值；估算失败不影响主统计（前端仍能画「未知」退化）。
	if textBytes, indexBytes, cerr := s.estimateComposition(ctx); cerr == nil {
		fillComposition(&st, textBytes, indexBytes)
	}
	return st, nil
}

// diskUsage 统计主库与 WAL 文件字节数（不存在按 0 计）。
func (s *Store) diskUsage() int64 {
	var total int64
	for _, p := range []string{s.path, s.path + "-wal"} {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	return total
}
