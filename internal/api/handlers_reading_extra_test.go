package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// sendReading 发起一次带 JSON 请求体的阅读接口调用。
func sendReading(t *testing.T, e *testEnv, method, target, payload string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(method, target, strings.NewReader(payload),
		map[string]string{"Content-Type": "application/json"})
}

// firstChapterID 取某本书的第一章 id（会话/进度测试用）。
func firstChapterID(t *testing.T, e *testEnv, bookID int64) int64 {
	t.Helper()
	page, err := e.st.ListChapters(context.Background(),
		store.ChapterQuery{BookID: bookID, From: 1, Limit: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("ListChapters(%d): err=%v items=%d", bookID, err, len(page.Items))
	}
	return page.Items[0].ID
}

// bookNewChapters 读详情里的追更角标。
func bookNewChapters(t *testing.T, e *testEnv, bookID int64) int {
	t.Helper()
	detail := decodeBody[store.BookDetail](t, e.get(fmt.Sprintf("/api/books/%d", bookID)))
	return detail.NewChapters
}

func TestProgressParaIndex(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("进度之书", "进度作者")
	chapterID := firstChapterID(t, e, bookID)

	rr := e.putJSON("/api/reading/progress", map[string]any{
		"bookId": bookID, "chapterId": chapterID, "percent": 0.2, "paraIndex": 7,
	})
	requireStatus(t, rr, http.StatusNoContent)
	items := e.progressItems()
	if len(items.Items) != 1 || items.Items[0].ParaIndex != 7 {
		t.Fatalf("进度 paraIndex = %+v，期望 7", items.Items)
	}

	requireError(t, e.putJSON("/api/reading/progress", map[string]any{
		"bookId": bookID, "chapterId": chapterID, "percent": 0.2, "paraIndex": -1,
	}), http.StatusBadRequest, codeInvalidParam)
}

func TestAckNewChapters(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("追更之书", "追更作者")
	if err := e.st.AddNewChapters(context.Background(), bookID, 3); err != nil {
		t.Fatalf("AddNewChapters: %v", err)
	}
	if got := bookNewChapters(t, e, bookID); got != 3 {
		t.Fatalf("ack 前 newChapters = %d，期望 3", got)
	}
	rr := e.do(http.MethodPost, fmt.Sprintf("/api/books/%d/ack-new-chapters", bookID), nil, nil)
	requireStatus(t, rr, http.StatusNoContent)
	if got := bookNewChapters(t, e, bookID); got != 0 {
		t.Fatalf("ack 后 newChapters = %d，期望 0", got)
	}
	requireError(t, e.do(http.MethodPost, "/api/books/999999/ack-new-chapters", nil, nil),
		http.StatusNotFound, codeNotFound)
}

func TestAddReadingSessionValidation(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("会话之书", "会话作者")
	otherID := e.seedBook("另一本书", "另一作者")
	chapterID := firstChapterID(t, e, bookID)
	otherChapter := firstChapterID(t, e, otherID)
	target := "/api/reading/sessions"

	requireError(t, sendReading(t, e, http.MethodPost, target,
		`{"bookId":0,"chapterId":1,"seconds":60}`), http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendReading(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":0}`, bookID, chapterID)),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendReading(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":86401}`, bookID, chapterID)),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendReading(t, e, http.MethodPost, target,
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":60}`, bookID, otherChapter)),
		http.StatusNotFound, codeNotFound)
	requireError(t, sendReading(t, e, http.MethodPost, target, `not json`),
		http.StatusBadRequest, codeInvalidParam)
}

func TestReadingSessionAndHistory(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("历史之书", "历史作者")
	chapterID := firstChapterID(t, e, bookID)

	requireStatus(t, sendReading(t, e, http.MethodPost, "/api/reading/sessions",
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":120}`, bookID, chapterID)),
		http.StatusNoContent)
	requireStatus(t, sendReading(t, e, http.MethodPost, "/api/reading/sessions",
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":60,"startedAt":"2026-10-01T10:00:00Z"}`,
			bookID, chapterID)), http.StatusNoContent)

	rr := e.get("/api/reading/history")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[readingHistoryResponse](t, rr)
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("历史 = %+v，期望 1 本书", body)
	}
	row := body.Items[0]
	if row.BookID != bookID || row.ChapterID != chapterID || row.TotalSeconds != 180 || row.Days != 2 {
		t.Fatalf("历史行 = %+v，期望秒数 180 / 天数 2", row)
	}
	if row.BookTitle == "" || row.ChapterTitle == "" || row.LastReadAt == "" {
		t.Fatalf("历史行联表字段缺失: %+v", row)
	}
}

func TestReadingHistoryPagination(t *testing.T) {
	e := newEnv(t)
	for _, title := range []string{"历史甲", "历史乙"} {
		id := e.seedBook(title, "历史作者")
		if err := e.st.AddReadingSession(context.Background(), store.ReadingSessionInput{
			BookID: id, ChapterID: firstChapterID(t, e, id), Seconds: 10,
		}); err != nil {
			t.Fatalf("AddReadingSession: %v", err)
		}
	}
	first := decodeBody[readingHistoryResponse](t, e.get("/api/reading/history?page=1&size=1"))
	if first.Total != 2 || len(first.Items) != 1 {
		t.Fatalf("第一页 = %+v", first)
	}
	second := decodeBody[readingHistoryResponse](t, e.get("/api/reading/history?page=2&size=1"))
	if len(second.Items) != 1 || second.Items[0].BookID == first.Items[0].BookID {
		t.Fatalf("第二页 = %+v", second.Items)
	}
	requireError(t, e.get("/api/reading/history?page=0"), http.StatusBadRequest, codeInvalidParam)
}

func TestReadingHistoryEmpty(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/reading/history")
	requireStatus(t, rr, http.StatusOK)
	if !strings.Contains(rr.Body.String(), `"items":[]`) {
		t.Fatalf("空历史 items 应为 []，body=%s", rr.Body.String())
	}
}

func TestReadingStats(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("统计之书", "统计作者")
	chapterID := firstChapterID(t, e, bookID)
	requireStatus(t, sendReading(t, e, http.MethodPost, "/api/reading/sessions",
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":120}`, bookID, chapterID)),
		http.StatusNoContent)

	rr := e.get("/api/stats/reading?days=7&tz=480")
	requireStatus(t, rr, http.StatusOK)
	stats := decodeBody[store.ReadingStats](t, rr)
	if stats.TotalSeconds != 120 || stats.TodaySeconds != 120 || stats.Streak != 1 {
		t.Fatalf("统计 = %+v，期望今日/累计 120、连续 1 天", stats)
	}
	if stats.WeekSeconds < 120 || len(stats.Days) != 7 || stats.Days[6].Seconds != 120 {
		t.Fatalf("热力图 = %+v", stats.Days)
	}
}

func TestReadingStatsBounds(t *testing.T) {
	e := newEnv(t)
	defaults := decodeBody[store.ReadingStats](t, e.get("/api/stats/reading"))
	if len(defaults.Days) != 90 || defaults.TotalSeconds != 0 || defaults.Streak != 0 {
		t.Fatalf("默认统计 = %+v，期望 90 天空窗", defaults)
	}
	requireError(t, e.get("/api/stats/reading?days=0"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/stats/reading?days=366"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/stats/reading?tz=900"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/stats/reading?tz=abc"), http.StatusBadRequest, codeInvalidParam)
}

// TestReadingStatsTZCrossMidnight 证明 tz 真的参与聚合：
// UTC 前一天 16:30 的会话在东八区（+480 分钟）属于「当天 00:30」。
func TestReadingStatsTZCrossMidnight(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("跨零点之书", "时效作者")
	chapterID := firstChapterID(t, e, bookID)
	// 取「东八区的今天」00:30 对应的 UTC 时刻（= 前一天 16:30 UTC）：
	// 同一个会话在 UTC 口径下属于前一天、在东八区口径下属于今天。
	localNow := time.Now().UTC().Add(8 * time.Hour)
	localMidnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	started := localMidnight.Add(30*time.Minute - 8*time.Hour)
	requireStatus(t, sendReading(t, e, http.MethodPost, "/api/reading/sessions",
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"seconds":120,"startedAt":%q}`,
			bookID, chapterID, started.Format(time.RFC3339))), http.StatusNoContent)

	local := decodeBody[store.ReadingStats](t, e.get("/api/stats/reading?days=7&tz=480"))
	if local.TodaySeconds != 120 || local.Streak != 1 {
		t.Fatalf("东八区统计 = %+v，期望今日 120、连续 1 天", local)
	}
	last := local.Days[len(local.Days)-1]
	if last.Date != localNow.Format("2006-01-02") || last.Seconds != 120 {
		t.Fatalf("东八区热力图末尾 = %+v，期望落在本地今天且 120 秒", last)
	}

	utc := decodeBody[store.ReadingStats](t, e.get("/api/stats/reading?days=7"))
	sessionDate := started.Format("2006-01-02")
	if sessionDate == last.Date {
		t.Fatalf("构造失败：会话 UTC 日期 %s 与本地今天相同", sessionDate)
	}
	if got := daySecondsOf(utc.Days, sessionDate); got != 120 {
		t.Fatalf("UTC 统计里 %s = %d，期望 120（同一会话按 UTC 日期归档）", sessionDate, got)
	}
}

// daySecondsOf 取热力图里某一天的秒数；没有该日返回 0。
func daySecondsOf(days []store.DaySeconds, date string) int {
	for _, d := range days {
		if d.Date == date {
			return d.Seconds
		}
	}
	return 0
}
