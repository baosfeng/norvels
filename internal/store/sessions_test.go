package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// 阅读会话 / 统计 / 历史的口径：时长 1–86400 s、章节必须属于书；
// 按日聚合与连续天数按日期取、缺失日补 0、今天没会话时连续天数为 0。

func TestAddReadingSessionValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	chapters := chapterIDsOf(t, ctx, st, bookID)
	otherBook := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	otherChapter := chapterIDsOf(t, ctx, st, otherBook)[0]

	requireErr(t, "book=0",
		st.AddReadingSession(ctx, ReadingSessionInput{ChapterID: chapters[0], Seconds: 60}), ErrInvalidArgument)
	requireErr(t, "chapter=0",
		st.AddReadingSession(ctx, ReadingSessionInput{BookID: bookID, Seconds: 60}), ErrInvalidArgument)
	for _, sec := range []int{0, -1, maxSessionSeconds + 1} {
		requireErr(t, fmt.Sprintf("seconds=%d", sec),
			st.AddReadingSession(ctx, ReadingSessionInput{BookID: bookID, ChapterID: chapters[0], Seconds: sec}),
			ErrInvalidArgument)
	}
	requireErr(t, "章节不属于该书",
		st.AddReadingSession(ctx, ReadingSessionInput{BookID: bookID, ChapterID: otherChapter, Seconds: 60}),
		ErrNotFound)
	requireErr(t, "非法 startedAt",
		st.AddReadingSession(ctx, ReadingSessionInput{
			BookID: bookID, ChapterID: chapters[0], Seconds: 60, StartedAt: "昨天",
		}), ErrInvalidArgument)
	requireEqual(t, "非法上报不落行", countRows(t, ctx, st, "reading_sessions"), 0)
}

func TestAddReadingSessionStores(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	chapters := chapterIDsOf(t, ctx, st, bookID)
	seedSession(t, ctx, st, bookID, chapters[0], 60, "2026-10-20T08:00:00Z")
	seedSession(t, ctx, st, bookID, chapters[1], 30, "")

	var (
		startedAt string
		seconds   int
	)
	requireOK(t, "读会话", st.DB().QueryRowContext(ctx,
		"SELECT started_at, seconds FROM reading_sessions ORDER BY id DESC LIMIT 1").
		Scan(&startedAt, &seconds))
	requireEqual(t, "缺省 startedAt 落库秒数", seconds, 30)
	requireEqual(t, "缺省 startedAt 非空", startedAt != "", true)
	requireEqual(t, "会话行数", countRows(t, ctx, st, "reading_sessions"), 2)
}

func TestReadingStatsTotals(t *testing.T) {
	ctx := context.Background()
	st, bookID, chapterID := seedStatsBook(t, ctx)
	today := dateUTC(nowUTC())
	offsets := []int{0, -1, -2}
	seconds := []int{60, 30, 10}
	expectedWeek := 0
	for i, off := range offsets {
		seedSession(t, ctx, st, bookID, chapterID, seconds[i], sessionTime(today, off))
		if !today.AddDate(0, 0, off).Before(mondayUTC(today)) {
			expectedWeek += seconds[i]
		}
	}

	stats, err := st.ReadingStats(ctx, 3)
	requireOK(t, "ReadingStats", err)
	requireEqual(t, "累计秒数", stats.TotalSeconds, 100)
	requireEqual(t, "今日秒数", stats.TodaySeconds, 60)
	requireEqual(t, "本周秒数", stats.WeekSeconds, expectedWeek)
	requireEqual(t, "连续天数", stats.Streak, 3)
	requireEqual(t, "日历行数", len(stats.Days), 3)
	requireEqual(t, "日历首日秒数", stats.Days[0].Seconds, 10)
	requireEqual(t, "日历末日秒数", stats.Days[2].Seconds, 60)
	requireEqual(t, "日历末日日期", stats.Days[2].Date, dateUTCString(today))
}

func TestReadingStatsCalendarWindows(t *testing.T) {
	ctx := context.Background()
	st, bookID, chapterID := seedStatsBook(t, ctx)
	today := dateUTC(nowUTC())
	seedSession(t, ctx, st, bookID, chapterID, 60, sessionTime(today, 0))

	wide, err := st.ReadingStats(ctx, 5)
	requireOK(t, "ReadingStats(5)", err)
	requireEqual(t, "5 天窗口行数", len(wide.Days), 5)
	requireEqual(t, "5 天窗口首日补 0", wide.Days[0].Seconds, 0)

	def, err := st.ReadingStats(ctx, 0)
	requireOK(t, "ReadingStats(0)", err)
	requireEqual(t, "默认窗口行数", len(def.Days), defaultStatsDays)

	clamped, err := st.ReadingStats(ctx, 999)
	requireOK(t, "ReadingStats(999)", err)
	requireEqual(t, "超上限窗口行数", len(clamped.Days), maxStatsDays)
}

func TestReadingStatsStreakResets(t *testing.T) {
	ctx := context.Background()
	st, bookID, chapterID := seedStatsBook(t, ctx)
	today := dateUTC(nowUTC())
	// 只读昨天与前天：今天没上报 → 连续天数按 0（不假装还在连续）。
	seedSession(t, ctx, st, bookID, chapterID, 30, sessionTime(today, -1))
	seedSession(t, ctx, st, bookID, chapterID, 30, sessionTime(today, -2))

	stats, err := st.ReadingStats(ctx, 7)
	requireOK(t, "ReadingStats", err)
	requireEqual(t, "今天无会话时 streak", stats.Streak, 0)
	requireEqual(t, "今天无会话时 today", stats.TodaySeconds, 0)
}

func TestReadingHistoryAggregates(t *testing.T) {
	ctx := context.Background()
	st, first, second, firstChapters := seedHistoryFixture(t, ctx)
	today := dateUTC(nowUTC())

	items, total, err := st.ReadingHistory(ctx, 1, 10)
	requireOK(t, "ReadingHistory", err)
	requireEqual(t, "历史 total", total, 2)
	requireEqual(t, "历史行数", len(items), 2)
	requireEqual(t, "首行是最后读的书", items[0].BookID, second)

	row := items[1]
	requireEqual(t, "书 id", row.BookID, first)
	requireEqual(t, "书名", row.BookTitle, "示例书甲")
	requireEqual(t, "累计秒数", row.TotalSeconds, 180)
	requireEqual(t, "有会话的天数", row.Days, 2)
	requireEqual(t, "最后章节", row.ChapterID, firstChapters[1])
	requireEqual(t, "最后阅读时间", row.LastReadAt,
		dateUTCString(today.AddDate(0, 0, -1))+"T12:00:00.000Z")
}

func TestReadingHistoryPaging(t *testing.T) {
	ctx := context.Background()
	st, first, _, _ := seedHistoryFixture(t, ctx)

	page2, total, err := st.ReadingHistory(ctx, 2, 1)
	requireOK(t, "ReadingHistory(page=2)", err)
	requireEqual(t, "分页 total", total, 2)
	requireEqual(t, "第二页行数", len(page2), 1)
	requireEqual(t, "第二页书", page2[0].BookID, first)
}

func TestReadingHistoryDirtyData(t *testing.T) {
	ctx := context.Background()
	st, _, _, _ := seedHistoryFixture(t, ctx)
	today := dateUTC(nowUTC())
	// 书/章节已被清掉但会话还在（表上没有外键）：列表按空标题降级而不是整页失败。
	mustExec(t, ctx, st, fmt.Sprintf(
		"INSERT INTO reading_sessions(book_id, chapter_id, started_at, seconds) "+
			"VALUES (999999, 888888, '%s', 5)", sessionTime(today, 0)))

	items, total, err := st.ReadingHistory(ctx, 1, 10)
	requireOK(t, "ReadingHistory(脏数据)", err)
	requireEqual(t, "脏数据后 total", total, 3)
	requireEqual(t, "脏数据行书 id", items[0].BookID, int64(999999))
	requireEqual(t, "脏数据行书名", items[0].BookTitle, "")
	requireEqual(t, "脏数据行章名", items[0].ChapterTitle, "")
}

// seedStatsBook 建一本带 3 章的示例书，返回 store 与书/首章 id。
func seedStatsBook(t *testing.T, ctx context.Context) (*Store, int64, int64) {
	t.Helper()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	return st, bookID, chapterIDsOf(t, ctx, st, bookID)[0]
}

// seedHistoryFixture 建两本示例书并写三条会话（甲两天前/昨天，乙今天）。
func seedHistoryFixture(t *testing.T, ctx context.Context) (*Store, int64, int64, []int64) {
	t.Helper()
	st := newTestStore(t)
	first := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	second := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	firstChapters := chapterIDsOf(t, ctx, st, first)
	today := dateUTC(nowUTC())
	seedSession(t, ctx, st, first, firstChapters[0], 60, sessionTime(today, -2))
	seedSession(t, ctx, st, first, firstChapters[1], 120, sessionTime(today, -1))
	seedSession(t, ctx, st, second, chapterIDsOf(t, ctx, st, second)[0], 30, sessionTime(today, 0))
	return st, first, second, firstChapters
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
