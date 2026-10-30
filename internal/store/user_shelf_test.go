package store

import (
	"context"
	"errors"
	"testing"
)

// 我的书架（0016）的口径：
//   - 加入 / 移出幂等；
//   - 按账号隔离：两个账号的书架互不可见；
//   - 书不存在是 ErrNotFound（静默成功会让前端以为加上了）；
//   - 删书时书架行随级联清单一起清掉。

func TestShelfAddRemoveIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)

	for i := 0; i < 2; i++ { // 第二次加入也必须成功（幂等）
		requireOK(t, "AddToShelf", st.AddToShelf(ctx, bookID))
	}
	ids, err := st.ShelfBookIDs(ctx)
	requireOK(t, "ShelfBookIDs", err)
	requireEqual(t, "加入两次后书架数量", len(ids), 1)

	for i := 0; i < 2; i++ { // 移出同样幂等
		requireOK(t, "RemoveFromShelf", st.RemoveFromShelf(ctx, bookID))
	}
	ids, err = st.ShelfBookIDs(ctx)
	requireOK(t, "ShelfBookIDs", err)
	requireEqual(t, "移出后书架数量", len(ids), 0)
}

// TestShelfIsolatedPerUser 两个账号的书架互不可见。
func TestShelfIsolatedPerUser(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	other, err := st.RegisterUser(ctx, "读者乙")
	requireOK(t, "RegisterUser", err)

	requireOK(t, "甲加入书架", st.AddToShelf(ctx, bookID))
	ids, err := st.ShelfBookIDs(WithUserID(ctx, other.ID))
	requireOK(t, "乙的书架", err)
	requireEqual(t, "乙看不到甲的书", len(ids), 0)

	// 乙加入同一本：两行（联合主键 user_id, book_id），不互相覆盖。
	requireOK(t, "乙加入书架", st.AddToShelf(WithUserID(ctx, other.ID), bookID))
	requireEqual(t, "书架总行数", countRows(t, ctx, st, "user_shelf"), 2)
	mine, total, err := st.ListShelf(ctx, 1, 10)
	requireOK(t, "甲的书架列表", err)
	requireEqual(t, "甲的列表长度", len(mine), 1)
	requireEqual(t, "甲的书架总数", total, 1)
}

// TestShelfValidation 非法 book id 400、书不存在 404。
func TestShelfValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, id := range []int64{0, -1} {
		if err := st.AddToShelf(ctx, id); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("AddToShelf(%d) = %v，期望 ErrInvalidArgument", id, err)
		}
	}
	if err := st.AddToShelf(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddToShelf(不存在的书) = %v，期望 ErrNotFound", err)
	}
	if err := st.RemoveFromShelf(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("RemoveFromShelf(0) = %v，期望 ErrInvalidArgument", err)
	}
}

// TestShelfListOrderAndPaging 按加入时间倒序分页，字段与系列列表同口径。
func TestShelfListOrderAndPaging(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	second := seedSeriesBook(t, ctx, st, 2, "示例书乙", "", 0, 30)
	for _, id := range []int64{first, second} {
		requireOK(t, "AddToShelf", st.AddToShelf(ctx, id))
	}
	// 让两行的 added_at 不同：同一毫秒内加入两本时顺序由 book_id 兜底。
	setAddedAt(t, ctx, st, first, "2026-10-01T00:00:00.000Z")
	setAddedAt(t, ctx, st, second, "2026-10-02T00:00:00.000Z")

	items, total, err := st.ListShelf(ctx, 1, 1)
	requireOK(t, "ListShelf", err)
	requireEqual(t, "书架总数", total, 2)
	requireEqual(t, "第一页长度", len(items), 1)
	requireEqual(t, "最近加入的排在前面", items[0].ID, second)
	if items[0].Title == "" {
		t.Error("书架行缺书名（BookBrief 未取到）")
	}
	requireEqual(t, "加入时间", items[0].AddedAt, "2026-10-02T00:00:00.000Z")

	page2, _, err := st.ListShelf(ctx, 2, 1)
	requireOK(t, "ListShelf(page=2)", err)
	requireEqual(t, "第二页", page2[0].ID, first)
}

// TestDeleteBookClearsShelf 删书要清掉书架行（它没有 books 外键，只有级联清单会清）。
func TestDeleteBookClearsShelf(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	requireOK(t, "AddToShelf", st.AddToShelf(ctx, bookID))
	requireOK(t, "DeleteBook", st.DeleteBook(ctx, bookID))
	requireEqual(t, "删书后书架行数", countRows(t, ctx, st, "user_shelf"), 0)
}

// setAddedAt 改一行的加入时间（排序断言用，避免依赖真实时钟）。
func setAddedAt(t *testing.T, ctx context.Context, st *Store, bookID int64, at string) {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE user_shelf SET added_at = ? WHERE book_id = ? AND user_id = ?",
		at, bookID, defaultUserID); err != nil {
		t.Fatalf("改加入时间: %v", err)
	}
}
