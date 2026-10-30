package store

import (
	"context"
	"errors"
	"testing"
)

// 个人数据按账号隔离的口径（0016）：漏一处就是串号，因此这里逐张表核对——
// 进度 / 会话 / 书签 / 书单，读与写两个方向都要分清「我的」与「别人的」。

// TestProgressIsolatedPerUser 两个账号的最近在读互不可见。
func TestProgressIsolatedPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookA := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	bookB := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	other := mustRegister(t, st, "读者乙")
	saveProgressFor(t, ctx, st, defaultUserID, bookA)
	saveProgressFor(t, ctx, st, other.ID, bookB)

	mine, err := st.ListReadingProgress(ctx, 5)
	requireOK(t, "甲的在读", err)
	requireEqual(t, "甲的在读数量", len(mine), 1)
	requireEqual(t, "甲的在读是甲的书", mine[0].BookID, bookA)

	theirs, err := st.ListReadingProgress(WithUserID(ctx, other.ID), 5)
	requireOK(t, "乙的在读", err)
	requireEqual(t, "乙的在读数量", len(theirs), 1)
	requireEqual(t, "乙的在读是乙的书", theirs[0].BookID, bookB)
}

// TestSessionsIsolatedPerUser 阅读会话 / 历史 / 统计都只统计自己的。
func TestSessionsIsolatedPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookA := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	bookB := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	other := mustRegister(t, st, "读者乙")
	addSessionFor(t, ctx, st, defaultUserID, bookA, 120)
	addSessionFor(t, ctx, st, other.ID, bookB, 300)

	items, total, err := st.ReadingHistory(ctx, 1, 10)
	requireOK(t, "甲的历史", err)
	requireEqual(t, "甲的历史总数", total, 1)
	requireEqual(t, "甲的历史是甲的书", items[0].BookID, bookA)
	requireEqual(t, "甲的累计秒数", items[0].TotalSeconds, 120)

	stats, err := st.ReadingStats(ctx, 7, 0)
	requireOK(t, "甲的统计", err)
	requireEqual(t, "甲的累计秒数（统计）", stats.TotalSeconds, 120)

	theirs, _, err := st.ReadingHistory(WithUserID(ctx, other.ID), 1, 10)
	requireOK(t, "乙的历史", err)
	requireEqual(t, "乙的历史是乙的书", theirs[0].BookID, bookB)
	otherStats, err := st.ReadingStats(WithUserID(ctx, other.ID), 7, 0)
	requireOK(t, "乙的统计", err)
	requireEqual(t, "乙的累计秒数", otherStats.TotalSeconds, 300)
}

// TestBookmarksIsolatedPerUser 书签按账号隔离：同段落可各存一条，互不可见、不可删。
func TestBookmarksIsolatedPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	chapterID := chapterIDsOf(t, ctx, st, bookID)[0]
	other := mustRegister(t, st, "读者乙")

	mine, _, err := st.AddBookmark(ctx, bookID, chapterID, 3, "甲的书签")
	requireOK(t, "甲加书签", err)
	// 同一位置、另一个账号：不撞唯一索引，也不覆盖甲的 label。
	theirs, created, err := st.AddBookmark(WithUserID(ctx, other.ID), bookID, chapterID, 3, "乙的书签")
	requireOK(t, "乙加书签", err)
	if !created {
		t.Error("乙在同段落加书签应新建，而不是命中甲的那条")
	}
	requireEqual(t, "乙的 label", theirs.Label, "乙的书签")

	mineList, err := st.ListBookmarks(ctx, bookID)
	requireOK(t, "甲的书签列表", err)
	requireEqual(t, "甲的书签数量", len(mineList), 1)
	requireEqual(t, "甲的 label 没被改", mineList[0].Label, "甲的书签")
	n, err := st.CountBookmarks(WithUserID(ctx, other.ID), bookID)
	requireOK(t, "乙的书签数", err)
	requireEqual(t, "乙的书签数", n, 1)

	// 乙删不掉甲的书签（按不存在处理，不泄露「这个 id 存在」）。
	if err := st.DeleteBookmark(WithUserID(ctx, other.ID), mine.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("乙删甲的书签 = %v，期望 ErrNotFound", err)
	}
	if n, err := st.CountBookmarks(ctx, bookID); err != nil || n != 1 {
		t.Errorf("甲的书签仍在: n=%d err=%v", n, err)
	}
}

// TestCollectionsIsolatedPerUser 书单按账号隔离：同名可各有一个，互相改不到。
func TestCollectionsIsolatedPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	other := mustRegister(t, st, "读者乙")

	mine, err := st.CreateCollection(ctx, "想读")
	requireOK(t, "甲建书单", err)
	// 同名书单在另一个账号下是合法的：唯一约束是 (user_id, name)。
	theirs, err := st.CreateCollection(WithUserID(ctx, other.ID), "想读")
	requireOK(t, "乙建同名书单", err)
	if mine.ID == theirs.ID {
		t.Fatal("两个账号的同名书单不该是同一行")
	}
	if _, err := st.CreateCollection(ctx, "想读"); !errors.Is(err, ErrConflict) {
		t.Errorf("同一账号重名 = %v，期望 ErrConflict", err)
	}

	requireEqual(t, "甲的书单数", lenList(t, ctx, st, 0), 1)
	requireEqual(t, "乙的书单数", lenList(t, ctx, st, other.ID), 1)

	// 乙改不到甲的书单。
	if err := st.RenameCollection(WithUserID(ctx, other.ID), mine.ID, "乱改"); !errors.Is(err, ErrNotFound) {
		t.Errorf("乙改甲的书单 = %v，期望 ErrNotFound", err)
	}
	if err := st.DeleteCollection(WithUserID(ctx, other.ID), mine.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("乙删甲的书单 = %v，期望 ErrNotFound", err)
	}
	requireEqual(t, "甲的书单还在", lenList(t, ctx, st, 0), 1)
}

// TestCollectionFilterIsolatedPerUser 按书单筛选书库时只认自己的书单。
func TestCollectionFilterIsolatedPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	other := mustRegister(t, st, "读者乙")
	coll, err := st.CreateCollection(ctx, "甲的书单")
	requireOK(t, "CreateCollection", err)
	requireOK(t, "加书", st.ReplaceCollectionBooks(ctx, coll.ID, []int64{bookID}))

	// 乙拿甲的 collection id 去筛：不该看到任何书。
	page, err := st.ListBooks(WithUserID(ctx, other.ID), BookQuery{Collection: coll.ID, Page: 1, Size: 10})
	requireOK(t, "乙按甲的书单筛选", err)
	requireEqual(t, "乙看到的书数", page.Total, 0)

	mine, err := st.ListBooks(ctx, BookQuery{Collection: coll.ID, Page: 1, Size: 10})
	requireOK(t, "甲按自己的书单筛选", err)
	requireEqual(t, "甲看到的书数", mine.Total, 1)
}

// ---------- 辅助 ----------

// mustRegister 建一个账号（失败即 Fatal）。
func mustRegister(t *testing.T, st *Store, name string) User {
	t.Helper()
	u, err := st.RegisterUser(context.Background(), name)
	requireOK(t, "RegisterUser("+name+")", err)
	return u
}

// saveProgressFor 以指定账号写一条进度。
func saveProgressFor(t *testing.T, ctx context.Context, st *Store, userID, bookID int64) {
	t.Helper()
	chapterID := chapterIDsOf(t, ctx, st, bookID)[0]
	requireOK(t, "SaveReadingProgress", st.SaveReadingProgress(WithUserID(ctx, userID), ReadingProgress{
		BookID: bookID, ChapterID: chapterID, Percent: 0.3,
	}))
}

// addSessionFor 以指定账号写一条阅读会话。
func addSessionFor(t *testing.T, ctx context.Context, st *Store, userID, bookID int64, seconds int) {
	t.Helper()
	chapterID := chapterIDsOf(t, ctx, st, bookID)[0]
	requireOK(t, "AddReadingSession", st.AddReadingSession(WithUserID(ctx, userID), ReadingSessionInput{
		BookID: bookID, ChapterID: chapterID, Seconds: seconds,
	}))
}

// lenList 数书单数量；userID 为 0 表示默认账号。
func lenList(t *testing.T, ctx context.Context, st *Store, userID int64) int {
	t.Helper()
	c := ctx
	if userID != 0 {
		c = WithUserID(ctx, userID)
	}
	items, err := st.ListCollections(c)
	requireOK(t, "ListCollections", err)
	return len(items)
}
