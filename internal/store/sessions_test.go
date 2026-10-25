package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// 阅读会话 / 统计 / 历史的口径：时长 1–86400 s、章节必须属于书；
// 按日聚合与连续天数都走 UTC 日期；历史按书聚合并带出最后读的那一章。

func TestAddReadingSession(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	chapters := chapterIDsOf(t, ctx, st, bookID)
	otherBook := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	otherChapter := chapterIDsOf(t, ctx, st, otherBook)[0]

	if err := st.AddReadingSession(ctx, ReadingSessionInput{BookID: 0, ChapterID: chapters[0], Seconds: 60}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("book=0 err = %v", err)
	}
	if err := st.AddReadingSession(ctx, ReadingSessionInput{BookID: bookID, ChapterID: 0, Seconds: 60}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("chapter=0 err = %v", err)
	}
	for _, sec := range []int{0, -1, maxSessionSeconds + 1} {
		if err := st.AddReadingSession(ctx, ReadingSessionInput{BookID: bookID, ChapterID: chapters[0], Seconds: sec}); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("seconds=%d err = %v", sec, err)
		}
	}
	if err := st.AddReadingSession(ctx, ReadingSessionInput{BookID: bookID, ChapterID: otherChapter, Seconds: 60}); !errors.Is(err, ErrNotFound) {
		t.Errorf("章节不属于该书 err = %v，期望 ErrNotFound", err)
	}
	if err := st.AddReadingSession(ctx, ReadingSessionInput{
		BookID: bookID, ChapterID: chapters[0], Seconds: 60, StartedAt: "昨天",
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("非法 startedAt err = %v，期望 ErrInvalidArgument", err)
	}

	seedSession(t, ctx, st, bookID, chapters[0], 60, "2026-10-20T08:00:00Z")
	seedSession(t, ctx, st, bookID, chapters[1], 30, "")
	var (
		startedAt string
		seconds   int
	)
	if err := st.DB().QueryRowContext(ctx,
		"SELECT started_at, seconds FROM reading_sessions ORDER BY id DESC LIMIT 1").
		Scan(&startedAt, &seconds); err != nil {
		t.Fatalf("读会话: %v", err)
	}
	if seconds != 30 || startedAt == "" {
		t.Errorf("缺省 startedAt 未落库: seconds=%d started_at=%q", seconds, startedAt)
	}
	if n := countRows(t, ctx, st, "reading_sessions"); n != 2 {
		t.Errorf("会话行数 = %d，期望 2", n)
	}
}

func TestReadingStats(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]
	chapters := chapterIDsOf(t, ctx, st, bookID)
	today := dateUTC(nowUTC())
	days := []struct {
		offset  int
		seconds int
	}{{0, 60}, {-1, 30}, {-2, 10}}

	expectedWeek := 0
	monday := mondayUTC(today)
	for _, d := range days {
		seedSession(t, ctx, st, bookID, chapters[0], d.seconds, sessionTime(today, d.offset))
		if !dateUTC(today.AddDate(0, 0, d.offset)).Before(monday) {
			expectedWeek += d.seconds
		}
	}
	stats, err := st.ReadingStats(ctx, 3)
	if err != nil {
		t.Fatalf("ReadingStats: %v", err)
	}
	if stats.TotalSeconds != 100 || stats.TodaySeconds != 60 || stats.WeekSeconds != expectedWeek {
		t.Errorf("统计 = %+v，期望 total=100 today=60 week=%d", stats, expectedWeek)
	}
	if stats.Streak != 3 {
		t.Errorf("连续天数 = %d，期望 3", stats.Streak)
	}
	if len(stats.Days) != 3 || stats.Days[0].Seconds != 10 || stats.Days[2].Seconds != 60 {
		t.Errorf("日历 = %+v，期望按时间升序且缺失日补 0", stats.Days)
	}
	if got := stats.Days[2].Date; got != dateUTCString(today) {
		t.Errorf("最后一天 = %q，期望今天 %q", got, dateUTCString(today))
	}

	wide, err := st.ReadingStats(ctx, 5)
	if err != nil || len(wide.Days) != 5 || wide.Days[0].Seconds != 0 {
		t.Errorf("5 天窗口 = %+v err=%v，期望首日补 0", wide.Days, err)
	}
	if def, err := st.ReadingStats(ctx, 0); err != nil || len(def.Days) != defaultStatsDays {
		t.Errorf("默认窗口 = %d 天 err=%v，期望 %d", len(def.Days), err, defaultStatsDays)
	}
	if clamped, err := st.ReadingStats(ctx, 999); err != nil || len(clamped.Days) != maxStatsDays {
		t.Errorf("超上限窗口 = %d 天 err=%v，期望 %d", len(clamped.Days), err, maxStatsDays)
	}
}

func TestReadingStatsStreakResets(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]
	chapterID := chapterIDsOf(t, ctx, st, bookID)[0]
	today := dateUTC(nowUTC())
	// 只读昨天与前天：今天没上报 → 连续天数按 0（不假装还在连续）。
	seedSession(t, ctx, st, bookID, chapterID, 30, sessionTime(today, -1))
	seedSession(t, ctx, st, bookID, chapterID, 30, sessionTime(today, -2))

	stats, err := st.ReadingStats(ctx, 7)
	if err != nil {
		t.Fatalf("ReadingStats: %v", err)
	}
	if stats.Streak != 0 || stats.TodaySeconds != 0 {
		t.Errorf("今天无会话：streak=%d today=%d，期望 0/0", stats.Streak, stats.TodaySeconds)
	}
}

func TestReadingHistory(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	second := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	firstChapters := chapterIDsOf(t, ctx, st, first)
	secondChapters := chapterIDsOf(t, ctx, st, second)
	today := dateUTC(nowUTC())
	seedSession(t, ctx, st, first, firstChapters[0], 60, sessionTime(today, -2))
	seedSession(t, ctx, st, first, firstChapters[1], 120, sessionTime(today, -1))
	seedSession(t, ctx, st, second, secondChapters[0], 30, sessionTime(today, 0))

	items, total, err := st.ReadingHistory(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ReadingHistory: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("历史 = %d 行 total=%d，期望 2/2", len(items), total)
	}
	if items[0].BookID != second {
		t.Errorf("首行应是最后读的 %d，实际 %d", second, items[0].BookID)
	}
	row := items[1]
	if row.BookID != first || row.TotalSeconds != 180 || row.Days != 2 {
		t.Errorf("示例书聚合 = %+v，期望 180 s / 2 天", row)
	}
	if row.ChapterID != firstChapters[1] {
		t.Errorf("最后章节 = %d，期望 %d", row.ChapterID, firstChapters[1])
	}
	if want := dateUTCString(today.AddDate(0, 0, -1)) + "T12:00:00.000Z"; row.LastReadAt != want {
		t.Errorf("最后阅读时间 = %q，期望 %q（库内时间统一毫秒 UTC 文本）", row.LastReadAt, want)
	}
	page2, total, err := st.ReadingHistory(ctx, 2, 1)
	if err != nil || total != 2 || len(page2) != 1 || page2[0].BookID != first {
		t.Errorf("第二页 = %+v total=%d err=%v", page2, total, err)
	}

	// 脏数据：书/章节已被清掉但会话还在（无外键），列表按空标题降级而不是整页失败。
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO reading_sessions(book_id, chapter_id, started_at, seconds) "+
			"VALUES (999999, 888888, '%s', 5)", sessionTime(today, 0)))
	items, total, err = st.ReadingHistory(ctx, 1, 10)
	if err != nil || total != 3 {
		t.Fatalf("脏数据后历史 total=%d err=%v，期望 3", total, err)
	}
	if items[0].BookID != 999999 || items[0].BookTitle != "" || items[0].ChapterTitle != "" {
		t.Errorf("脏数据行 = %+v，期望空标题降级", items[0])
	}
}

// seedSession 写一条阅读会话（startedAt 为空时用服务端当前时间）。
func seedSession(t *testing.T, ctx context.Context, st *Store, bookID, chapterID int64,
	seconds int, startedAt string) {
	t.Helper()
	if err := st.AddReadingSession(ctx, ReadingSessionInput{
		BookID: bookID, ChapterID: chapterID, Seconds: seconds, StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("AddReadingSession: %v", err)
	}
}

// sessionTime 返回相对 today 偏移 offset 天的 12:00 UTC 时间文本。
func sessionTime(today time.Time, offset int) string {
	return dateUTCString(today.AddDate(0, 0, offset)) + "T12:00:00Z"
}

// mondayUTC 返回 today 所在周的周一（UTC），用于独立推算 weekSeconds。
func mondayUTC(today time.Time) time.Time {
	monday := today
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, -1)
	}
	return monday
}
