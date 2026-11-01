package recstore

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func interestOf(t *testing.T, s *Store, userID int64, key string) float64 {
	t.Helper()
	m, err := s.Interests(context.Background(), userID)
	if err != nil {
		t.Fatalf("读兴趣: %v", err)
	}
	return m[key]
}

func TestRecordEventsRejectsInvalid(t *testing.T) {
	cases := []struct {
		name   string
		userID int64
		events []Event
	}{
		{"未登录", 0, []Event{{Kind: KindClick, BookID: 1}}},
		{"未知 kind", 1, []Event{{Kind: "bogus", BookID: 1}}},
		{"缺 book", 1, []Event{{Kind: KindClick}}},
		{"分类超长", 1, []Event{{Kind: KindClick, BookID: 1, CategoryL1: strings.Repeat("武", maxFieldLen+1)}}},
		{"extra 超长", 1, []Event{{Kind: KindClick, BookID: 1, Extra: strings.Repeat("x", maxExtraLen+1)}}},
		{"时间非法", 1, []Event{{Kind: KindClick, BookID: 1, TS: "昨天"}}},
		{"超批量", 1, make([]Event, MaxEventBatch+1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := openTemp(t)
			n, err := s.RecordEvents(context.Background(), c.userID, c.events)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，得到 n=%d err=%v", n, err)
			}
			if n != 0 {
				t.Fatalf("被拒绝的批次不应计入接受数，得到 %d", n)
			}
		})
	}
}

func TestRecordEventsEmptyBatch(t *testing.T) {
	s := openTemp(t)
	n, err := s.RecordEvents(context.Background(), 1, nil)
	if err != nil || n != 0 {
		t.Fatalf("空批次应为 (0, nil)，得到 (%d, %v)", n, err)
	}
}

func TestRecordEventsImpressionAggregatesOnly(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindImpression, BookID: 5}, Event{Kind: KindImpression, BookID: 5})

	if n := countRows(t, s, "events"); n != 0 {
		t.Fatalf("impression 不应落 events 明细，得到 %d 行", n)
	}
	imp, _, _, _, _, ok := bookStats(t, s, 5)
	if !ok || imp != 2 {
		t.Fatalf("book_stats.impressions = %d(ok=%v)，期望 2", imp, ok)
	}
	var daily int
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT SUM(impressions) FROM book_daily WHERE book_id = 5").Scan(&daily); err != nil {
		t.Fatalf("读 book_daily: %v", err)
	}
	if daily != 2 {
		t.Fatalf("book_daily.impressions = %d，期望 2", daily)
	}
	recent, err := s.RecentImpressions(context.Background(), 1, time.Time{})
	if err != nil || !recent[5] {
		t.Fatalf("recent_items 应记下书 5，得到 %v err=%v", recent, err)
	}
}

func TestRecordEventsAggregatesByKind(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 5, AuthorID: 12, CategoryL1: "武侠", CategoryL2: "传统"},
		Event{Kind: KindRead, BookID: 5, AuthorID: 12, CategoryL1: "武侠", Value: 600},
		Event{Kind: KindShelf, BookID: 5, CategoryL1: "武侠"},
	)
	imp, clicks, reads, seconds, shelf, ok := bookStats(t, s, 5)
	if !ok {
		t.Fatal("book_stats 应有书 5 的行")
	}
	if imp != 0 || clicks != 1 || reads != 1 || seconds != 600 || shelf != 1 {
		t.Fatalf("聚合 = imp%d click%d read%d sec%d shelf%d", imp, clicks, reads, seconds, shelf)
	}
	if got := interestOf(t, s, 1, "category_l1:武侠"); math.Abs(got-7) > 1e-2 {
		t.Fatalf("武侠兴趣 = %v，期望 7（click1 + read3 + shelf3）", got)
	}
	if got := interestOf(t, s, 1, "author:12"); math.Abs(got-4) > 1e-2 {
		t.Fatalf("作者兴趣 = %v，期望 4（click1 + read3）", got)
	}
	if n := countRows(t, s, "events"); n != 3 {
		t.Fatalf("events 行数 = %d，期望 3", n)
	}
}

func TestRecordEventsInterestDeltaByKind(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindRating, BookID: 1, CategoryL1: "评分"},
		Event{Kind: KindView, BookID: 2, CategoryL1: "浏览"},
		Event{Kind: KindSearch, CategoryL1: "搜索"},
		Event{Kind: KindRead, BookID: 3, CategoryL1: "超长阅读", Value: maxReadSeconds},
	)
	if got := interestOf(t, s, 1, "category_l1:评分"); math.Abs(got-5) > 1e-2 {
		t.Fatalf("rating 增量 = %v，期望 5", got)
	}
	if got := interestOf(t, s, 1, "category_l1:浏览"); math.Abs(got-0.5) > 1e-2 {
		t.Fatalf("view 增量 = %v，期望 0.5", got)
	}
	m, err := s.Interests(context.Background(), 1)
	if err != nil {
		t.Fatalf("Interests: %v", err)
	}
	if _, ok := m["category_l1:搜索"]; ok {
		t.Fatal("search 不应产生兴趣权重")
	}
	if got := m["category_l1:超长阅读"]; math.Abs(got-4) > 1e-2 {
		t.Fatalf("read 增量封顶后 = %v，期望 1+3", got)
	}
}

func TestRecordEventsSplitsCategoryLevels(t *testing.T) {
	s := openTemp(t)
	// 同一本书同时带一级/二级分类：两个维度各记一条，不再合并成同一个 key。
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠", CategoryL2: "江湖"})
	m, err := s.Interests(context.Background(), 1)
	if err != nil {
		t.Fatalf("Interests: %v", err)
	}
	if _, ok := m["category:武侠"]; ok {
		t.Fatalf("旧键 category 应作废，实际仍写入: %v", m)
	}
	if got := m["category_l1:武侠"]; math.Abs(got-1) > 1e-6 {
		t.Fatalf("一级分类权重 = %v，期望 1（只计 1 次点击）", got)
	}
	if got := m["category_l2:江湖"]; math.Abs(got-1) > 1e-6 {
		t.Fatalf("二级分类权重 = %v，期望 1（独立计数，不与 L1 合并）", got)
	}
}

func TestRecordEventsReadDecaysOldInterest(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"})
	old := formatTime(time.Now().UTC().AddDate(0, 0, -14))
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE user_interest SET weight = 2, updated_at = ? WHERE user_id = 1", old); err != nil {
		t.Fatalf("改写旧权重: %v", err)
	}
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"})
	// 2·e⁻¹ ≈ 0.736 + 1
	if got := interestOf(t, s, 1, "category_l1:武侠"); math.Abs(got-1.736) > 1e-2 {
		t.Fatalf("衰减后再累加 = %v，期望 ≈1.736", got)
	}
}

func TestRecordEventsUnparsableUpdatedAtKeepsWeight(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"})
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE user_interest SET updated_at = '垃圾时间' WHERE user_id = 1"); err != nil {
		t.Fatalf("改坏时间: %v", err)
	}
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"})
	// 时间不可解析时不衰减，也不把权重清零：宁可多给一点兴趣，也别把用户偏好丢掉。
	if got := interestOf(t, s, 1, "category_l1:武侠"); math.Abs(got-2) > 1e-2 {
		t.Fatalf("不可解析时间时权重 = %v，期望 2", got)
	}
}

func TestRecordEventsCooccur(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindRead, BookID: 1})
	recordEvents(t, s, 1, Event{Kind: KindRead, BookID: 2})
	pairCount := func() int { return countRows(t, s, "cooccur") }
	if n := pairCount(); n != 2 {
		t.Fatalf("首对共读应有双向 2 行，得到 %d", n)
	}
	recordEvents(t, s, 1, Event{Kind: KindRead, BookID: 1}) // 重读：与书 2 的共读再累计一次
	var users int
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT MAX(users) FROM cooccur WHERE book_id = 1 AND other_id = 2").Scan(&users); err != nil {
		t.Fatalf("读共读强度: %v", err)
	}
	if users != 2 {
		t.Fatalf("重读后 users = %d，期望 2", users)
	}
}

func TestRecordEventsClampsNegativeValue(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindRead, BookID: 9, Value: -5})
	if _, _, _, seconds, _, _ := bookStats(t, s, 9); seconds != 0 {
		t.Fatalf("负停留秒数应夹成 0，得到 %d", seconds)
	}
}

func TestRecordEventsViewOnlyTouchesLastAt(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindView, BookID: 4})
	if n := countRows(t, s, "book_daily"); n != 0 {
		t.Fatalf("view 不应产生按日聚合，得到 %d 行", n)
	}
	if n := countRows(t, s, "events"); n != 1 {
		t.Fatalf("view 应落事件明细，得到 %d 行", n)
	}
}

func TestRecordEventsClosedDB(t *testing.T) {
	s := openTemp(t)
	closeStore(t, s)
	if _, err := s.RecordEvents(context.Background(), 1,
		[]Event{{Kind: KindClick, BookID: 1}}); err == nil {
		t.Fatal("库已关闭时写入应报错")
	}
}

func TestRecordReadingClampsSeconds(t *testing.T) {
	s := openTemp(t)
	if err := s.RecordReading(context.Background(), 1, 6, 12, "武侠", "传统", 1<<20, ""); err != nil {
		t.Fatalf("RecordReading: %v", err)
	}
	if _, _, reads, seconds, _, _ := bookStats(t, s, 6); reads != 1 || seconds != maxReadSeconds {
		t.Fatalf("读书记录 = reads%d seconds%d，期望 1/%d", reads, seconds, maxReadSeconds)
	}
	if err := s.RecordReading(context.Background(), 1, 6, 12, "武侠", "", -3, ""); err != nil {
		t.Fatalf("负秒数不应报错: %v", err)
	}
	if err := s.RecordReading(context.Background(), 1, 0, 0, "", "", 10, ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("缺 book 应报 ErrInvalidArgument，得到 %v", err)
	}
}
