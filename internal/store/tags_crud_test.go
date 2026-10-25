package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 标签 CRUD 的口径：同名返回已有（Created=false）、改名重名 409、
// 删除要全库解绑；名字去首尾空白后比较，超长直接拒绝而不是静默截断。

func TestListTagsWithCountMatchesListTags(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	if err := st.SetBookTags(ctx, ids[0], []string{"玄幻", "悬疑"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	if err := st.SetBookTags(ctx, ids[1], []string{"玄幻"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}

	withCount, err := st.ListTagsWithCount(ctx)
	if err != nil {
		t.Fatalf("ListTagsWithCount: %v", err)
	}
	legacy, err := st.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(withCount) != 2 || withCount[0].Name != "玄幻" || withCount[0].BookCount != 2 {
		t.Fatalf("标签聚合 = %+v，期望玄幻 2 本在前", withCount)
	}
	if len(legacy) != len(withCount) || legacy[0].ID != withCount[0].ID {
		t.Errorf("两个入口口径不一致: %+v vs %+v", legacy, withCount)
	}
}

func TestCreateTag(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]

	created, err := st.CreateTag(ctx, "  玄幻 ")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if !created.Created || created.ID <= 0 || created.Name != "玄幻" {
		t.Fatalf("新建标签 = %+v，期望去空白且 Created=true", created)
	}
	again, err := st.CreateTag(ctx, "玄幻")
	if err != nil {
		t.Fatalf("CreateTag 同名: %v", err)
	}
	if again.Created || again.ID != created.ID {
		t.Errorf("同名标签 = %+v，期望返回已有且 Created=false", again)
	}
	if err := st.SetBookTags(ctx, bookID, []string{"玄幻"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	linked, err := st.CreateTag(ctx, "玄幻")
	if err != nil || linked.BookCount != 1 {
		t.Errorf("同名标签书籍数 = %d err=%v，期望 1", linked.BookCount, err)
	}
	if _, err := st.CreateTag(ctx, "  "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空名 err = %v，期望 ErrInvalidArgument", err)
	}
	long := strings.Repeat("标", maxTagRunes+1)
	if _, err := st.CreateTag(ctx, long); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("超长名 err = %v，期望 ErrInvalidArgument", err)
	}
}

func TestRenameTag(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.CreateTag(ctx, "甲标签")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := st.CreateTag(ctx, "乙标签"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if err := st.RenameTag(ctx, first.ID, "乙标签"); !errors.Is(err, ErrConflict) {
		t.Errorf("改成重名 err = %v，期望 ErrConflict", err)
	}
	if err := st.RenameTag(ctx, first.ID, "新标签"); err != nil {
		t.Errorf("正常改名: %v", err)
	}
	tags, err := st.ListTagsWithCount(ctx)
	if err != nil {
		t.Fatalf("ListTagsWithCount: %v", err)
	}
	found := false
	for _, tag := range tags {
		found = found || (tag.ID == first.ID && tag.Name == "新标签")
	}
	if !found {
		t.Errorf("改名未生效: %+v", tags)
	}
	if err := st.RenameTag(ctx, 999999, "随便"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的标签 err = %v，期望 ErrNotFound", err)
	}
	if err := st.RenameTag(ctx, first.ID, " "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空名 err = %v，期望 ErrInvalidArgument", err)
	}
	if err := st.RenameTag(ctx, 0, "随便"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
}

func TestDeleteTag(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]
	if err := st.SetBookTags(ctx, bookID, []string{"玄幻", "悬疑"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	tags, err := st.ListTagsWithCount(ctx)
	if err != nil {
		t.Fatalf("ListTagsWithCount: %v", err)
	}
	var xuanhuan int64
	for _, tag := range tags {
		if tag.Name == "玄幻" {
			xuanhuan = tag.ID
		}
	}
	if xuanhuan == 0 {
		t.Fatalf("没有玄幻标签: %+v", tags)
	}

	if err := st.DeleteTag(ctx, xuanhuan); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	remaining, err := st.ListBookTags(ctx, bookID)
	if err != nil || len(remaining) != 1 || remaining[0] != "悬疑" {
		t.Errorf("删标签后书的标签 = %v err=%v，期望只剩悬疑", remaining, err)
	}
	// 书本身不动。
	if _, err := st.GetBook(ctx, bookID); err != nil {
		t.Errorf("删标签把书弄丢了: %v", err)
	}
	if err := st.DeleteTag(ctx, xuanhuan); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 err = %v，期望 ErrNotFound", err)
	}
	if err := st.DeleteTag(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("id=0 err = %v，期望 ErrInvalidArgument", err)
	}
}
