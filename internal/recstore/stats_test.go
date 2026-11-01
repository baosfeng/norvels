package recstore

import (
	"context"
	"errors"
	"math"
	"testing"
)

// dayAgo 返回 n 天前的 UTC 日期文本（book_daily.day 的口径）。
func dayAgo(n int) string { return dayOf(formatTime(nowUTC().AddDate(0, 0, -n))) }

func TestDailyStatsEmptyFillsEveryDay(t *testing.T) {
	s := openTemp(t)
	got, err := s.DailyStats(context.Background(), 3)
	if err != nil {
		t.Fatalf("DailyStats: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("行数 = %d，期望 3（缺日补 0）", len(got))
	}
	wantDates := []string{dayAgo(2), dayAgo(1), dayAgo(0)}
	for i, d := range got {
		if d.Date != wantDates[i] {
			t.Fatalf("第 %d 行日期 = %q，期望 %q（升序、含今天）", i, d.Date, wantDates[i])
		}
		if d.Impressions != 0 || d.Clicks != 0 || d.Reads != 0 || d.Seconds != 0 {
			t.Fatalf("空库第 %d 行应为 0: %+v", i, d)
		}
	}
}

func TestDailyStatsAggregatesByDay(t *testing.T) {
	s := openTemp(t)
	now := nowUTC()
	yesterday := formatTime(now.AddDate(0, 0, -1))
	recordEvents(t, s, 1,
		Event{Kind: KindImpression, BookID: 1},
		Event{Kind: KindImpression, BookID: 2},
		Event{Kind: KindRead, BookID: 1, Value: 600},
		Event{Kind: KindClick, BookID: 3, TS: yesterday},
	)
	got, err := s.DailyStats(context.Background(), 3)
	if err != nil {
		t.Fatalf("DailyStats: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("行数 = %d，期望 3", len(got))
	}
	checkDailyRow(t, got[2], dayAgo(0), DailyStat{Impressions: 2, Reads: 1, Seconds: 600})
	checkDailyRow(t, got[1], dayAgo(1), DailyStat{Clicks: 1})
	checkDailyRow(t, got[0], dayAgo(2), DailyStat{})
}

// checkDailyRow 断言某一格的日期与四个计数（want 只填期望非零的那几项）。
func checkDailyRow(t *testing.T, got DailyStat, wantDate string, want DailyStat) {
	t.Helper()
	if got.Date != wantDate {
		t.Fatalf("日期 = %q，期望 %q", got.Date, wantDate)
	}
	if got.Impressions != want.Impressions || got.Clicks != want.Clicks ||
		got.Reads != want.Reads || got.Seconds != want.Seconds {
		t.Fatalf("%s 汇总 = %+v，期望 %+v", wantDate, got, want)
	}
}

func TestDailyStatsClampsDays(t *testing.T) {
	s := openTemp(t)
	if got, err := s.DailyStats(context.Background(), 0); err != nil || len(got) != 1 {
		t.Fatalf("days=0 应夹到 1 天，得到 %d err=%v", len(got), err)
	}
	if got, err := s.DailyStats(context.Background(), maxStatsDays+50); err != nil || len(got) != maxStatsDays {
		t.Fatalf("超上限应夹到 %d 天，得到 %d err=%v", maxStatsDays, len(got), err)
	}
}

func TestSummarySumsWindowAndCumulativeShelves(t *testing.T) {
	s := openTemp(t)
	old := formatTime(nowUTC().AddDate(0, 0, -100))
	recordEvents(t, s, 1,
		Event{Kind: KindImpression, BookID: 1},
		Event{Kind: KindClick, BookID: 1},
		Event{Kind: KindRead, BookID: 1, Value: 300},
		// 100 天前的收藏：不进窗口汇总，但计入累计收藏数。
		Event{Kind: KindShelf, BookID: 9, TS: old},
	)
	got, err := s.Summary(context.Background(), 7)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Days != 7 || got.Impressions != 1 || got.Clicks != 1 || got.Reads != 1 || got.ReadSeconds != 300 {
		t.Fatalf("窗口汇总 = %+v", got)
	}
	if got.ShelfAdds != 1 {
		t.Fatalf("收藏应为累计值 1，得到 %d", got.ShelfAdds)
	}
}

func TestSummaryClampsDays(t *testing.T) {
	s := openTemp(t)
	got, err := s.Summary(context.Background(), 999)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Days != maxStatsDays {
		t.Fatalf("Days = %d，期望夹到 %d", got.Days, maxStatsDays)
	}
}

func TestInterestTopOrdersByDecayedWeight(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"}, // +1
		Event{Kind: KindShelf, BookID: 2, CategoryL1: "都市"}, // +3
		Event{Kind: KindRating, BookID: 3, AuthorID: 12},    // +5 → author:12
	)
	got, err := s.InterestTop(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("InterestTop: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("行数 = %d，期望 3: %+v", len(got), got)
	}
	if got[0].Dim != dimAuthor || got[0].Key != "12" {
		t.Fatalf("榜首应是作者 12，得到 %+v", got[0])
	}
	if got[1].Key != "都市" || got[2].Key != "武侠" {
		t.Fatalf("分类应按权重降序: %+v", got)
	}
}

func TestInterestTopDecaysOldWeights(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindShelf, BookID: 1, CategoryL1: "旧兴趣"}, // 权重 3
		Event{Kind: KindClick, BookID: 2, CategoryL1: "新兴趣"}, // 权重 1
	)
	old := formatTime(nowUTC().AddDate(0, 0, -14))
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE user_interest SET updated_at = ? WHERE key = '旧兴趣'", old); err != nil {
		t.Fatalf("改旧时间: %v", err)
	}
	got, err := s.InterestTop(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("InterestTop: %v", err)
	}
	// 3·e⁻¹ ≈ 1.10 > 1：衰减后旧兴趣仍略高于新兴趣，但已几乎持平。
	if got[0].Key != "旧兴趣" || math.Abs(got[0].Weight-3/math.E) > 1e-2 {
		t.Fatalf("衰减后榜首 = %+v", got[0])
	}
}

func TestInterestTopClampsLimit(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 1, CategoryL1: "甲"},
		Event{Kind: KindClick, BookID: 2, CategoryL1: "乙"},
		Event{Kind: KindClick, BookID: 3, CategoryL1: "丙"},
	)
	if got, err := s.InterestTop(context.Background(), 1, 0); err != nil || len(got) != 1 {
		t.Fatalf("limit=0 应夹到 1，得到 %d err=%v", len(got), err)
	}
	if got, err := s.InterestTop(context.Background(), 1, maxInterestLimit+9); err != nil || len(got) != 3 {
		t.Fatalf("超上限应返回全部 3 行，得到 %d err=%v", len(got), err)
	}
	if _, err := s.InterestTop(context.Background(), 0, 10); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("user_id=0 应报 ErrInvalidArgument")
	}
}

func TestTopInterestRowsTieBreak(t *testing.T) {
	rows := []InterestRow{
		{Dim: dimCategoryL1, Key: "乙", Weight: 1},
		{Dim: dimAuthor, Key: "3", Weight: 1},
		{Dim: dimCategoryL1, Key: "甲", Weight: 1},
	}
	got := topInterestRows(rows, 3)
	if got[0].Dim != dimAuthor || got[0].Key != "3" {
		t.Fatalf("同权重应按 dim 升序（author < category），得到 %+v", got)
	}
	// 同 dim 按 key 字节序：「乙」(U+4E59) 排在「甲」(U+7532) 前面。
	if got[1].Key != "乙" || got[2].Key != "甲" {
		t.Fatalf("同 dim 应按 key 升序，得到 %+v", got)
	}
}

func TestStatsQueriesOnClosedDB(t *testing.T) {
	s := openTemp(t)
	closeStore(t, s)
	ctx := context.Background()
	if _, err := s.DailyStats(ctx, 7); err == nil {
		t.Fatal("DailyStats 在库关闭后应报错")
	}
	if _, err := s.Summary(ctx, 7); err == nil {
		t.Fatal("Summary 在库关闭后应报错")
	}
	if _, err := s.InterestTop(ctx, 1, 10); err == nil {
		t.Fatal("InterestTop 在库关闭后应报错")
	}
}
