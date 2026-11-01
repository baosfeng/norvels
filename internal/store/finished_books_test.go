package store

import (
	"context"
	"testing"
)

// saveProgressWith 直接写一条进度（不经过章节校验会失败，所以借 SeedBook 的章节）。
func seedProgress(t *testing.T, ctx context.Context, st *Store, bookID int64, percent float64) {
	t.Helper()
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: bookID, From: 1, Limit: 1})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("取章节失败: %+v err=%v", page, err)
	}
	if err := st.SaveReadingProgress(ctx, ReadingProgress{
		BookID: bookID, ChapterID: page.Items[0].ID, Percent: percent,
	}); err != nil {
		t.Fatalf("SaveReadingProgress: %v", err)
	}
}

func TestFinishedBookIDs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	reading := seedCategoryBook(t, ctx, st, "读了一半", "作者甲", "", "", "fb1")
	almost := seedCategoryBook(t, ctx, st, "快读完了", "作者乙", "", "", "fb2")
	done := seedCategoryBook(t, ctx, st, "读完了", "作者丙", "", "", "fb3")
	untouched := seedCategoryBook(t, ctx, st, "没碰过", "作者丁", "", "", "fb4")

	seedProgress(t, ctx, st, reading, 0.4)
	seedProgress(t, ctx, st, almost, 0.99) // 边界值算读完
	seedProgress(t, ctx, st, done, 1)

	got, err := st.FinishedBookIDs(ctx)
	if err != nil {
		t.Fatalf("FinishedBookIDs: %v", err)
	}
	if len(got) != 2 || !got[almost] || !got[done] {
		t.Fatalf("读完集合 = %v，期望只含 %d/%d", got, almost, done)
	}
	if got[reading] || got[untouched] {
		t.Fatalf("未读完的书不应计入: %v", got)
	}
}

func TestFinishedBookIDsEmptyLibrary(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	got, err := st.FinishedBookIDs(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("空库应为空 map，得到 %v err=%v", got, err)
	}
}

func TestFinishedBookIDsOnClosedDB(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := st.FinishedBookIDs(ctx); err == nil {
		t.Fatal("库已关闭时应报错")
	}
}
