package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 书单的口径：名字去首尾空白后全库唯一（重名 409）、书单不存在 404、
// 载荷里出现不存在的书是 400；全量替换空列表即清空。

func TestCollectionsCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if items, err := st.ListCollections(ctx); err != nil || len(items) != 0 {
		t.Fatalf("空库书单 = %+v err=%v", items, err)
	}

	c, err := st.CreateCollection(ctx, "  想读  ")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if c.ID <= 0 || c.Name != "想读" || c.BookCount != 0 {
		t.Errorf("新建书单 = %+v，期望名字去空白、书目数 0", c)
	}
	if _, err := st.CreateCollection(ctx, "想读"); !errors.Is(err, ErrConflict) {
		t.Errorf("重名 err = %v，期望 ErrConflict", err)
	}
	if _, err := st.CreateCollection(ctx, "   "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空名 err = %v，期望 ErrInvalidArgument", err)
	}
	if _, err := st.CreateCollection(ctx, strings.Repeat("书", maxCollectionNameRunes+1)); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("超长名 err = %v，期望 ErrInvalidArgument", err)
	}
	items, err := st.ListCollections(ctx)
	if err != nil || len(items) != 1 || items[0].ID != c.ID {
		t.Errorf("书单列表 = %+v err=%v", items, err)
	}
}

func TestRenameCollection(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.CreateCollection(ctx, "甲单")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if _, err := st.CreateCollection(ctx, "乙单"); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	if err := st.RenameCollection(ctx, first.ID, "乙单"); !errors.Is(err, ErrConflict) {
		t.Errorf("改成重名 err = %v，期望 ErrConflict", err)
	}
	if err := st.RenameCollection(ctx, first.ID, "新名"); err != nil {
		t.Errorf("正常改名: %v", err)
	}
	if err := st.RenameCollection(ctx, 999999, "随便"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的书单 err = %v，期望 ErrNotFound", err)
	}
	if err := st.RenameCollection(ctx, first.ID, " "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空名 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.RenameCollection(ctx, 0, "随便"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
}

func TestReplaceCollectionBooks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	c, err := st.CreateCollection(ctx, "待读")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	requireCollection(t, ctx, st, c.ID, 2, func() error {
		return st.ReplaceCollectionBooks(ctx, c.ID, ids)
	})
	// 书单筛选与书单内书数同口径。
	page, err := st.ListBooks(ctx, BookQuery{Collection: c.ID, Page: 1, Size: 10})
	if err != nil || page.Total != 2 {
		t.Errorf("按书单筛书 = %+v err=%v", page, err)
	}

	requireCollection(t, ctx, st, c.ID, 1, func() error {
		return st.ReplaceCollectionBooks(ctx, c.ID, []int64{ids[0], ids[0]})
	})
	requireCollection(t, ctx, st, c.ID, 0, func() error {
		return st.ReplaceCollectionBooks(ctx, c.ID, nil)
	})

	if err := st.ReplaceCollectionBooks(ctx, c.ID, []int64{999999}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("不存在的书 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.ReplaceCollectionBooks(ctx, 999999, ids); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的书单 err = %v，期望 ErrNotFound", err)
	}
	if err := st.ReplaceCollectionBooks(ctx, c.ID, []int64{0}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
	tooMany := make([]int64, maxCollectionBooks+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	if err := st.ReplaceCollectionBooks(ctx, c.ID, tooMany); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("超上限 err = %v，期望 ErrInvalidArgument", err)
	}
}

func TestDeleteCollectionAndBookCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	c, err := st.CreateCollection(ctx, "会删的单")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := st.ReplaceCollectionBooks(ctx, c.ID, ids); err != nil {
		t.Fatalf("ReplaceCollectionBooks: %v", err)
	}
	// 删书只摘掉这一本，书单与另一本都还在。
	if err := st.DeleteBook(ctx, ids[0]); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	requireCollection(t, ctx, st, c.ID, 1, nil)

	if err := st.DeleteCollection(ctx, c.ID); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	if n := countRows(t, ctx, st, "collection_books"); n != 0 {
		t.Errorf("删除书单后 collection_books 残留 %d 行", n)
	}
	if err := st.DeleteCollection(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 err = %v，期望 ErrNotFound", err)
	}
	if err := st.DeleteCollection(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
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
			if c.BookCount != want {
				t.Errorf("书单 %d 书目数 = %d，期望 %d", id, c.BookCount, want)
			}
			return
		}
	}
	t.Fatalf("书单 %d 不在列表里: %+v", id, items)
}
