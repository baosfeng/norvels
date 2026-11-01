package recstore

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestInterestsErrors(t *testing.T) {
	s := openTemp(t)
	if _, err := s.InterestsAt(context.Background(), 0, otherSlot()); err == nil {
		t.Fatal("user_id=0 应报错")
	}
	closeStore(t, s)
	if _, err := s.InterestsAt(context.Background(), 1, otherSlot()); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestInterestsDecaysOnRead(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"})
	old := formatTime(time.Now().UTC().Add(-7 * 24 * time.Hour))
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE user_interest SET weight = 1, updated_at = ? WHERE user_id = 1", old); err != nil {
		t.Fatalf("改写权重: %v", err)
	}
	got := interestOf(t, s, 1, "category_l1:武侠")
	if math.Abs(got-math.Exp(-0.5)) > 1e-2 {
		t.Fatalf("7 天后权重 = %v，期望 ≈ e⁻⁰·⁵", got)
	}
	// 衰减到阈值以下的分量不返回：否则调用方要为一行早已失效的偏好做无谓计算。
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE user_interest SET weight = 1e-9, updated_at = ? WHERE user_id = 1", old); err != nil {
		t.Fatalf("改小权重: %v", err)
	}
	if m := globalInterest(t, s, 1); len(m) != 0 {
		t.Fatalf("过小权重应被过滤，得到 %v", m)
	}
	// 时间不可解析时保持原值（不衰减），只影响排序强度，不影响正确性。
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE user_interest SET weight = 3, updated_at = '怪' WHERE user_id = 1"); err != nil {
		t.Fatalf("改坏时间: %v", err)
	}
	if got := interestOf(t, s, 1, "category_l1:武侠"); math.Abs(got-3) > 1e-9 {
		t.Fatalf("时间不可解析时权重 = %v，期望 3", got)
	}
}

func TestCooccurExcludesReadBooks(t *testing.T) {
	s := openTemp(t)
	// 用户 1 读 1/2/3，用户 2 读 1 和 9：用户 1 的邻居里只有 9 是他没读过的。
	recordEvents(t, s, 1, Event{Kind: KindRead, BookID: 1}, Event{Kind: KindRead, BookID: 2},
		Event{Kind: KindRead, BookID: 3})
	recordEvents(t, s, 2, Event{Kind: KindRead, BookID: 1}, Event{Kind: KindRead, BookID: 9})
	pairs, err := s.Cooccur(context.Background(), 1, 50)
	if err != nil {
		t.Fatalf("Cooccur: %v", err)
	}
	if len(pairs) != 1 || pairs[0].BookID != 1 || pairs[0].OtherID != 9 {
		t.Fatalf("共读邻居 = %+v，期望只剩（书1 → 书9）", pairs)
	}
	if _, err := s.Cooccur(context.Background(), 0, 10); err == nil {
		t.Fatal("user_id=0 应报错")
	}
	closeStore(t, s)
	if _, err := s.Cooccur(context.Background(), 1, 10); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func seedHotBooks(t *testing.T) *Store {
	t.Helper()
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindRead, BookID: 10})
	recordEvents(t, s, 2, Event{Kind: KindRead, BookID: 10})
	recordEvents(t, s, 1,
		Event{Kind: KindRead, BookID: 11, Value: 60},
		Event{Kind: KindClick, BookID: 11}, Event{Kind: KindClick, BookID: 11},
		Event{Kind: KindImpression, BookID: 99},
	)
	return s
}

func TestHotBooksRanking(t *testing.T) {
	s := seedHotBooks(t)
	hot, err := s.HotBooks(context.Background(), 7, 10)
	if err != nil {
		t.Fatalf("HotBooks: %v", err)
	}
	if len(hot) != 2 {
		t.Fatalf("热度排行 = %+v，期望 2 本（纯曝光不算热度）", hot)
	}
	if hot[0].BookID != 10 || hot[0].Reads != 2 {
		t.Fatalf("热度榜首 = %+v，期望书 10 reads=2", hot[0])
	}
	if hot[1].BookID != 11 || hot[1].Clicks != 2 || hot[1].Seconds != 60 {
		t.Fatalf("热度第二名 = %+v", hot[1])
	}
}

func TestHotBooksWindowAndLimit(t *testing.T) {
	s := seedHotBooks(t)
	// 窗口与条数的边界：0 → 默认 7 天；超大 → 夹到 365 天；limit=1 只留榜首。
	for _, days := range []int{0, maxHotDays + 999} {
		got, err := s.HotBooks(context.Background(), days, 1)
		if err != nil || len(got) != 1 || got[0].BookID != 10 {
			t.Fatalf("days=%d 结果 = %+v err=%v", days, got, err)
		}
	}
	closeStore(t, s)
	if _, err := s.HotBooks(context.Background(), 7, 10); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestRecentImpressions(t *testing.T) {
	s := openTemp(t)
	old := formatTime(time.Now().UTC().AddDate(0, 0, -30))
	recordEvents(t, s, 1,
		Event{Kind: KindImpression, BookID: 1},
		Event{Kind: KindImpression, BookID: 2, TS: old},
	)
	all, err := s.RecentImpressions(context.Background(), 1, time.Time{})
	if err != nil || len(all) != 2 {
		t.Fatalf("不限时间的曝光集合 = %v err=%v", all, err)
	}
	fresh, err := s.RecentImpressions(context.Background(), 1, time.Now().UTC().AddDate(0, 0, -7))
	if err != nil || len(fresh) != 1 || !fresh[1] {
		t.Fatalf("近 7 天曝光集合 = %v err=%v", fresh, err)
	}
	if _, err := s.RecentImpressions(context.Background(), 0, time.Time{}); err == nil {
		t.Fatal("user_id=0 应报错")
	}
	closeStore(t, s)
	if _, err := s.RecentImpressions(context.Background(), 1, time.Time{}); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestReadBooks(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindRead, BookID: 1},
		Event{Kind: KindClick, BookID: 2},
		Event{Kind: KindSearch, CategoryL1: "武侠"},
	)
	read, err := s.ReadBooks(context.Background(), 1)
	if err != nil {
		t.Fatalf("ReadBooks: %v", err)
	}
	if !read[1] || read[2] {
		t.Fatalf("已读集合 = %v，期望只有书 1", read)
	}
	if _, err := s.ReadBooks(context.Background(), 0); err == nil {
		t.Fatal("user_id=0 应报错")
	}
	closeStore(t, s)
	if _, err := s.ReadBooks(context.Background(), 1); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestStats(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindRead, BookID: 1, CategoryL1: "武侠"},
		Event{Kind: KindImpression, BookID: 2},
	)
	recordEvents(t, s, 2, Event{Kind: KindClick, BookID: 3})
	st, err := s.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Events != 2 || st.Books != 3 || st.Users != 2 || st.Interests != 1 ||
		st.Cooccur != 0 || st.RecentItems != 1 {
		t.Fatalf("Stats = %+v", st)
	}
	closeStore(t, s)
	if _, err := s.Stats(context.Background()); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestInterestsAtBoostsCurrentSlot(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	// 事件写在一个确定时刻，随后分别在「同一时段」与「另一时段」读兴趣。
	at := time.Now()
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠", TS: formatTime(at)})
	weightAt := func(now time.Time) float64 {
		m, err := s.InterestsAt(ctx, 1, now)
		if err != nil {
			t.Fatalf("InterestsAt: %v", err)
		}
		return m["category_l1:武侠"]
	}
	sameSlot := weightAt(at)
	otherSlotWeight := weightAt(at.Add(6 * time.Hour))
	if math.Abs(sameSlot-2.5) > 1e-2 {
		t.Fatalf("同时段兴趣 = %v，期望 1.0 + 1.5x1.0", sameSlot)
	}
	// 隔了 6 小时再读，全局权重会自然衰减约 1.8%（14 天半衰），放宽容差。
	if math.Abs(otherSlotWeight-1.0) > 3e-2 {
		t.Fatalf("其它时段兴趣 = %v，期望只有全局约 1.0", otherSlotWeight)
	}
}
