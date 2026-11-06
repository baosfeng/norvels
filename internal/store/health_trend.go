package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ImportTrendPoint struct {
	Date     string `json:"date"` // YYYY-MM-DD（本地时区）
	Books    int    `json:"books"`
	Chapters int    `json:"chapters"`
}

// ImportTrend 的实现在 dashboard_read.go（0022 起优先读 import_daily）。

const (
	// dateLayout 是趋势图使用的本地日期键。
	dateLayout = "2006-01-02"
	// maxTrendDays 是趋势查询的天数上限（一年）。
	maxTrendDays = 366
)

// trendStartDay 返回「今天往回 days-1 天」的本地零点。
func trendStartDay(days int) time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return today.AddDate(0, 0, -(days - 1))
}

// trendCounts 按本地日期聚合书数与章节数，返回两张 date→count 表（降级路径）。
//
// 章节不再 JOIN 24,697,647 行的 chapters 逐行 parseTime：章节数在 books.chapter_count
// 上本来就有，按书归日再累加即可。NAS 生产库实测从 28.85 s 降到亚秒级。
func (s *Store) trendCounts(ctx context.Context, start time.Time) (map[string]int, map[string]int, error) {
	return s.countByDayFromBooks(ctx,
		"SELECT imported_at, chapter_count FROM books WHERE imported_at >= ?", start)
}

// countByDayFromBooks 执行「取 (时间文本, 章节数)」的查询并按本地日期聚合。
//
// 时间文本由 store 统一写成 UTC（YYYY-MM-DDTHH:MM:SS.sssZ），
// 这里解析后转本地时区再取日期，避免 UTC 跨日把凌晨导入算到前一天。
// 章节数直接取每行的 chapter_count（books 表 7 万行），不再为每一章查一次归属。
func (s *Store) countByDayFromBooks(ctx context.Context, query string, args ...any) (map[string]int, map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("store: import trend: %w", err)
	}
	defer rows.Close()

	books := make(map[string]int, 32)
	chapters := make(map[string]int, 32)
	for rows.Next() {
		var (
			raw          sql.NullString
			chapterCount int
		)
		if err := rows.Scan(&raw, &chapterCount); err != nil {
			return nil, nil, fmt.Errorf("store: scan import trend: %w", err)
		}
		if !raw.Valid || raw.String == "" {
			continue
		}
		t, err := parseTime(raw.String)
		if err != nil {
			continue // 历史脏数据不影响趋势图
		}
		day := t.Local().Format(dateLayout)
		books[day]++
		chapters[day] += chapterCount
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("store: import trend rows: %w", err)
	}
	return books, chapters, nil
}
