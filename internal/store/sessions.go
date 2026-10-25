package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// 阅读会话 / 统计 / 历史（P3 §阅读时长统计、§阅读历史）。
//
// 会话由前端按「页面可见时累计、≥60 s 且切换章节/隐藏/卸载才上报」的节奏写，
// 因此这里只做单行插入：不 upsert、不合并，一条上报就是一行。
//
// 日期口径统一用 UTC（库内时间全是 UTC 文本）：按日聚合与「连续天数」都走 SQLite
// 的 date(started_at)。服务端不假设部署机时区，容器里通常是 UTC，跟着它走才不会
// 出现「同一台机器换时区后历史统计整体平移」的问题。

// 会话时长的跨边界区间：1 s–24 h。上限挡的是把一天以上塞进一条上报的错误客户端。
const (
	minSessionSeconds = 1
	maxSessionSeconds = 86400
)

// ReadingSessionInput 是一次阅读会话上报（startedAt 空 = 服务端当前时间）。
type ReadingSessionInput struct {
	BookID    int64
	ChapterID int64
	Seconds   int
	StartedAt string
}

// DaySeconds 是某一天的阅读秒数（热力图的一格）。
type DaySeconds struct {
	Date    string `json:"date"` // YYYY-MM-DD（UTC）
	Seconds int    `json:"seconds"`
}

// ReadingStats 是阅读统计：累计 / 今日 / 本周 / 连续天数 + 近 N 天日历。
type ReadingStats struct {
	TotalSeconds int          `json:"totalSeconds"`
	TodaySeconds int          `json:"todaySeconds"`
	WeekSeconds  int          `json:"weekSeconds"`
	Streak       int          `json:"streak"`
	Days         []DaySeconds `json:"days"`
}

// HistoryRow 是按书聚合的一行阅读历史。
type HistoryRow struct {
	BookID       int64  `json:"bookId"`
	BookTitle    string `json:"bookTitle"`
	ChapterID    int64  `json:"chapterId"`
	ChapterTitle string `json:"chapterTitle"`
	LastReadAt   string `json:"lastReadAt"`   // 该书最后一次会话的开始时间
	TotalSeconds int    `json:"totalSeconds"` // 该书累计秒数
	Days         int    `json:"days"`         // 该书有会话的天数（去重）
}

// AddReadingSession 记录一次阅读会话。
//
// 章节必须属于这本书（不属于 → ErrNotFound，与 saveReadingProgress 同一口径）；
// 时长越界 → ErrInvalidArgument，不静默截断：前端已经保证 ≥60 s，越界说明客户端算错。
func (s *Store) AddReadingSession(ctx context.Context, in ReadingSessionInput) error {
	if in.BookID <= 0 || in.ChapterID <= 0 {
		return fmt.Errorf("%w: book=%d chapter=%d", ErrInvalidArgument, in.BookID, in.ChapterID)
	}
	if in.Seconds < minSessionSeconds || in.Seconds > maxSessionSeconds {
		return fmt.Errorf("%w: seconds %d 必须在 %d~%d 之间",
			ErrInvalidArgument, in.Seconds, minSessionSeconds, maxSessionSeconds)
	}
	if err := s.assertChapterOfBook(ctx, in.BookID, in.ChapterID); err != nil {
		return err
	}
	startedAt, err := sessionStart(in.StartedAt)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO reading_sessions(book_id, chapter_id, started_at, seconds) VALUES (?, ?, ?, ?)",
		in.BookID, in.ChapterID, startedAt, in.Seconds); err != nil {
		return fmt.Errorf("store: add reading session: %w", err)
	}
	return nil
}

// sessionStart 归一化会话开始时间：空串取服务端当前时间，非空必须可解析。
func sessionStart(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return formatTime(nowUTC()), nil
	}
	t, err := parseTime(raw)
	if err != nil {
		return "", fmt.Errorf("%w: startedAt %q 不是合法时间", ErrInvalidArgument, raw)
	}
	return formatTime(t), nil
}

// ReadingStats 返回阅读统计；days 夹在 1–365（默认 90）。
//
// Streak 的口径：从今天往前，连续的「有会话的天」；今天没有会话就是 0
// （正在读的书还没上报，不该假装连续到昨天）。
func (s *Store) ReadingStats(ctx context.Context, days, tzOffsetMinutes int) (ReadingStats, error) {
	days = normalizeStatsDays(days)
	if err := validateTZOffset(tzOffsetMinutes); err != nil {
		return ReadingStats{}, err
	}
	// 「今天」按调用方时区取：库内时间与容器时区都是 UTC，跟着部署机走会错日。
	today := dateUTC(dayInOffset(nowUTC(), tzOffsetMinutes))
	total, todaySeconds, weekSeconds, err := s.readingTotals(ctx, today, tzOffsetMinutes)
	if err != nil {
		return ReadingStats{}, err
	}
	byDay, err := s.dailySeconds(ctx, dateUTC(today.AddDate(0, 0, -(days-1))), tzOffsetMinutes)
	if err != nil {
		return ReadingStats{}, err
	}
	streak, err := s.readingStreak(ctx, today, tzOffsetMinutes)
	if err != nil {
		return ReadingStats{}, err
	}
	return ReadingStats{
		TotalSeconds: total,
		TodaySeconds: todaySeconds,
		WeekSeconds:  weekSeconds,
		Streak:       streak,
		Days:         fillDailySeconds(today, days, byDay),
	}, nil
}

// normalizeStatsDays 夹取统计窗口：days ≤ 0 取默认 90，超过 365 取上限。
func normalizeStatsDays(days int) int {
	switch {
	case days <= 0:
		return defaultStatsDays
	case days > maxStatsDays:
		return maxStatsDays
	default:
		return days
	}
}

const (
	defaultStatsDays = 90
	maxStatsDays     = 365
)

// readingTotals 一次查询取回累计 / 今日 / 本周（本周从周一起算，UTC）。
func (s *Store) readingTotals(ctx context.Context, today time.Time, tzOffsetMinutes int) (int, int, int, error) {
	var total, todaySeconds, weekSeconds int
	mod := tzModifier(tzOffsetMinutes)
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(seconds), 0), "+
			"COALESCE(SUM(CASE WHEN date(started_at, ?) = ? THEN seconds END), 0), "+
			"COALESCE(SUM(CASE WHEN date(started_at, ?) >= ? THEN seconds END), 0) "+
			"FROM reading_sessions",
		mod, dateUTCString(today), mod, dateUTCString(weekStartOf(today))).
		Scan(&total, &todaySeconds, &weekSeconds)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("store: reading totals: %w", err)
	}
	return total, todaySeconds, weekSeconds, nil
}

// dailySeconds 读回 [from, 今天] 的按日秒数；没有会话的日子不出现在 map 里。
func (s *Store) dailySeconds(ctx context.Context, from time.Time, tzOffsetMinutes int) (map[string]int, error) {
	mod := tzModifier(tzOffsetMinutes)
	rows, err := s.db.QueryContext(ctx,
		"SELECT date(started_at, ?), SUM(seconds) FROM reading_sessions "+
			"WHERE date(started_at, ?) >= ? GROUP BY 1", mod, mod, dateUTCString(from))
	if err != nil {
		return nil, fmt.Errorf("store: daily seconds: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int, 90)
	for rows.Next() {
		var (
			day     string
			seconds int
		)
		if err := rows.Scan(&day, &seconds); err != nil {
			return nil, fmt.Errorf("store: scan daily seconds: %w", err)
		}
		out[day] = seconds
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: daily seconds rows: %w", err)
	}
	return out, nil
}

// readingStreak 从今天往前数连续有会话的天数（今天无会话 = 0）。
//
// 单独一次查询而不是复用窗口内的 map：窗口只有 90 天，而连续天数可能更长
// （每天只读 1 分钟也会累积）；口径仍然是「按 UTC 日期去重」。
func (s *Store) readingStreak(ctx context.Context, today time.Time, tzOffsetMinutes int) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT date(started_at, ?) FROM reading_sessions GROUP BY 1 ORDER BY 1 DESC", tzModifier(tzOffsetMinutes))
	if err != nil {
		return 0, fmt.Errorf("store: reading streak: %w", err)
	}
	defer rows.Close()

	streak := 0
	want := dateUTCString(today)
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return 0, fmt.Errorf("store: scan streak day: %w", err)
		}
		if day != want {
			break // 今天没有会话，或中间断了
		}
		streak++
		want = dateUTCString(today.AddDate(0, 0, -streak))
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("store: reading streak rows: %w", err)
	}
	return streak, nil
}

// fillDailySeconds 把聚合结果铺成 [今天-days+1, 今天] 的连续日历，缺失日补 0。
func fillDailySeconds(today time.Time, days int, byDay map[string]int) []DaySeconds {
	out := make([]DaySeconds, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := dateUTCString(today.AddDate(0, 0, -i))
		out = append(out, DaySeconds{Date: day, Seconds: byDay[day]})
	}
	return out
}

// ReadingHistory 按书聚合分页历史：最后章节 / 最后阅读时间 / 累计秒数 / 天数。
//
// 按书聚合而不是按会话列：用户要看的是「这本书读了多久、读到哪」，
// 逐条会话是统计的原材料，不是列表的内容。
func (s *Store) ReadingHistory(ctx context.Context, page, size int) ([]HistoryRow, int, error) {
	page, size = normPage(page, size)
	total, err := s.countHistoryBooks(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, historyQuery, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("store: reading history: %w", err)
	}
	defer rows.Close()

	items := make([]HistoryRow, 0, size)
	for rows.Next() {
		var row HistoryRow
		if err := rows.Scan(&row.BookID, &row.BookTitle, &row.ChapterID,
			&row.ChapterTitle, &row.LastReadAt, &row.TotalSeconds, &row.Days); err != nil {
			return nil, 0, fmt.Errorf("store: scan reading history: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: reading history rows: %w", err)
	}
	return items, total, nil
}

// historyQuery 是历史列表的聚合查询：先按书聚合，再带出最后一次会话的章节。
//
// 章节用相关子查询取 (started_at, id) 最大的那条：同一秒内的多次上报用 id 兜底，
// 保证「最后读的那一章」稳定可复现。
const historyQuery = "SELECT g.bid, COALESCE(b.title, ''), " +
	"COALESCE(c.id, 0), COALESCE(c.title, ''), " +
	"g.last_at, g.total_seconds, g.days FROM (" +
	"SELECT s.book_id AS bid, MAX(s.started_at) AS last_at, SUM(s.seconds) AS total_seconds, " +
	"COUNT(DISTINCT date(s.started_at)) AS days, " +
	"(SELECT s2.chapter_id FROM reading_sessions s2 WHERE s2.book_id = s.book_id " +
	"ORDER BY s2.started_at DESC, s2.id DESC LIMIT 1) AS last_chapter " +
	"FROM reading_sessions s GROUP BY s.book_id) g " +
	"LEFT JOIN books b ON b.id = g.bid " +
	"LEFT JOIN chapters c ON c.id = g.last_chapter " +
	"ORDER BY g.last_at DESC, g.bid DESC LIMIT ? OFFSET ?"

// countHistoryBooks 数历史里出现过的书（分页总数的同口径）。
func (s *Store) countHistoryBooks(ctx context.Context) (int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT book_id) FROM reading_sessions").Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count reading history: %w", err)
	}
	return total, nil
}

// dateUTC 把时间截到 UTC 当天（只保留年月日，便于比较与拼日期串）。
func dateUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// weekStartOf 返回所在周的周一（UTC）；Go 的 Weekday 里周日是 0，要单独处理。
func weekStartOf(day time.Time) time.Time {
	offset := int(day.Weekday()) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return day.AddDate(0, 0, -offset)
}

// dateUTCString 把时间格式化成 YYYY-MM-DD（UTC）。
func dateUTCString(t time.Time) string { return dateUTC(t).Format("2006-01-02") }
