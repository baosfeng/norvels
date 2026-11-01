package recstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// 测试辅助：每个用例一个独立的临时库，互不干扰（库文件在 t.TempDir 里随用例删除）。

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "norvels-rec.db"))
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// recordEvents 是「写入必须成功」的测试捷径：失败直接 Fatal，用例正文只关心结果。
func recordEvents(t *testing.T, s *Store, userID int64, events ...Event) {
	t.Helper()
	if _, err := s.RecordEvents(context.Background(), userID, events); err != nil {
		t.Fatalf("写入事件失败: %v", err)
	}
}

// countRows 数一张表的行数（表名只来自测试里的常量，不存在注入面）。
func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("数 %s 行数失败: %v", table, err)
	}
	return n
}

// bookStats 读回一本书的聚合行（不存在则零值 + false）。
func bookStats(t *testing.T, s *Store, bookID int64) (impressions, clicks, reads, seconds, shelf int, ok bool) {
	t.Helper()
	err := s.db.QueryRowContext(context.Background(),
		"SELECT impressions, clicks, reads, read_seconds, shelf_adds FROM book_stats WHERE book_id = ?",
		bookID).Scan(&impressions, &clicks, &reads, &seconds, &shelf)
	return impressions, clicks, reads, seconds, shelf, err == nil
}

// closeStore 关闭库并断言无错误（错误路径用例里用来制造「库已关」）。
func closeStore(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatalf("关闭库失败: %v", err)
	}
}

var testNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
