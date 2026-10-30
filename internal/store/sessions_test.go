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

	stats, err := st.ReadingStats(ctx, 3, 0)
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

	wide, err := st.ReadingStats(ctx, 5, 0)
	requireOK(t, "ReadingStats(5)", err)
	requireEqual(t, "5 天窗口行数", len(wide.Days), 5)
	requireEqual(t, "5 天窗口首日补 0", wide.Days[0].Seconds, 0)

	def, err := st.ReadingStats(ctx, 0, 0)
	requireOK(t, "ReadingStats(0)", err)
	requireEqual(t, "默认窗口行数", len(def.Days), defaultStatsDays)

	clamped, err := st.ReadingStats(ctx, 999, 0)
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

	stats, err := st.ReadingStats(ctx, 7, 0)
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
		"INSERT INTO reading_sessions(user_id, book_id, chapter_id, started_at, seconds) "+
			"VALUES (%d, 999999, 888888, '%s', 5)", defaultUserID, sessionTime(today, 0)))

	items, total, err := st.ReadingHistory(ctx, 1, 10)
	requireOK(t, "ReadingHistory(脏数据)", err)
	requireEqual(t, "脏数据后 total", total, 3)
	requireEqual(t, "脏数据行书 id", items[0].BookID, int64(999999))
	requireEqual(t, "脏数据行书名", items[0].BookTitle, "")
	requireEqual(t, "脏数据行章名", items[0].ChapterTitle, "")
}

func TestDeleteReadingHistoryOnlyCurrentUser(t *testing.T) {
	ctx := context.Background()
	st, first, second, _ := seedHistoryFixture(t, ctx)
	other := mustRegister(t, st, "读者乙")
	addSessionFor(t, ctx, st, other.ID, first, 300)
	saveProgressFor(t, ctx, st, defaultUserID, first)

	requireOK(t, "DeleteReadingHistory", st.DeleteReadingHistory(ctx, first))
	requireEqual(t, "甲在这本书上的会话清空", mineSessionsOf(t, ctx, st, first), 0)
	// 别的账号在这本书上的会话仍在：只删 ScopeUserID(ctx) 的行。
	requireEqual(t, "乙的会话仍在", sessionsOf(t, ctx, st, other.ID), 1)
	// 甲的另一本书不受影响：按书删，不按账号清空。
	requireEqual(t, "甲另一本书的会话仍在", mineSessionsOf(t, ctx, st, second), 1)
	// 不级联：进度保留（删历史是清时长明细，不是让进度回退）。
	requireEqual(t, "进度行数不变", countRows(t, ctx, st, "reading_progress"), 1)
}

func TestDeleteReadingHistoryMissing(t *testing.T) {
	ctx := context.Background()
	st, first, _, _ := seedHistoryFixture(t, ctx)
	other := mustRegister(t, st, "读者丙")
	// 书存在，但丙自己在这本书上没有会话（甲读过）：不能删别人的，也不当成功。
	requireErr(t, "别人的历史不算我的", st.DeleteReadingHistory(WithUserID(ctx, other.ID), first), ErrNotFound)
	requireErr(t, "book=0", st.DeleteReadingHistory(ctx, 0), ErrInvalidArgument)
	requireEqual(t, "失败的删除不动数据", mineSessionsOf(t, ctx, st, first), 2)
}

func TestClearReadingHistoryOnlyCurrentUser(t *testing.T) {
	ctx := context.Background()
	st, first, second, _ := seedHistoryFixture(t, ctx)
	other := mustRegister(t, st, "读者丁")
	addSessionFor(t, ctx, st, other.ID, first, 300)
	saveProgressFor(t, ctx, st, defaultUserID, first)

	requireOK(t, "ClearReadingHistory", st.ClearReadingHistory(ctx))
	requireEqual(t, "甲的历史清空", mineSessionsOf(t, ctx, st, 0), 0)
	requireEqual(t, "乙的会话仍在", sessionsOf(t, ctx, st, other.ID), 1)
	requireEqual(t, "清空后进度保留", countRows(t, ctx, st, "reading_progress"), 1)
	requireEqual(t, "清空不动书", countRows(t, ctx, st, "books") >= 2, true)

	// 幂等：已经是空的再清一次仍然成功（不返回 ErrNotFound）。
	requireOK(t, "空库再清一次", st.ClearReadingHistory(ctx))
	requireOK(t, "乙再清自己的", st.ClearReadingHistory(WithUserID(ctx, other.ID)))
	requireEqual(t, "乙清空后也无会话", sessionsOf(t, ctx, st, other.ID), 0)
	requireEqual(t, "清空后第二本书也没了", mineSessionsOf(t, ctx, st, second), 0)
}

// mineSessionsOf 数「默认账号（甲）在指定书上」的会话行数；bookID=0 表示不按书过滤。
func mineSessionsOf(t *testing.T, ctx context.Context, st *Store, bookID int64) int {
	t.Helper()
	query := "SELECT COUNT(*) FROM reading_sessions WHERE user_id = ?"
	args := []any{defaultUserID}
	if bookID != 0 {
		query += " AND book_id = ?"
		args = append(args, bookID)
	}
	var n int
	requireOK(t, "数甲的会话行", st.DB().QueryRowContext(ctx, query, args...).Scan(&n))
	return n
}

// sessionsOf 数某个账号的阅读会话行数。
func sessionsOf(t *testing.T, ctx context.Context, st *Store, userID int64) int {
	t.Helper()
	query := "SELECT COUNT(*) FROM reading_sessions"
	args := []any{}
	if userID != 0 {
		query += " WHERE user_id = ?"
		args = append(args, userID)
	}
	var n int
	requireOK(t, "数会话行", st.DB().QueryRowContext(ctx, query, args...).Scan(&n))
	return n
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

func TestReadingStatsTZOffset(t *testing.T) {
	ctx := context.Background()
	st, bookID, chapterID := seedStatsBook(t, ctx)
	localToday := dateUTC(dayInOffset(nowUTC(), 480))
	// 东八区「今天 00:30」= UTC 前一天 16:30：跨零点的会话必须算进本地今天。
	utcStarted := localToday.AddDate(0, 0, -1).Add(16*time.Hour + 30*time.Minute)
	seedSession(t, ctx, st, bookID, chapterID, 60, formatTime(utcStarted))

	stats, err := st.ReadingStats(ctx, 7, 480)
	requireOK(t, "ReadingStats(tz=480)", err)
	requireEqual(t, "东八区今日秒数", stats.TodaySeconds, 60)
	requireEqual(t, "东八区连续天数", stats.Streak, 1)

	utcStats, err := st.ReadingStats(ctx, 7, 0)
	requireOK(t, "ReadingStats(tz=0)", err)
	wantUTC := 0
	if dateUTC(utcStarted).Equal(dateUTC(nowUTC())) {
		wantUTC = 60
	}
	requireEqual(t, "UTC 今日秒数", utcStats.TodaySeconds, wantUTC)

	_, err = st.ReadingStats(ctx, 7, 900)
	requireErr(t, "tz 越界", err, ErrInvalidArgument)
}
