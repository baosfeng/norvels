package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"norvels/internal/store"
)

// 卷管理（C2）的接口口径：新建 / 改名 / 删除 / 重排 + 章节跨卷移动。
// 五个端点都即时生效；失败路径必须「原样不动库」，每个失败用例都回读库断言。

// volumeEnvelope 与卷响应体同形（chapters 恒为空数组）。
type volumeEnvelope struct {
	Volume struct {
		ID       int64                `json:"id"`
		Idx      int                  `json:"idx"`
		Title    string               `json:"title"`
		Chapters []store.ChapterBrief `json:"chapters"`
	} `json:"volume"`
}

// requireStatus 断言 HTTP 状态码。
func requireStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("状态码 = %d，期望 %d，body=%s", rr.Code, want, rr.Body.String())
	}
}

// requireTrue 集中断言分支（测试函数里不再散落 if）。
func requireTrue(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Fatalf(format, args...)
	}
}

// requireChapterItemsKept 断言目录（id/idx/字数）逐一未变。
func requireChapterItemsKept(t *testing.T, before, after []store.ChapterBrief) {
	t.Helper()
	requireTrue(t, len(after) == len(before), "章数 = %d，期望 %d", len(after), len(before))
	for i := range before {
		requireTrue(t, after[i].ID == before[i].ID && after[i].Idx == before[i].Idx &&
			after[i].WordCount == before[i].WordCount,
			"第 %d 章被改动: before=%+v after=%+v", i, before[i], after[i])
	}
}

// requireItemVolume 断言目录第 pos 章的 volumeId。
func requireItemVolume(t *testing.T, items []store.ChapterBrief, pos int, want int64) {
	t.Helper()
	requireTrue(t, pos < len(items), "目录只有 %d 章，取不到第 %d 章", len(items), pos)
	requireTrue(t, items[pos].VolumeID == want, "第 %d 章 volumeId = %d，期望 %d", pos, items[pos].VolumeID, want)
}

func volumesPath(bookID int64) string {
	return "/api/books/" + strconv.FormatInt(bookID, 10) + "/volumes"
}

func volumePathOf(bookID, volumeID int64) string {
	return volumesPath(bookID) + "/" + strconv.FormatInt(volumeID, 10)
}

// detailVolumes 读书籍详情里的卷列表（含未分卷伪卷）。
func detailVolumes(t *testing.T, env *testEnv, bookID int64) []store.VolumeNode {
	t.Helper()
	return decodeBody[store.BookDetail](t, env.get(bookPath(bookID))).Volumes
}

// realVolumes 只保留真实卷（去掉伪卷 id=0）。
func realVolumes(t *testing.T, env *testEnv, bookID int64) []store.VolumeNode {
	t.Helper()
	out := make([]store.VolumeNode, 0, 4)
	for _, v := range detailVolumes(t, env, bookID) {
		if v.ID > 0 {
			out = append(out, v)
		}
	}
	return out
}

// TestCreateVolumeAPI 新建卷：201 + 卷节点；重名 409 volume_name_taken、空名 400，失败不落库。
func TestCreateVolumeAPI(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("create")

	rr := env.postJSON(volumesPath(bookID), `{"title":"  卷三  "}`)
	requireStatus(t, rr, http.StatusCreated)
	body := decodeBody[volumeEnvelope](t, rr)
	requireTrue(t, body.Volume.ID > 0 && body.Volume.Idx == 2 && body.Volume.Title == "卷三" &&
		len(body.Volume.Chapters) == 0, "新卷 = %+v，期望 idx=2/title=卷三/chapters=[]", body.Volume)
	requireTrue(t, len(realVolumes(t, env, bookID)) == 3, "详情卷数不是 3")

	requireError(t, env.postJSON(volumesPath(bookID), `{"title":" 卷一 "}`),
		http.StatusConflict, codeVolumeNameTaken)
	requireError(t, env.postJSON(volumesPath(bookID), `{"title":"   "}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.postJSON(volumesPath(bookID), `{}`),
		http.StatusBadRequest, codeInvalidParam)
	requireTrue(t, len(realVolumes(t, env, bookID)) == 3, "失败后卷数不是 3")
}

// TestRenameVolumeAPI 改名：200 + 卷节点；重名 409、空名 400、卷/书不匹配 404，失败不改库。
func TestRenameVolumeAPI(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("rename")
	vols := env.volumeIDs(bookID)
	target := volumePathOf(bookID, vols[1])

	rr := env.patch(target, `{"title":"新卷二"}`)
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[volumeEnvelope](t, rr)
	requireTrue(t, body.Volume.ID == vols[1] && body.Volume.Idx == 1 &&
		body.Volume.Title == "新卷二" && len(body.Volume.Chapters) == 0, "改名结果 = %+v", body.Volume)

	requireError(t, env.patch(target, `{"title":"卷一"}`), http.StatusConflict, codeVolumeNameTaken)
	requireError(t, env.patch(target, `{"title":"  "}`), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.patch(volumePathOf(bookID, 999999), `{"title":"不存在"}`),
		http.StatusNotFound, codeNotFound)

	otherID, _ := env.seedVolumeBook("rename-other")
	otherVols := env.volumeIDs(otherID)
	requireError(t, env.patch(volumePathOf(bookID, otherVols[0]), `{"title":"跨书"}`),
		http.StatusNotFound, codeNotFound)
	requireTrue(t, realVolumes(t, env, bookID)[1].Title == "新卷二", "失败后卷名被改动")
}

// TestDeleteVolumeAPI 删卷：204；章落未分卷、idx/字数不变，剩余卷 idx 重排。
func TestDeleteVolumeAPI(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("delete")
	vols := env.volumeIDs(bookID)
	before := env.bookChapters(bookID).Items

	rr := env.do(http.MethodDelete, volumePathOf(bookID, vols[0]), nil, nil)
	requireStatus(t, rr, http.StatusNoContent)
	after := env.bookChapters(bookID).Items
	requireChapterItemsKept(t, before, after)
	for _, pos := range []int{0, 1, 2} {
		requireItemVolume(t, after, pos, 0)
	}
	requireItemVolume(t, after, 3, vols[1])

	rest := realVolumes(t, env, bookID)
	requireTrue(t, len(rest) == 1 && rest[0].ID == vols[1] && rest[0].Idx == 0,
		"剩余卷 = %+v，期望卷二且 idx=0", rest)
	volumes := detailVolumes(t, env, bookID)
	tail := volumes[len(volumes)-1]
	requireTrue(t, tail.ID == 0 && len(tail.Chapters) == 3, "未分卷 = %+v，期望 3 章", tail)

	requireError(t, env.do(http.MethodDelete, volumePathOf(bookID, vols[0]), nil, nil),
		http.StatusNotFound, codeNotFound)
	requireTrue(t, env.bookChapters(bookID).Total == 4, "失败后章数不是 4")
}

// TestReorderVolumesAPI 重排：200 后详情按新序；缺 id/重复/非本书卷/非法 id 都是 400 且不动库。
func TestReorderVolumesAPI(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("order")
	vols := env.volumeIDs(bookID)
	orderPath := volumesPath(bookID) + "/order"

	rr := env.putJSON(orderPath, map[string]any{"ids": []int64{vols[1], vols[0]}})
	requireStatus(t, rr, http.StatusOK)
	rest := realVolumes(t, env, bookID)
	requireTrue(t, len(rest) == 2 && rest[0].ID == vols[1] && rest[1].ID == vols[0],
		"新卷序 = %+v，期望卷二在前", rest)

	otherID, _ := env.seedVolumeBook("order-other")
	otherVols := env.volumeIDs(otherID)
	cases := []struct {
		name string
		ids  []int64
	}{
		{"缺 id", []int64{vols[0]}},
		{"重复 id", []int64{vols[0], vols[0]}},
		{"非本书卷", []int64{vols[0], otherVols[0]}},
		{"非法 id", []int64{vols[0], 0}},
	}
	for _, c := range cases {
		requireError(t, env.putJSON(orderPath, map[string]any{"ids": c.ids}),
			http.StatusBadRequest, codeInvalidParam)
	}
	got := realVolumes(t, env, bookID)
	requireTrue(t, got[0].ID == vols[1], "失败后卷序被改动: %+v", got)
}

// TestMoveChaptersAPI 章节跨卷移动：200 {"moved":n}；只改 volumeId，可移回未分卷；
// 跨书章节/跨书卷 404、空/重复 400，失败不动库。
func TestMoveChaptersAPI(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedVolumeBook("move")
	vols := env.volumeIDs(bookID)
	movePath := chaptersPath(bookID) + "move"
	before := env.bookChapters(bookID).Items

	rr := env.postJSON(movePath, `{"chapterIds":[`+itoa64(ids[0])+`,`+itoa64(ids[3])+`],`+
		`"volumeId":`+itoa64(vols[1])+`}`)
	requireStatus(t, rr, http.StatusOK)
	requireTrue(t, decodeBody[moveChaptersResponse](t, rr).Moved == 2, "moved 不是 2")
	after := env.bookChapters(bookID).Items
	requireChapterItemsKept(t, before, after)
	requireItemVolume(t, after, 0, vols[1])
	requireItemVolume(t, after, 3, vols[1])

	rr = env.postJSON(movePath, `{"chapterIds":[`+itoa64(ids[0])+`],"volumeId":0}`)
	requireStatus(t, rr, http.StatusOK)
	requireTrue(t, decodeBody[moveChaptersResponse](t, rr).Moved == 1, "移回未分卷 moved 不是 1")
	requireItemVolume(t, env.bookChapters(bookID).Items, 0, 0)

	otherID, otherIDs := env.seedVolumeBook("move-other")
	otherVols := env.volumeIDs(otherID)
	requireError(t, env.postJSON(movePath, `{"chapterIds":[`+itoa64(otherIDs[0])+`],"volumeId":0}`),
		http.StatusNotFound, codeNotFound)
	requireError(t, env.postJSON(movePath, `{"chapterIds":[`+itoa64(ids[1])+`],`+
		`"volumeId":`+itoa64(otherVols[0])+`}`), http.StatusNotFound, codeNotFound)
	requireError(t, env.postJSON(movePath, `{"chapterIds":[],"volumeId":0}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.postJSON(movePath, `{"chapterIds":[`+itoa64(ids[1])+`,`+itoa64(ids[1])+`],`+
		`"volumeId":0}`), http.StatusBadRequest, codeInvalidParam)
	requireItemVolume(t, env.bookChapters(bookID).Items, 1, vols[0])
}

// TestVolumeRouteBadID 路径 id / 卷 id 非正整数：400 invalid_id，不进 store。
func TestVolumeRouteBadID(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("badid")
	requireError(t, env.postJSON("/api/books/abc/volumes", `{"title":"卷"}`),
		http.StatusBadRequest, codeInvalidID)
	requireError(t, env.patch(volumesPath(bookID)+"/abc", `{"title":"卷"}`),
		http.StatusBadRequest, codeInvalidID)
}
