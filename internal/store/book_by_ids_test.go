package store

import (
	"context"
	"testing"
)

// seedCategoryBook 造一本带分类的书（推荐召回按分类/作者取书时用）。
func seedCategoryBook(t *testing.T, ctx context.Context, st *Store, title, author, l1, l2, sha string) int64 {
	t.Helper()
	b := sampleBook(title, author, "/books/"+title+".txt", sha)
	b.CategoryL1, b.CategoryL2 = l1, l2
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return res.BookID
}

func TestBooksByIDsKeepsInputOrder(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedCategoryBook(t, ctx, st, "甲书", "作者甲", "武侠", "", "a1")
	second := seedCategoryBook(t, ctx, st, "乙书", "作者乙", "都市", "", "b1")

	items, err := st.BooksByIDs(ctx, []int64{second, first})
	if err != nil {
		t.Fatalf("BooksByIDs: %v", err)
	}
	if len(items) != 2 || items[0].ID != second || items[1].ID != first {
		t.Fatalf("顺序 = %+v，期望按传入顺序 [%d %d]", items, second, first)
	}
	if items[1].CategoryL1 != "武侠" || items[1].AuthorName != "作者甲" {
		t.Fatalf("字段未按列口径填充: %+v", items[1])
	}
}

func TestBooksByIDsSkipsMissingAndDuplicates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedCategoryBook(t, ctx, st, "丙书", "作者丙", "", "", "c1")

	items, err := st.BooksByIDs(ctx, []int64{id, id, 0, -3, 99999})
	if err != nil {
		t.Fatalf("BooksByIDs: %v", err)
	}
	if len(items) != 1 || items[0].ID != id {
		t.Fatalf("应只返回存在的书一次，得到 %+v", items)
	}
	if empty, err := st.BooksByIDs(ctx, nil); err != nil || empty != nil {
		t.Fatalf("空入参应为 (nil, nil)，得到 %+v err=%v", empty, err)
	}
}

func TestBooksByIDsSkipsSuperseded(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := seedCategoryBook(t, ctx, st, "被取代的书", "作者丁", "", "", "d1")
	if _, err := st.db.ExecContext(ctx, "UPDATE books SET superseded_by = 7 WHERE id = ?", id); err != nil {
		t.Fatalf("标记取代: %v", err)
	}
	items, err := st.BooksByIDs(ctx, []int64{id})
	if err != nil {
		t.Fatalf("BooksByIDs: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("旧版本不应出现在推荐候选里，得到 %+v", items)
	}
}

// TestBooksByIDsChunksOverLimit 断言超量入参被分段查询，而不是报错。
//
// 回归的是「召回候选池 520 > 单条 IN 上限 500」：分段必须对调用方完全透明，
// 否则用户多出几个兴趣分类就拿到 500，而错误信息还指向 sqlite 而不是业务。
func TestBooksByIDsChunksOverLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	want := seedCategoryBook(t, ctx, st, "跨段书", "作者", "武侠", "", "chunk1")
	ids := make([]int64, maxIDsPerQuery+1)
	for i := range ids {
		ids[i] = int64(10000 + i)
	}
	ids[maxIDsPerQuery] = want // 唯一命中落在第二段
	items, err := st.BooksByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("超量应分段查询而不是报错: %v", err)
	}
	if len(items) != 1 || items[0].ID != want {
		t.Fatalf("跨段 id 应被查到，得到 %+v", items)
	}
}

func TestRandomBookSummaries(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for i, sha := range []string{"r1", "r2", "r3"} {
		seedCategoryBook(t, ctx, st, "随机书"+string(rune('A'+i)), "作者", "", "", sha)
	}
	all, err := st.RandomBookSummaries(ctx, 10)
	if err != nil || len(all) != 3 {
		t.Fatalf("n 超过总数应返回全部，得到 %d err=%v", len(all), err)
	}
	some, err := st.RandomBookSummaries(ctx, 2)
	if err != nil || len(some) != 2 {
		t.Fatalf("取 2 本得到 %d err=%v", len(some), err)
	}
	if some[0].ID == some[1].ID {
		t.Fatalf("随机取样不应重复: %+v", some)
	}
	if none, err := st.RandomBookSummaries(ctx, 0); err != nil || none != nil {
		t.Fatalf("n=0 应为 (nil, nil)，得到 %+v err=%v", none, err)
	}
}

func TestRandomBookSummariesEmptyLibrary(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	got, err := st.RandomBookSummaries(ctx, 5)
	if err != nil || got != nil {
		t.Fatalf("空库应返回 (nil, nil)，得到 %+v err=%v", got, err)
	}
}
