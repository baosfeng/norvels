package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 卷管理（C2）的库层口径：单事务、卷名唯一、idx 连续、移动只改 volume_id。
// 断言集中放在下面的辅助函数里，测试函数本身保持低分支（门禁 4 也扫测试文件）。

// seedVolumeFixture 写一本三卷四章的书：卷一 2 章、卷二 1 章、卷三 1 章。
// 返回 bookID、按卷序的卷 id、按目录序的章 id。
func seedVolumeFixture(t *testing.T, ctx context.Context, st *Store, name string) (int64, []int64, []int64) {
	t.Helper()
	vols := []model.ParsedVolume{
		{Idx: 0, Title: "卷一"},
		{Idx: 1, Title: "卷二"},
		{Idx: 2, Title: "卷三"},
	}
	chapters := []model.ParsedChapter{
		{Idx: 0, VolumeIdx: 0, Title: "第一章", Content: "卷一正文甲。", WordCount: 6},
		{Idx: 1, VolumeIdx: 0, Title: "第二章", Content: "卷一正文乙。", WordCount: 6},
		{Idx: 2, VolumeIdx: 1, Title: "第三章", Content: "卷二正文。", WordCount: 5},
		{Idx: 3, VolumeIdx: 2, Title: "第四章", Content: "卷三正文。", WordCount: 5},
	}
	bookID, ids := seedStructBook(t, ctx, st, name, vols, chapters)
	return bookID, volumeIDsOf(t, ctx, st, bookID), ids
}

// requireTrue 集中测试断言的分支（测试函数里不再散落 if）。
func requireTrue(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Fatalf(format, args...)
	}
}

// requireStoreErr 断言错误命中指定哨兵。
func requireStoreErr(t *testing.T, err, want error, what string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err = %v，期望 %v", what, err, want)
	}
}

// volumeIdxOf 读某个卷在库里的 idx。
func volumeIdxOf(t *testing.T, ctx context.Context, st *Store, volumeID int64) int {
	t.Helper()
	var idx int
	if err := st.DB().QueryRowContext(ctx, "SELECT idx FROM volumes WHERE id = ?", volumeID).Scan(&idx); err != nil {
		t.Fatalf("读卷 %d 的 idx: %v", volumeID, err)
	}
	return idx
}

// requireVolumeIDs 断言按卷序的卷 id 序列。
func requireVolumeIDs(t *testing.T, ctx context.Context, st *Store, bookID int64, want []int64) {
	t.Helper()
	got := volumeIDsOf(t, ctx, st, bookID)
	requireTrue(t, len(got) == len(want), "卷数 = %d，期望 %d（%v）", len(got), len(want), got)
	for i := range want {
		requireTrue(t, got[i] == want[i], "第 %d 卷 id = %d，期望 %d", i, got[i], want[i])
	}
}

// requireVolumeIdxSeq 断言 ids 的库里 idx 依次为 0..n-1。
func requireVolumeIdxSeq(t *testing.T, ctx context.Context, st *Store, ids []int64) {
	t.Helper()
	for i, id := range ids {
		requireTrue(t, volumeIdxOf(t, ctx, st, id) == i, "卷 %d 的 idx 不是 %d", id, i)
	}
}

// requireChapterLayout 断言目录结构（id/idx/字数/标题）逐一未变。
func requireChapterLayout(t *testing.T, before, after []ChapterBrief) {
	t.Helper()
	requireTrue(t, len(after) == len(before), "章数 = %d，期望 %d", len(after), len(before))
	for i := range before {
		requireTrue(t, after[i].ID == before[i].ID && after[i].Idx == before[i].Idx &&
			after[i].WordCount == before[i].WordCount && after[i].Title == before[i].Title,
			"第 %d 章被改动: before=%+v after=%+v", i, before[i], after[i])
	}
}

// requireChapterVolume 断言目录第 pos 章的卷归属。
func requireChapterVolume(t *testing.T, ctx context.Context, st *Store, bookID int64, pos int, want int64) {
	t.Helper()
	items := chaptersOf(t, ctx, st, bookID)
	requireTrue(t, pos < len(items), "章节目录只有 %d 章，取不到第 %d 章", len(items), pos)
	requireTrue(t, items[pos].VolumeID == want, "第 %d 章 volume_id = %d，期望 %d", pos, items[pos].VolumeID, want)
}

// bookVolumeCountOf 读 books.volume_count。
func bookVolumeCountOf(t *testing.T, ctx context.Context, st *Store, bookID int64) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx, "SELECT volume_count FROM books WHERE id = ?", bookID).Scan(&n); err != nil {
		t.Fatalf("读书 %d 的 volume_count: %v", bookID, err)
	}
	return n
}

// staleUpdatedAt 是一个明显过期的 updated_at，用来断言写操作确实刷新了它。
const staleUpdatedAt = "2000-01-01T00:00:00.000Z"

// markBookStale 把书的 updated_at 打回过期值（断言「写后刷新」用，避免同毫秒抖动）。
func markBookStale(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET updated_at = ? WHERE id = ?", staleUpdatedAt, bookID); err != nil {
		t.Fatalf("打旧书 %d 的 updated_at: %v", bookID, err)
	}
}

// bookUpdatedAtOf 读书的 updated_at。
func bookUpdatedAtOf(t *testing.T, ctx context.Context, st *Store, bookID int64) string {
	t.Helper()
	var ts string
	if err := st.DB().QueryRowContext(ctx, "SELECT updated_at FROM books WHERE id = ?", bookID).Scan(&ts); err != nil {
		t.Fatalf("读书 %d 的 updated_at: %v", bookID, err)
	}
	return ts
}

// requireBookTouched 断言写操作刷新了书的 updated_at。
func requireBookTouched(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	requireTrue(t, bookUpdatedAtOf(t, ctx, st, bookID) != staleUpdatedAt, "book %d 的 updated_at 未刷新", bookID)
}

// TestCreateVolumeAppendsIdx 新建卷：idx = 当前卷数、卷名去空白、volume_count 与 updated_at 同事务更新。
func TestCreateVolumeAppendsIdx(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "create")
	markBookStale(t, ctx, st, bookID)

	vol, err := st.CreateVolume(ctx, bookID, "  卷四  ")
	requireNoErr(t, err, "CreateVolume")
	requireTrue(t, vol.ID > 0 && vol.Idx == 3 && vol.Title == "卷四" && len(vol.Chapters) == 0,
		"新卷 = %+v，期望 idx=3/title=卷四/chapters=[]", vol)
	requireTrue(t, volumeIdxOf(t, ctx, st, vol.ID) == 3, "库里新卷 idx 不是 3")
	requireVolumeIDs(t, ctx, st, bookID, []int64{volumeIDs[0], volumeIDs[1], volumeIDs[2], vol.ID})
	requireTrue(t, bookVolumeCountOf(t, ctx, st, bookID) == 4, "volume_count 不是 4")
	requireBookTouched(t, ctx, st, bookID)
}

// TestCreateVolumeRejectsBadName 非法卷名一律不落库：空名 / 重名（去空白比较）/ 超长。
func TestCreateVolumeRejectsBadName(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, _, _ := seedVolumeFixture(t, ctx, st, "bad-name")

	cases := []struct {
		name  string
		title string
		want  error
	}{
		{"空白名", "   ", ErrInvalidArgument},
		{"重名去空白", " 卷一 ", ErrVolumeNameTaken},
		{"超长", strings.Repeat("卷", maxVolumeTitleRunes+1), ErrInvalidArgument},
	}
	for _, c := range cases {
		_, err := st.CreateVolume(ctx, bookID, c.title)
		requireStoreErr(t, err, c.want, c.name)
	}
	requireTrue(t, bookVolumeCountOf(t, ctx, st, bookID) == 3, "失败后 volume_count 不是 3")
}

// TestRenameVolume 改名成功只改 title；重名 409 哨兵、空名 400，两者都不动库。
func TestRenameVolume(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "rename")
	markBookStale(t, ctx, st, bookID)

	vol, err := st.RenameVolume(ctx, bookID, volumeIDs[1], "  新卷二 ")
	requireNoErr(t, err, "RenameVolume")
	requireTrue(t, vol.ID == volumeIDs[1] && vol.Idx == 1 && vol.Title == "新卷二" && len(vol.Chapters) == 0,
		"改名结果 = %+v", vol)
	requireBookTouched(t, ctx, st, bookID)

	_, err = st.RenameVolume(ctx, bookID, volumeIDs[1], "卷一")
	requireStoreErr(t, err, ErrVolumeNameTaken, "重名")
	_, err = st.RenameVolume(ctx, bookID, volumeIDs[1], "  ")
	requireStoreErr(t, err, ErrInvalidArgument, "空名")
	// 与自身同名（只是多了空白）允许：排除自己后不再算重名。
	_, err = st.RenameVolume(ctx, bookID, volumeIDs[1], " 新卷二 ")
	requireNoErr(t, err, "改回同名")
	requireTrue(t, realTitleOf(t, ctx, st, volumeIDs[1]) == "新卷二", "失败后卷名被改动")
}

// realTitleOf 读某个卷的标题。
func realTitleOf(t *testing.T, ctx context.Context, st *Store, volumeID int64) string {
	t.Helper()
	var title string
	if err := st.DB().QueryRowContext(ctx, "SELECT title FROM volumes WHERE id = ?", volumeID).Scan(&title); err != nil {
		t.Fatalf("读卷 %d 的标题: %v", volumeID, err)
	}
	return title
}

// TestRenameVolumeMissing 卷不属于本书 / 不存在都是 ErrNotFound，且不刷新另一本书。
func TestRenameVolumeMissing(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "rename-missing")
	otherBookID, _, _ := seedVolumeFixture(t, ctx, st, "rename-other")
	markBookStale(t, ctx, st, otherBookID)

	_, err := st.RenameVolume(ctx, otherBookID, volumeIDs[0], "跨书改名")
	requireStoreErr(t, err, ErrNotFound, "跨书改名")
	_, err = st.RenameVolume(ctx, bookID, 999999, "不存在")
	requireStoreErr(t, err, ErrNotFound, "卷不存在")
	requireTrue(t, bookUpdatedAtOf(t, ctx, st, otherBookID) == staleUpdatedAt, "另一本书的 updated_at 被刷新了")
}

// TestDeleteVolumeLandsChaptersInLooseVolume 删卷：章落未分卷、目录结构不变，
// 剩余卷 idx 重排成 0..n-1 且 volume_count 重算。
func TestDeleteVolumeLandsChaptersInLooseVolume(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "delete")
	before := chaptersOf(t, ctx, st, bookID)
	markBookStale(t, ctx, st, bookID)

	requireNoErr(t, st.DeleteVolume(ctx, bookID, volumeIDs[1]), "DeleteVolume")
	requireVolumeIDs(t, ctx, st, bookID, []int64{volumeIDs[0], volumeIDs[2]})
	requireVolumeIdxSeq(t, ctx, st, []int64{volumeIDs[0], volumeIDs[2]})
	requireTrue(t, bookVolumeCountOf(t, ctx, st, bookID) == 2, "volume_count 不是 2")
	requireBookTouched(t, ctx, st, bookID)

	after := chaptersOf(t, ctx, st, bookID)
	requireChapterLayout(t, before, after)
	requireChapterVolume(t, ctx, st, bookID, 2, 0)
	requireChapterVolume(t, ctx, st, bookID, 0, volumeIDs[0])
	requireChapterVolume(t, ctx, st, bookID, 3, volumeIDs[2])

	detail := bookByID(t, ctx, st, bookID)
	requireTrue(t, len(detail.Volumes) == 3, "详情卷数 = %d，期望 3（两卷 + 未分卷）", len(detail.Volumes))
	loose := detail.Volumes[len(detail.Volumes)-1]
	requireTrue(t, loose.ID == 0 && len(loose.Chapters) == 1 && loose.Chapters[0].ID == after[2].ID,
		"未分卷 = %+v，期望含第三章", loose)
}

// TestDeleteVolumeMissing 重复删同一个卷 → ErrNotFound，卷数与章节不受影响。
func TestDeleteVolumeMissing(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "delete-missing")
	requireNoErr(t, st.DeleteVolume(ctx, bookID, volumeIDs[1]), "首次删除")

	err := st.DeleteVolume(ctx, bookID, volumeIDs[1])
	requireStoreErr(t, err, ErrNotFound, "重复删除")
	requireTrue(t, bookVolumeCountOf(t, ctx, st, bookID) == 2, "失败后 volume_count 不是 2")
	requireTrue(t, len(chaptersOf(t, ctx, st, bookID)) == 4, "失败后章数不是 4")
}

// TestReorderVolumes 重排成功按新序写 idx，volume_count 不变、updated_at 刷新。
func TestReorderVolumes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "reorder")
	want := []int64{volumeIDs[2], volumeIDs[0], volumeIDs[1]}
	markBookStale(t, ctx, st, bookID)

	requireNoErr(t, st.ReorderVolumes(ctx, bookID, want), "ReorderVolumes")
	requireVolumeIDs(t, ctx, st, bookID, want)
	requireVolumeIdxSeq(t, ctx, st, want)
	requireTrue(t, bookVolumeCountOf(t, ctx, st, bookID) == 3, "volume_count 不是 3")
	requireBookTouched(t, ctx, st, bookID)
}

// TestReorderVolumesRejects 缺 id / 重复 / 非本书卷 / 非法 id 都是 400 且顺序不变。
func TestReorderVolumesRejects(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, _ := seedVolumeFixture(t, ctx, st, "reorder-reject")
	otherBookID, otherVols, _ := seedVolumeFixture(t, ctx, st, "reorder-other")

	cases := []struct {
		name string
		ids  []int64
	}{
		{"缺一卷", []int64{volumeIDs[0], volumeIDs[1]}},
		{"重复 id", []int64{volumeIDs[0], volumeIDs[0], volumeIDs[1]}},
		{"非本书卷", []int64{volumeIDs[0], volumeIDs[1], otherVols[0]}},
		{"非法 id", []int64{volumeIDs[0], volumeIDs[1], 0}},
	}
	for _, c := range cases {
		requireStoreErr(t, st.ReorderVolumes(ctx, bookID, c.ids), ErrInvalidArgument, c.name)
	}
	requireVolumeIDs(t, ctx, st, bookID, volumeIDs)
	requireTrue(t, bookVolumeCountOf(t, ctx, st, otherBookID) == 3, "另一本书 volume_count 被改动")
}

// TestMoveChaptersToVolume 移动只改 volume_id（idx / 字数不变），可移回未分卷。
func TestMoveChaptersToVolume(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, chapterIDs := seedVolumeFixture(t, ctx, st, "move")
	before := chaptersOf(t, ctx, st, bookID)
	markBookStale(t, ctx, st, bookID)

	moved, err := st.MoveChaptersToVolume(ctx, bookID, []int64{chapterIDs[0], chapterIDs[3]}, volumeIDs[1])
	requireNoErr(t, err, "MoveChaptersToVolume")
	requireTrue(t, moved == 2, "moved = %d，期望 2", moved)
	after := chaptersOf(t, ctx, st, bookID)
	requireChapterLayout(t, before, after)
	requireChapterVolume(t, ctx, st, bookID, 0, volumeIDs[1])
	requireChapterVolume(t, ctx, st, bookID, 3, volumeIDs[1])
	requireBookTouched(t, ctx, st, bookID)

	moved, err = st.MoveChaptersToVolume(ctx, bookID, []int64{chapterIDs[0]}, 0)
	requireNoErr(t, err, "移回未分卷")
	requireTrue(t, moved == 1, "移回未分卷 moved = %d，期望 1", moved)
	requireChapterVolume(t, ctx, st, bookID, 0, 0)
}

// TestMoveChaptersToVolumeRejects 跨书章节/卷、空数组、重复 id、负卷 id 全部拒绝且不动库。
func TestMoveChaptersToVolumeRejects(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID, volumeIDs, chapterIDs := seedVolumeFixture(t, ctx, st, "move-reject")
	_, otherVols, otherChapters := seedVolumeFixture(t, ctx, st, "move-other")

	cases := []struct {
		name     string
		ids      []int64
		volumeID int64
		want     error
	}{
		{"跨书章节", []int64{otherChapters[0]}, 0, ErrNotFound},
		{"跨书卷", []int64{chapterIDs[1]}, otherVols[0], ErrNotFound},
		{"不存在的卷", []int64{chapterIDs[1]}, 999999, ErrNotFound},
		{"空数组", nil, 0, ErrInvalidArgument},
		{"重复 id", []int64{chapterIDs[1], chapterIDs[1]}, 0, ErrInvalidArgument},
		{"负卷 id", []int64{chapterIDs[1]}, -1, ErrInvalidArgument},
	}
	for _, c := range cases {
		_, err := st.MoveChaptersToVolume(ctx, bookID, c.ids, c.volumeID)
		requireStoreErr(t, err, c.want, c.name)
	}
	requireChapterVolume(t, ctx, st, bookID, 1, volumeIDs[0])
}
