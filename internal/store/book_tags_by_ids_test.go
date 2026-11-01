package store

import (
	"context"
	"testing"
)

func TestBookTagsByIDs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedCategoryBook(t, ctx, st, "标签批查甲", "作者甲", "", "", "bt1")
	second := seedCategoryBook(t, ctx, st, "标签批查乙", "作者乙", "", "", "bt2")
	third := seedCategoryBook(t, ctx, st, "无标签书", "作者丙", "", "", "bt3")

	if err := st.SetBookTags(ctx, first, []string{"武侠", "群像"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	if err := st.SetBookTags(ctx, second, []string{"科幻"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}

	got, err := st.BookTagsByIDs(ctx, []int64{first, second, third, first, 0, -1, 999999})
	if err != nil {
		t.Fatalf("BookTagsByIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("返回条目 = %d，期望 2（无标签/不存在/重复都不计）: %v", len(got), got)
	}
	// 名字升序是 UTF-8 字节序：「武」(U+6B66) 在「群」(U+7FA4) 之前。
	if tags := got[first]; len(tags) != 2 || tags[0] != "武侠" || tags[1] != "群像" {
		t.Fatalf("书 %d 标签 = %v，期望按名字升序 [武侠 群像]", first, tags)
	}
	if _, ok := got[third]; ok {
		t.Fatal("没标签的书不该出现在 map 里")
	}
}

func TestBookTagsByIDsEmptyInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if got, err := st.BookTagsByIDs(ctx, nil); err != nil || got != nil {
		t.Fatalf("空入参应为 (nil, nil)，得到 %v err=%v", got, err)
	}
}

// TestBookTagsByIDsChunksOverLimit 断言超量入参被分段查询（不报错、不丢结果）。
func TestBookTagsByIDsChunksOverLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := make([]int64, maxIDsPerQuery+1)
	for i := range ids {
		ids[i] = int64(10000 + i)
	}
	got, err := st.BookTagsByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("超量应分段查询而不是报错: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("不存在的 id 不应出现在 map 里，得到 %v", got)
	}
}

func TestBookTagsByIDsOnClosedDB(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := st.BookTagsByIDs(ctx, []int64{1}); err == nil {
		t.Fatal("库已关闭时应报错")
	}
}
