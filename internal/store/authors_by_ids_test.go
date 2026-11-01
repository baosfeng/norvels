package store

import (
	"context"
	"testing"
)

// authorIDsOfBooks 取一批书的作者 id（造数据时用：SaveBook 会自动建作者）。
func authorIDsOfBooks(t *testing.T, ctx context.Context, st *Store, bookIDs []int64) []int64 {
	t.Helper()
	books, err := st.BooksByIDs(ctx, bookIDs)
	if err != nil {
		t.Fatalf("BooksByIDs: %v", err)
	}
	out := make([]int64, 0, len(books))
	for _, b := range books {
		out = append(out, b.AuthorID)
	}
	return out
}

func TestAuthorNamesByIDs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedCategoryBook(t, ctx, st, "作者名甲书", "作者甲", "", "", "an1")
	second := seedCategoryBook(t, ctx, st, "作者名乙书", "作者乙", "", "", "an2")
	ids := authorIDsOfBooks(t, ctx, st, []int64{first, second})

	names, err := st.AuthorNamesByIDs(ctx, append(ids, ids[0], 0, -7, 999999))
	if err != nil {
		t.Fatalf("AuthorNamesByIDs: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("返回条目 = %d，期望 2（重复与非正 id 不计）: %v", len(names), names)
	}
	if names[ids[0]] != "作者甲" || names[ids[1]] != "作者乙" {
		t.Fatalf("作者名 = %v", names)
	}
	// 缺失 id 不出现在 map 里：调用方据此回退展示。
	if _, ok := names[999999]; ok {
		t.Fatal("不存在的作者不该出现在 map 里")
	}
}

func TestAuthorNamesByIDsEmptyInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if got, err := st.AuthorNamesByIDs(ctx, nil); err != nil || got != nil {
		t.Fatalf("空入参应为 (nil, nil)，得到 %v err=%v", got, err)
	}
	// 只有非正 id 时同样视为「没有要查的对象」。
	if got, err := st.AuthorNamesByIDs(ctx, []int64{0, -3}); err != nil || got != nil {
		t.Fatalf("全是非正 id 应为 (nil, nil)，得到 %v err=%v", got, err)
	}
}

// TestAuthorNamesByIDsChunksOverLimit 断言超量入参被分段查询（不报错、不丢结果）。
func TestAuthorNamesByIDsChunksOverLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := make([]int64, maxIDsPerQuery+1)
	for i := range ids {
		ids[i] = int64(10000 + i)
	}
	got, err := st.AuthorNamesByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("超量应分段查询而不是报错: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("不存在的 id 不应出现在 map 里，得到 %v", got)
	}
}

func TestAuthorNamesByIDsOnClosedDB(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := st.AuthorNamesByIDs(ctx, []int64{1}); err == nil {
		t.Fatal("库已关闭时应报错")
	}
}
