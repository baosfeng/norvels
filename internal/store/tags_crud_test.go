package store

import (
	"context"
	"strings"
	"testing"
)

// 标签 CRUD 的口径：同名返回已有（Created=false）、改名重名 409、
// 删除要全库解绑；名字去首尾空白后比较，超长直接拒绝而不是静默截断。

func TestListTagsWithCountMatchesListTags(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ids := seedRatingBooks(t, ctx, st, 2)
	requireOK(t, "SetBookTags 书1", st.SetBookTags(ctx, ids[0], []string{"玄幻", "悬疑"}))
	requireOK(t, "SetBookTags 书2", st.SetBookTags(ctx, ids[1], []string{"玄幻"}))

	withCount, err := st.ListTagsWithCount(ctx)
	requireOK(t, "ListTagsWithCount", err)
	legacy, err := st.ListTags(ctx)
	requireOK(t, "ListTags", err)
	requireEqual(t, "标签数", len(withCount), 2)
	requireEqual(t, "玄幻书籍数", withCount[0].Name, "玄幻")
	requireEqual(t, "玄幻计数", withCount[0].BookCount, 2)
	requireEqual(t, "两个入口行数一致", len(legacy), len(withCount))
	requireEqual(t, "两个入口顺序一致", legacy[0].ID, withCount[0].ID)
}

func TestCreateTagReusesSameName(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]

	created, err := st.CreateTag(ctx, "  玄幻 ")
	requireOK(t, "CreateTag", err)
	requireEqual(t, "新建标记", created.Created, true)
	requireEqual(t, "名字去空白", created.Name, "玄幻")
	requireEqual(t, "id 为正", created.ID > 0, true)

	again, err := st.CreateTag(ctx, "玄幻")
	requireOK(t, "CreateTag 同名", err)
	requireEqual(t, "同名不新建", again.Created, false)
	requireEqual(t, "同名返回同一行", again.ID, created.ID)

	requireOK(t, "SetBookTags", st.SetBookTags(ctx, bookID, []string{"玄幻"}))
	linked, err := st.CreateTag(ctx, "玄幻")
	requireOK(t, "CreateTag 已关联", err)
	requireEqual(t, "同名标签书籍数", linked.BookCount, 1)
}

func TestCreateTagValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, err := st.CreateTag(ctx, "  ")
	requireErr(t, "空名", err, ErrInvalidArgument)
	_, err = st.CreateTag(ctx, strings.Repeat("标", maxTagRunes+1))
	requireErr(t, "超长名", err, ErrInvalidArgument)
}

func TestRenameTag(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedTag(t, ctx, st, "甲标签")
	seedTag(t, ctx, st, "乙标签")

	requireOK(t, "RenameTag", st.RenameTag(ctx, first.ID, "新标签"))
	tags, err := st.ListTagsWithCount(ctx)
	requireOK(t, "ListTagsWithCount", err)
	requireEqual(t, "改名后存在新名字", tagIDByName(tags, "新标签"), first.ID)
}

func TestRenameTagErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := seedTag(t, ctx, st, "甲标签")
	seedTag(t, ctx, st, "乙标签")

	requireErr(t, "改成重名", st.RenameTag(ctx, first.ID, "乙标签"), ErrConflict)
	requireErr(t, "不存在的标签", st.RenameTag(ctx, 999999, "随便"), ErrNotFound)
	requireErr(t, "空名", st.RenameTag(ctx, first.ID, " "), ErrInvalidArgument)
	requireErr(t, "id=0", st.RenameTag(ctx, 0, "随便"), ErrInvalidArgument)
}

func TestDeleteTagUnbindsBooks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedRatingBooks(t, ctx, st, 1)[0]
	requireOK(t, "SetBookTags", st.SetBookTags(ctx, bookID, []string{"玄幻", "悬疑"}))
	tags, err := st.ListTagsWithCount(ctx)
	requireOK(t, "ListTagsWithCount", err)
	xuanhuan := tagIDByName(tags, "玄幻")
	requireEqual(t, "玄幻标签存在", xuanhuan > 0, true)

	requireOK(t, "DeleteTag", st.DeleteTag(ctx, xuanhuan))
	remaining, err := st.ListBookTags(ctx, bookID)
	requireOK(t, "ListBookTags", err)
	requireSliceEqual(t, "删标签后的书标签", remaining, []string{"悬疑"})
	_, err = st.GetBook(ctx, bookID)
	requireOK(t, "书本身不动", err)
}

func TestDeleteTagErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	tag := seedTag(t, ctx, st, "会删的标签")
	requireOK(t, "DeleteTag", st.DeleteTag(ctx, tag.ID))

	requireErr(t, "重复删除", st.DeleteTag(ctx, tag.ID), ErrNotFound)
	requireErr(t, "id=0", st.DeleteTag(ctx, 0), ErrInvalidArgument)
}

// seedTag 建一个标签，失败即 Fatal。
func seedTag(t *testing.T, ctx context.Context, st *Store, name string) TagSummary {
	t.Helper()
	tag, err := st.CreateTag(ctx, name)
	if err != nil {
		t.Fatalf("CreateTag(%q): %v", name, err)
	}
	return tag
}

// tagIDByName 返回指定名字的标签 id；找不到返回 0。
func tagIDByName(tags []TagSummary, name string) int64 {
	for _, tag := range tags {
		if tag.Name == name {
			return tag.ID
		}
	}
	return 0
}
