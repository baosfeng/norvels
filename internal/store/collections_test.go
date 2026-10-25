package store

import (
	"context"
	"strings"
	"testing"
)

// 书单的口径：名字去首尾空白后全库唯一（重名 409）、书单不存在 404、
// 载荷里出现不存在的书是 400；全量替换空列表即清空。

func TestCollectionsCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	items, err := st.ListCollections(ctx)
	requireOK(t, "空库书单", err)
	requireEqual(t, "空库书单行数", len(items), 0)

	c, err := st.CreateCollection(ctx, "  想读  ")
	requireOK(t, "CreateCollection", err)
	requireEqual(t, "名字去空白", c.Name, "想读")
	requireEqual(t, "书目数", c.BookCount, 0)
	requireEqual(t, "id 为正", c.ID > 0, true)

	items, err = st.ListCollections(ctx)
	requireOK(t, "ListCollections", err)
	requireEqual(t, "书单行数", len(items), 1)
	requireEqual(t, "书单 id", items[0].ID, c.ID)
}

func TestCollectionsValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.CreateCollection(ctx, "想读")
	requireOK(t, "CreateCollection", err)

	_, err = st.CreateCollection(ctx, "想读")
	requireErr(t, "重名", err, ErrConflict)
	_, err = st.CreateCollection(ctx, "   ")
	requireErr(t, "空名", err, ErrInvalidArgument)
	_, err = st.CreateCollection(ctx, strings.Repeat("书", maxCollectionNameRunes+1))
	requireErr(t, "超长名", err, ErrInvalidArgument)
	requireErr(t, "改名空名", st.RenameCollection(ctx, first.ID, " "), ErrInvalidArgument)
	requireErr(t, "改名 id=0", st.RenameCollection(ctx, 0, "随便"), ErrInvalidArgument)
	requireErr(t, "删除 id=0", st.DeleteCollection(ctx, 0), ErrInvalidArgument)
}

func TestRenameCollection(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.CreateCollection(ctx, "甲单")
	requireOK(t, "CreateCollection 甲", err)
	_, err = st.CreateCollection(ctx, "乙单")
	requireOK(t, "CreateCollection 乙", err)

	requireErr(t, "改成重名", st.RenameCollection(ctx, first.ID, "乙单"), ErrConflict)
	requireOK(t, "正常改名", st.RenameCollection(ctx, first.ID, "新名"))
	requireErr(t, "不存在的书单", st.RenameCollection(ctx, 999999, "随便"), ErrNotFound)
}

func TestReplaceCollectionBooks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	c, err := st.CreateCollection(ctx, "待读")
	requireOK(t, "CreateCollection", err)

	requireCollection(t, ctx, st, c.ID, 2, func() error {
		return st.ReplaceCollectionBooks(ctx, c.ID, ids)
	})
	page, err := st.ListBooks(ctx, BookQuery{Collection: c.ID, Page: 1, Size: 10})
	requireOK(t, "按书单筛书", err)
	requireEqual(t, "按书单筛书总数", page.Total, 2)

	requireCollection(t, ctx, st, c.ID, 1, func() error {
		return st.ReplaceCollectionBooks(ctx, c.ID, []int64{ids[0], ids[0]})
	})
	requireCollection(t, ctx, st, c.ID, 0, func() error {
		return st.ReplaceCollectionBooks(ctx, c.ID, nil)
	})

	requireErr(t, "不存在的书", st.ReplaceCollectionBooks(ctx, c.ID, []int64{999999}), ErrInvalidArgument)
	requireErr(t, "不存在的书单", st.ReplaceCollectionBooks(ctx, 999999, ids), ErrNotFound)
	requireErr(t, "书 id=0", st.ReplaceCollectionBooks(ctx, c.ID, []int64{0}), ErrInvalidArgument)
	tooMany := make([]int64, maxCollectionBooks+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	requireErr(t, "超上限", st.ReplaceCollectionBooks(ctx, c.ID, tooMany), ErrInvalidArgument)
}

func TestDeleteCollectionAndBookCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	c, err := st.CreateCollection(ctx, "会删的单")
	requireOK(t, "CreateCollection", err)
	requireOK(t, "ReplaceCollectionBooks", st.ReplaceCollectionBooks(ctx, c.ID, ids))

	// 删书只摘掉这一本，书单与另一本都还在。
	requireOK(t, "DeleteBook", st.DeleteBook(ctx, ids[0]))
	requireCollection(t, ctx, st, c.ID, 1, nil)

	requireOK(t, "DeleteCollection", st.DeleteCollection(ctx, c.ID))
	requireEqual(t, "删书单后关联行数", countRows(t, ctx, st, "collection_books"), 0)
	requireErr(t, "重复删除", st.DeleteCollection(ctx, c.ID), ErrNotFound)
}

// requireCollection 执行一次替换（fn 可为 nil 表示不替换），并断言书单书目数。
func requireCollection(t *testing.T, ctx context.Context, st *Store, id int64, want int, fn func() error) {
	t.Helper()
	if fn != nil {
		if err := fn(); err != nil {
			t.Fatalf("ReplaceCollectionBooks: %v", err)
		}
	}
	items, err := st.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	for _, c := range items {
		if c.ID == id {
			requireEqual(t, "书目数", c.BookCount, want)
			return
		}
	}
	t.Fatalf("书单 %d 不在列表里: %+v", id, items)
}
