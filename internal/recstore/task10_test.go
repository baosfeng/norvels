package recstore

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 本文件覆盖 task-10 的三块 recstore 能力：作者级拉黑、理由分布、分类点击分布。

func TestDislikeWithAuthorBlacklistsAuthor(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, AuthorID: 7})
	recordEvents(t, s, 1, Event{Kind: KindDislike, BookID: 1, AuthorID: 7})

	authors, err := s.NegativeAuthors(ctx, 1)
	if err != nil {
		t.Fatalf("NegativeAuthors: %v", err)
	}
	if !authors[7] || len(authors) != 1 {
		t.Fatalf("作者拉黑集合 = %v，期望只有 7", authors)
	}
	books, err := s.NegativeItems(ctx, 1)
	if err != nil || !books[1] {
		t.Fatalf("书级拉黑仍应生效: %v err=%v", books, err)
	}
}

func TestDislikeWithoutAuthorOnlyBlacklistsBook(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	recordEvents(t, s, 1, Event{Kind: KindDislike, BookID: 2})

	authors, err := s.NegativeAuthors(ctx, 1)
	if err != nil || len(authors) != 0 {
		t.Fatalf("未带作者不应拉黑作者: %v err=%v", authors, err)
	}
	books, _ := s.NegativeItems(ctx, 1)
	if !books[2] {
		t.Fatalf("书级拉黑应生效: %v", books)
	}
}

func TestNegativeAuthorsErrors(t *testing.T) {
	s := openTemp(t)
	if _, err := s.NegativeAuthors(context.Background(), 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("user_id=0 应报 ErrInvalidArgument")
	}
	closeStore(t, s)
	if _, err := s.NegativeAuthors(context.Background(), 1); err == nil {
		t.Fatal("库已关闭应报错")
	}
}

func TestReasonMix(t *testing.T) {
	s := openTemp(t)
	old := formatTime(nowUTC().AddDate(0, 0, -40))
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 1, Extra: "因为你常看武侠"},
		Event{Kind: KindClick, BookID: 2, Extra: "因为你常看武侠"},
		Event{Kind: KindClick, BookID: 3, Extra: "随机发现"},
		Event{Kind: KindClick, BookID: 4},                   // 空 extra 跳过
		Event{Kind: KindImpression, BookID: 5, Extra: "曝光"}, // 只统计 click
		Event{Kind: KindClick, BookID: 6, Extra: "旧理由", TS: old},
	)
	got, err := s.ReasonMix(context.Background(), 7)
	if err != nil {
		t.Fatalf("ReasonMix: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("理由分布 = %+v，期望 2 条（窗口外/空/非 click 都排除）", got)
	}
	if got[0].Reason != "因为你常看武侠" || got[0].Clicks != 2 {
		t.Fatalf("榜首 = %+v", got[0])
	}
	if got[1].Reason != "随机发现" || got[1].Clicks != 1 {
		t.Fatalf("第二名 = %+v", got[1])
	}
}

func TestCategoryClicks(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"},
		Event{Kind: KindClick, BookID: 2, CategoryL1: "武侠"},
		Event{Kind: KindClick, BookID: 3, CategoryL1: "都市"},
		Event{Kind: KindClick, BookID: 4}, // 空分类跳过
		Event{Kind: KindImpression, BookID: 5, CategoryL1: "曝光分类"},
	)
	got, err := s.CategoryClicks(context.Background(), 7)
	if err != nil {
		t.Fatalf("CategoryClicks: %v", err)
	}
	if len(got) != 2 || got[0].CategoryL1 != "武侠" || got[0].Clicks != 2 {
		t.Fatalf("分类点击 = %+v", got)
	}
	if got[1].CategoryL1 != "都市" || got[1].Clicks != 1 {
		t.Fatalf("第二名 = %+v", got[1])
	}
}

func TestStatsWindowStart(t *testing.T) {
	if got := statsWindowStart(0); got != dayOf(formatTime(nowUTC())) {
		t.Fatalf("days=0 应夹到 1 天（今天），得到 %q", got)
	}
	if got := statsWindowStart(3); got != dayAgo(2) {
		t.Fatalf("days=3 起点 = %q，期望 %q", got, dayAgo(2))
	}
}

func TestMixQueriesOnClosedDB(t *testing.T) {
	s := openTemp(t)
	closeStore(t, s)
	if _, err := s.ReasonMix(context.Background(), 7); err == nil {
		t.Fatal("ReasonMix 在库关闭后应报错")
	}
	if _, err := s.CategoryClicks(context.Background(), 7); err == nil {
		t.Fatal("CategoryClicks 在库关闭后应报错")
	}
}

func TestInterestsIncludesCurrentSlot(t *testing.T) {
	s := openTemp(t)
	recordEvents(t, s, 1, Event{Kind: KindClick, BookID: 1, CategoryL1: "武侠"})
	m, err := s.Interests(context.Background(), 1)
	if err != nil {
		t.Fatalf("Interests: %v", err)
	}
	if m["category_l1:武侠"] <= 0 {
		t.Fatalf("Interests 应返回非空兴趣: %v", m)
	}
	if _, err := s.Interests(context.Background(), 0); err == nil {
		t.Fatal("user_id=0 应报错")
	}
}

func TestSlotOfTimestamp(t *testing.T) {
	// 坏时间归到当前时段，不阻断写入。
	if got := slotOfTimestamp("垃圾时间"); got != slotOf(nowUTC()) {
		t.Fatalf("不可解析时间应归到当前时段，得到 %d", got)
	}
	// 14 点落在下午槽（2）。
	ts := formatTime(time.Date(2026, 11, 1, 14, 0, 0, 0, time.Local))
	if got := slotOfTimestamp(ts); got != 2 {
		t.Fatalf("14 点应归到槽 2，得到 %d", got)
	}
}
