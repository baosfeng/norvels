package recstore

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestDislikeWritesNegativeItemAndLowersInterest(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	// 先点一次建立兴趣（分类 1 / 标签 1 / 系列 1）。
	recordEvents(t, s, 1, Event{
		Kind: KindClick, BookID: 1, CategoryL1: "武侠", Tags: []string{"群像"}, Series: "山河",
	})
	// 再 dislike：兴趣各减 2 → 全部夹到 0，并记住这本书。
	recordEvents(t, s, 1, Event{
		Kind: KindDislike, BookID: 1, CategoryL1: "武侠", Tags: []string{"群像"}, Series: "山河",
	})

	neg, err := s.NegativeItems(ctx, 1)
	if err != nil {
		t.Fatalf("NegativeItems: %v", err)
	}
	if !neg[1] || len(neg) != 1 {
		t.Fatalf("负反馈集合 = %v，期望只有书 1", neg)
	}
	if n := countRows(t, s, "events"); n != 2 {
		t.Fatalf("dislike 应落事件明细，events = %d", n)
	}
	interest, err := s.Interests(ctx, 1)
	if err != nil {
		t.Fatalf("Interests: %v", err)
	}
	for key, weight := range interest {
		if weight < 0 {
			t.Fatalf("兴趣不应出现负权重: %s = %v", key, weight)
		}
	}
	// 1 - 2 被夹到 0，因此这三维都不再出现在兴趣表里。
	for _, key := range []string{"category_l1:武侠", "tag:群像", "series:山河"} {
		if _, ok := interest[key]; ok {
			t.Fatalf("%s 应被压到 0 并过滤，实际仍有: %v", key, interest[key])
		}
	}
}

func TestDislikeRepeatIsIdempotent(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindDislike, BookID: 2},
		Event{Kind: KindDislike, BookID: 2},
	)
	if n := countRows(t, s, "negative_items"); n != 1 {
		t.Fatalf("重复 dislike 应只留一行，得到 %d", n)
	}
}

func TestRecordEventsTagsAndSeriesInterest(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{
		Kind: KindClick, BookID: 3, Tags: []string{"武侠", "群像"}, Series: "山河",
	})
	interest, err := s.Interests(context.Background(), 1)
	if err != nil {
		t.Fatalf("Interests: %v", err)
	}
	for _, key := range []string{"tag:武侠", "tag:群像", "series:山河"} {
		if got := interest[key]; math.Abs(got-1) > 1e-6 {
			t.Fatalf("%s = %v，期望 1", key, got)
		}
	}
}

func TestRecordEventsRejectsBadTagsAndSeries(t *testing.T) {
	s := openTemp(t)
	long := make([]byte, maxFieldLen+1)
	for i := range long {
		long[i] = 'x'
	}
	cases := map[string]Event{
		"标签过多": {Kind: KindClick, BookID: 1, Tags: make([]string, maxTagsPerEvent+1)},
		"标签过长": {Kind: KindClick, BookID: 1, Tags: []string{string(long)}},
		"系列过长": {Kind: KindClick, BookID: 1, Series: string(long)},
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.RecordEvents(context.Background(), 1, []Event{ev}); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("应报 ErrInvalidArgument，得到 %v", err)
			}
		})
	}
}

func TestNegativeItemsErrors(t *testing.T) {
	s := openTemp(t)
	if _, err := s.NegativeItems(context.Background(), 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("user_id=0 应报 ErrInvalidArgument")
	}
	closeStore(t, s)
	if _, err := s.NegativeItems(context.Background(), 1); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestInterestTopIncludesTagAndSeries(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindShelf, BookID: 1, Tags: []string{"群像"}, Series: "山河"})
	rows, err := s.InterestTop(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("InterestTop: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Dim+":"+r.Key] = true
	}
	if !seen["tag:群像"] || !seen["series:山河"] {
		t.Fatalf("兴趣榜应含标签与系列维度: %+v", rows)
	}
	if math.Abs(rows[0].Weight-3) > 1e-6 {
		t.Fatalf("shelf 权重应为 3，得到 %+v", rows[0])
	}
}
