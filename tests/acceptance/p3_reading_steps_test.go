package acceptance

// P3 阅读时长、阅读历史与追更角标清零的验收断言。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func registerP3ReadingSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我把这本书的阅读时长上报为 (\d+) 秒$`, func(ctx context.Context, n string) error {
		return w.reportSession(atoiSafe(n), time.Time{})
	})
	sc.Step(`^我上报一条东八区今天凌晨的阅读会话$`, func(ctx context.Context) error {
		return w.reportLocalMidnightSession()
	})
	sc.Step(`^阅读统计的今日时长应当至少为 (\d+) 秒$`, func(ctx context.Context, min string) error {
		return w.assertTodaySeconds(0, atoiSafe(min))
	})
	sc.Step(`^以东八区口径查询的今日时长应当至少为 (\d+) 秒$`, func(ctx context.Context, min string) error {
		return w.assertTodaySeconds(480, atoiSafe(min))
	})
	sc.Step(`^阅读历史里这本书应当累计 (\d+) 秒$`, func(ctx context.Context, want string) error {
		return w.assertHistorySeconds(atoiSafe(want))
	})
	sc.Step(`^阅读历史里这本书的天数应当至少为 (\d+) 天$`, func(ctx context.Context, min string) error {
		return w.assertHistoryDays(atoiSafe(min))
	})
	sc.Step(`^这本书的追更角标应当为 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertNewChapters(atoiSafe(want))
	})
	sc.Step(`^我把这本书的新章节标记为已读$`, func(ctx context.Context) error {
		return w.ackNewChapters()
	})
}

// reportSession 上报一次阅读时长；started 为零值时由服务端取当前时间。
func (w *world) reportSession(seconds int, started time.Time) error {
	payload := map[string]any{"bookId": w.lastBookID, "chapterId": w.firstChapter, "seconds": seconds}
	if !started.IsZero() {
		payload["startedAt"] = started.UTC().Format(time.RFC3339)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPost, "/api/reading/sessions", string(raw))
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("POST /api/reading/sessions 状态码 = %d，body=%s", code, body)
	}
	return nil
}

// reportLocalMidnightSession 上报「东八区今天 00:30」的 120 秒会话（UTC 前一天 16:30）。
func (w *world) reportLocalMidnightSession() error {
	localNow := time.Now().UTC().Add(8 * time.Hour)
	midnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	return w.reportSession(120, midnight.Add(30*time.Minute-8*time.Hour))
}

// assertTodaySeconds 按指定时区口径断言今日时长下限。
func (w *world) assertTodaySeconds(tzMinutes, min int) error {
	path := "/api/stats/reading?days=7"
	if tzMinutes != 0 {
		path = fmt.Sprintf("%s&tz=%d", path, tzMinutes)
	}
	var res struct {
		TodaySeconds int `json:"todaySeconds"`
	}
	code, body, err := w.getJSON(path, &res)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET %s 状态码 = %d，body=%s", path, code, body)
	}
	if res.TodaySeconds < min {
		return fmt.Errorf("今日时长 = %d 秒，期望至少 %d 秒", res.TodaySeconds, min)
	}
	return nil
}

// historyRow 是 GET /api/reading/history 的一行（只取断言字段）。
type historyRow struct {
	BookID       int64 `json:"bookId"`
	TotalSeconds int   `json:"totalSeconds"`
	Days         int   `json:"days"`
}

func (w *world) thisBookHistory() (historyRow, error) {
	var res struct {
		Items []historyRow `json:"items"`
	}
	code, body, err := w.getJSON("/api/reading/history?size=50", &res)
	if err != nil {
		return historyRow{}, err
	}
	if code != http.StatusOK {
		return historyRow{}, fmt.Errorf("GET /api/reading/history 状态码 = %d，body=%s", code, body)
	}
	for _, it := range res.Items {
		if it.BookID == w.lastBookID {
			return it, nil
		}
	}
	return historyRow{}, fmt.Errorf("阅读历史里没有当前书 %d（%+v）", w.lastBookID, res.Items)
}

func (w *world) assertHistorySeconds(want int) error {
	row, err := w.thisBookHistory()
	if err != nil {
		return err
	}
	if row.TotalSeconds != want {
		return fmt.Errorf("历史累计 = %d 秒，期望 %d", row.TotalSeconds, want)
	}
	return nil
}

func (w *world) assertHistoryDays(min int) error {
	row, err := w.thisBookHistory()
	if err != nil {
		return err
	}
	if row.Days < min {
		return fmt.Errorf("历史天数 = %d，期望至少 %d", row.Days, min)
	}
	return nil
}

func (w *world) assertNewChapters(want int) error {
	var detail struct {
		NewChapters int `json:"newChapters"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 书详情状态码 = %d，body=%s", code, body)
	}
	if detail.NewChapters != want {
		return fmt.Errorf("追更角标 = %d，期望 %d（书架快照：%s）", detail.NewChapters, want, w.shelfSnapshot())
	}
	return nil
}

// shelfSnapshot 在角标断言失败时留一份书架现场：书 id/书名/章数/角标。
// 追更偶发不累加时，用来区分「没追加」和「角标落在另一本书上」。
func (w *world) shelfSnapshot() string {
	var page struct {
		Items []map[string]any `json:"items"`
	}
	code, body, err := w.getJSON("/api/books?size=10", &page)
	if err != nil || code != http.StatusOK {
		return fmt.Sprintf("读取书架失败(code=%d, body=%s)", code, body)
	}
	parts := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		parts = append(parts, fmt.Sprintf("%v:%v:c%v:n%v:u%v:i%v", it["id"], it["title"], it["chapterCount"], it["newChapters"], it["updatedAt"], it["importedAt"]))
	}
	return fmt.Sprintf("lastBookID=%d [%s]", w.lastBookID, strings.Join(parts, " | "))
}

func (w *world) ackNewChapters() error {
	code, body, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/ack-new-chapters", w.lastBookID), "")
	if err != nil {
		return err
	}
	if code != http.StatusNoContent {
		return fmt.Errorf("ack 角标状态码 = %d，期望 204，body=%s", code, body)
	}
	return nil
}
