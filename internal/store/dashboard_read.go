package store

import (
	"context"
	"fmt"
)

// 本文件是驾驶舱三张预计算表（0022）的**读侧**：章节长度直方图、导入趋势、作者列表。
//
// 共同约定：预计算不可用（老库没跑 0022、回填未完成、口径版本落后、时区变了）时
// 降级为原实时 SQL 并让调用方标 stale，**绝不返回 500**。降级路径保留是为了
// 「启动即可用」：大库回填要几十秒，那段时间驾驶舱必须还能打开。

// lengthHistFromTable 从预计算表读长度直方图（一行一桶）。
//
// 缺失的桶补 0：表里只会有「真有数据的桶」，而前端要的是固定的 5 根柱子。
// 桶号越界的残留行直接跳过（口径变更后可能留下旧桶号），不让它污染展示。
func (s *Store) lengthHistFromTable(ctx context.Context) ([]int, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT bucket, count FROM "+tableChapterLengthHist+" ORDER BY bucket")
	if err != nil {
		return nil, fmt.Errorf("store: read length hist: %w", err)
	}
	defer rows.Close()

	counts := make([]int, len(lengthBucketEdges))
	for rows.Next() {
		var bucket, count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, fmt.Errorf("store: scan length hist: %w", err)
		}
		if bucket < 0 || bucket >= len(counts) || count <= 0 {
			continue
		}
		counts[bucket] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read length hist rows: %w", err)
	}
	return counts, nil
}

// ImportTrend 返回最近 days 天（含今天）每天的导入书数与章节数。
//
// 章节按所属书的 imported_at 归日（章节本身没有导入时间），
// 没有数据的日期补 0，保证前端 x 轴连续。
//
// 0022 起读 import_daily（一天一行），不再 JOIN 24,697,647 行的 chapters：
// 原语句把每一章拉进 Go 逐行 parseTime，NAS 生产库单次要 28.85 s。
func (s *Store) ImportTrend(ctx context.Context, days int) ([]ImportTrendPoint, error) {
	if days <= 0 {
		days = 14
	}
	if days > maxTrendDays {
		days = maxTrendDays
	}
	start := trendStartDay(days)
	out := make([]ImportTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		out = append(out, ImportTrendPoint{Date: start.AddDate(0, 0, i).Format(dateLayout)})
	}
	if ok, _ := s.PrecomputeStatus(ctx); ok {
		if books, chapters, err := s.countByDayFromTable(ctx); err == nil {
			fillTrend(out, books, chapters)
			return out, nil
		}
	}
	// 降级：预计算不可用时按原口径实时算（慢，但结果一致，有测试守着）。
	books, chapters, err := s.trendCounts(ctx, start)
	if err != nil {
		return nil, err
	}
	fillTrend(out, books, chapters)
	return out, nil
}

// fillTrend 把两张 date→count 表填进趋势点。
func fillTrend(out []ImportTrendPoint, books, chapters map[string]int) {
	for i := range out {
		out[i].Books = books[out[i].Date]
		out[i].Chapters = chapters[out[i].Date]
	}
}

// countByDayFromTable 读 import_daily 并聚合成「本地日 → 计数」。
//
// 表里的 day 已经是**本地日**（写入侧用 localDayKey 归并，见 dashboard_precompute.go），
// 所以这里不必再做时区换算：时区变化由 PrecomputeStatus 的口径校验兜住。
func (s *Store) countByDayFromTable(ctx context.Context) (map[string]int, map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT day, book_count, chapter_count FROM "+tableImportDaily+" ORDER BY day")
	if err != nil {
		return nil, nil, fmt.Errorf("store: import trend from table: %w", err)
	}
	defer rows.Close()

	books := make(map[string]int, 32)
	chapters := make(map[string]int, 32)
	for rows.Next() {
		var (
			day          string
			bookCount    int
			chapterCount int
		)
		if err := rows.Scan(&day, &bookCount, &chapterCount); err != nil {
			return nil, nil, fmt.Errorf("store: scan import trend day: %w", err)
		}
		books[day] += bookCount
		chapters[day] += chapterCount
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("store: import trend from table rows: %w", err)
	}
	return books, chapters, nil
}
