package recstore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPruneRemovesOldEvents(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC()
	old := formatTime(now.AddDate(0, 0, -120))
	recent := formatTime(now.AddDate(0, 0, -1))
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 1, TS: old},
		Event{Kind: KindRead, BookID: 1, TS: old},
		Event{Kind: KindImpression, BookID: 1, TS: old},
		Event{Kind: KindClick, BookID: 2, TS: recent},
		Event{Kind: KindImpression, BookID: 2, TS: recent},
	)
	if n := countRows(t, s, "events"); n != 3 {
		t.Fatalf("清理前 events = %d，期望 3（impression 不落明细）", n)
	}

	deleted, err := s.Prune(context.Background(), now.AddDate(0, 0, -90))
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("删除行数 = %d，期望 2", deleted)
	}
	if n := countRows(t, s, "events"); n != 1 {
		t.Fatalf("清理后 events = %d，期望只剩 1 条新事件", n)
	}
	recent2, err := s.RecentImpressions(context.Background(), 1, time.Time{})
	if err != nil {
		t.Fatalf("RecentImpressions: %v", err)
	}
	if recent2[1] || !recent2[2] {
		t.Fatalf("曝光记录应按窗口清理: %v", recent2)
	}

	// 幂等：再跑一次没有可删的行，也不该报错。
	again, err := s.Prune(context.Background(), now.AddDate(0, 0, -90))
	if err != nil || again != 0 {
		t.Fatalf("重复清理 = (%d, %v)，期望 (0, nil)", again, err)
	}
}

func TestPruneKeepsAggregates(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC()
	old := formatTime(now.AddDate(0, 0, -120))
	recordEvents(t, s, 1, Event{
		Kind: KindRead, BookID: 7, AuthorID: 12, CategoryL1: "武侠", Value: 600, TS: old,
	})
	if _, err := s.Prune(context.Background(), now.AddDate(0, 0, -90)); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	// 聚合是固定行数的汇总，不随明细过期：热度、兴趣、共读仍然有效。
	if _, _, reads, seconds, _, ok := bookStats(t, s, 7); !ok || reads != 1 || seconds != 600 {
		t.Fatalf("book_stats 不应被清理: ok=%v reads=%d seconds=%d", ok, reads, seconds)
	}
	if got := interestOf(t, s, 1, "category:武侠"); got <= 0 {
		t.Fatalf("兴趣权重不应被清理，得到 %v", got)
	}
}

func TestPruneRejectsZeroTime(t *testing.T) {
	s := openTemp(t)
	if _, err := s.Prune(context.Background(), time.Time{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("零值时间应报 ErrInvalidArgument，得到 %v", err)
	}
}

func TestPruneExpiredUsesRetentionWindow(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC()
	recordEvents(t, s, 1,
		Event{Kind: KindClick, BookID: 3, TS: formatTime(now.AddDate(0, 0, -(retentionDays + 1)))},
		Event{Kind: KindClick, BookID: 4, TS: formatTime(now.AddDate(0, 0, -(retentionDays - 1)))},
	)
	deleted, err := s.PruneExpired(context.Background(), now)
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("删除行数 = %d，期望 1（只删过窗口的那条）", deleted)
	}
	read, err := s.ReadBooks(context.Background(), 1)
	if err != nil || len(read) != 0 {
		t.Fatalf("click 不写已读集合: %v err=%v", read, err)
	}
	if n := countRows(t, s, "events"); n != 1 {
		t.Fatalf("窗口内的事件应保留，得到 %d", n)
	}
}

func TestPruneOnClosedDB(t *testing.T) {
	s := openTemp(t)
	closeStore(t, s)
	if _, err := s.Prune(context.Background(), time.Now().UTC()); err == nil {
		t.Fatal("库已关闭时清理应报错")
	}
}
