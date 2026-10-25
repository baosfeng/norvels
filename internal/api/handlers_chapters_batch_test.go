package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 章节批量编辑（管理页 M2）的接口口径：批量删除 / 合并 / 重编号。
// 三类失败都必须「原样不动库」：一次请求要么全成、要么全不动。

// chaptersPath 拼接「某本书的章节集合」前缀（三个批量端点共用）。
func chaptersPath(bookID int64) string {
	return "/api/books/" + strconv.FormatInt(bookID, 10) + "/chapters/"
}

// seedVolumeBook 写一本两卷四章的书：卷一连续三章、卷二一章。
//
// 卷结构是这两类 409 的前提——「跨卷」要两个真实卷，「不连续」要同一卷里挑开一章；
// 单卷的 seedBook / seedParagraphBook 造不出来。tag 用来在同一库里放两本互相独立的
// 样本书（SaveBook 按 SHA256 去重，不换指纹第二次会返回同一本）。
func (e *testEnv) seedVolumeBook(tag string) (bookID int64, ids []int64) {
	e.t.Helper()
	sum := sha256.Sum256([]byte("norvels-test:volume-book:" + tag))
	book := &model.ParsedBook{
		Title:      "分卷样本" + tag,
		Author:     "样本作者",
		SourcePath: filepath.Join(e.t.TempDir(), "volume-"+tag+".txt"),
		SourceName: "volume-" + tag + ".txt",
		Encoding:   "utf-8",
		FileSize:   4096,
		SHA256:     hex.EncodeToString(sum[:]),
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "卷一"}, {Idx: 1, Title: "卷二"}},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "第一章 起", Content: "卷一开篇正文。", WordCount: 7},
			{Idx: 1, VolumeIdx: 0, Title: "第二章 承", Content: "卷一续篇正文。", WordCount: 7},
			{Idx: 2, VolumeIdx: 0, Title: "第三章 转", Content: "卷一收尾正文。", WordCount: 7},
			{Idx: 3, VolumeIdx: 1, Title: "第四章 结", Content: "卷二正文。", WordCount: 5},
		},
	}
	res, err := e.st.SaveBook(context.Background(), book, store.SaveOptions{})
	if err != nil {
		e.t.Fatalf("SaveBook: %v", err)
	}
	for _, item := range e.bookChapters(res.BookID).Items {
		ids = append(ids, item.ID)
	}
	if len(ids) != 4 {
		e.t.Fatalf("样本书章数 = %d，期望 4", len(ids))
	}
	return res.BookID, ids
}

// volumeIDs 按卷序返回详情里的真实卷 id（伪卷 id=0 不在其中）。
func (e *testEnv) volumeIDs(bookID int64) []int64 {
	e.t.Helper()
	detail := decodeBody[store.BookDetail](e.t, e.get(bookPath(bookID)))
	out := make([]int64, 0, len(detail.Volumes))
	for _, v := range detail.Volumes {
		if v.ID > 0 {
			out = append(out, v.ID)
		}
	}
	return out
}

// TestBatchDeleteChapters 批量删除：204、序号补齐，剩下的章 id 不变。
func TestBatchDeleteChapters(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedVolumeBook("a")
	target := chaptersPath(bookID) + "batch-delete"

	rr := env.postJSON(target, `{"chapterIds":[`+itoa64(ids[2])+`,`+itoa64(ids[3])+`]}`)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	got := env.bookChapters(bookID)
	if got.Total != 2 || got.Items[0].ID != ids[0] || got.Items[1].ID != ids[1] || got.Items[1].Idx != 1 {
		t.Fatalf("删后目录 = %+v", got)
	}
}

// TestBatchDeleteChaptersRejects 空数组 / 重复 / 非正 / 跨书 全部拒绝且不改库。
func TestBatchDeleteChaptersRejects(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedVolumeBook("a")
	otherBook, otherIDs := env.seedVolumeBook("b")
	target := chaptersPath(bookID) + "batch-delete"

	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"空数组", `{"chapterIds":[]}`, http.StatusBadRequest, codeInvalidParam},
		{"缺字段", `{}`, http.StatusBadRequest, codeInvalidParam},
		{"重复 id", `{"chapterIds":[` + itoa64(ids[0]) + `,` + itoa64(ids[0]) + `]}`, http.StatusBadRequest, codeInvalidParam},
		{"非正 id", `{"chapterIds":[0]}`, http.StatusBadRequest, codeInvalidParam},
		{"类型不符", `{"chapterIds":["x"]}`, http.StatusBadRequest, codeInvalidParam},
		{"非法 JSON", `{"chapterIds":`, http.StatusBadRequest, codeInvalidParam},
		{"任一章不属于本书", `{"chapterIds":[` + itoa64(ids[0]) + `,` + itoa64(otherIDs[0]) + `]}`, http.StatusNotFound, codeNotFound},
	}
	for _, tc := range cases {
		rr := env.postJSON(target, tc.body)
		if rr.Code != tc.status {
			t.Errorf("%s：状态码 = %d，期望 %d（body=%s）", tc.name, rr.Code, tc.status, rr.Body.String())
			continue
		}
		if code := errorCodeOf(t, rr); code != tc.code {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, code, tc.code)
		}
	}
	if got := env.bookChapters(bookID); got.Total != 4 {
		t.Errorf("被拒的批量删除改了目录：总章数 = %d，期望 4", got.Total)
	}
	if got := env.bookChapters(otherBook); got.Total != 4 {
		t.Errorf("跨书删除动了另一本书：总章数 = %d，期望 4", got.Total)
	}
}

// TestMergeChapters 合并同卷连续两章：200 + 全书章数，首章 id 保留、正文按目录顺序拼接。
func TestMergeChapters(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedVolumeBook("a")
	target := chaptersPath(bookID) + "merge"

	rr := env.postJSON(target, `{"chapterIds":[`+itoa64(ids[1])+`,`+itoa64(ids[0])+`]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if body := decodeBody[mergeResponse](t, rr); body.Chapters != 3 {
		t.Fatalf("全书章数 = %d，期望 3", body.Chapters)
	}
	got := env.bookChapters(bookID)
	wantIDs := []int64{ids[0], ids[2], ids[3]}
	for i, want := range wantIDs {
		if got.Items[i].ID != want || got.Items[i].Idx != i {
			t.Fatalf("合并后目录第 %d 项 = %+v，期望 id=%d idx=%d", i, got.Items[i], want, i)
		}
	}
	// 首章保留 id（书签/阅读进度按它指过来）；正文顺序跟目录走，不跟请求参数顺序走。
	chapter := decodeBody[store.ChapterContent](t, env.get(chapterURL(bookID, ids[0])))
	if !strings.Contains(chapter.Content, "卷一开篇正文。") || !strings.Contains(chapter.Content, "卷一续篇正文。") {
		t.Fatalf("合并后正文 = %q", chapter.Content)
	}
	if strings.Index(chapter.Content, "开篇") > strings.Index(chapter.Content, "续篇") {
		t.Fatalf("合并后正文顺序反了：%q", chapter.Content)
	}
}

// TestMergeChaptersNotMergeable 跨卷 / 同卷不连续 → 409 not_mergeable 且带原因、不改库。
func TestMergeChaptersNotMergeable(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedVolumeBook("a")
	target := chaptersPath(bookID) + "merge"

	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"跨卷", `{"chapterIds":[` + itoa64(ids[2]) + `,` + itoa64(ids[3]) + `]}`, "跨卷"},
		{"不连续", `{"chapterIds":[` + itoa64(ids[0]) + `,` + itoa64(ids[2]) + `]}`, "不连续"},
	}
	for _, tc := range cases {
		rr := env.postJSON(target, tc.body)
		if rr.Code != http.StatusConflict {
			t.Errorf("%s：状态码 = %d，期望 409（body=%s）", tc.name, rr.Code, rr.Body.String())
			continue
		}
		payload := decodeBody[errorBody](t, rr)
		if payload.Error.Code != codeNotMergeable {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, payload.Error.Code, codeNotMergeable)
		}
		if !strings.Contains(payload.Error.Message, tc.message) {
			t.Errorf("%s：错误消息 = %q，应含 %q", tc.name, payload.Error.Message, tc.message)
		}
	}
	if got := env.bookChapters(bookID); got.Total != 4 {
		t.Errorf("被拒的合并改了目录：总章数 = %d，期望 4", got.Total)
	}
}

// TestMergeChaptersRejects 少于 2 章 / 空数组 400，章节不存在或跨书 404。
func TestMergeChaptersRejects(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedVolumeBook("a")
	_, otherIDs := env.seedVolumeBook("b")
	target := chaptersPath(bookID) + "merge"

	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"只有一章", `{"chapterIds":[` + itoa64(ids[0]) + `]}`, http.StatusBadRequest, codeInvalidParam},
		{"空数组", `{"chapterIds":[]}`, http.StatusBadRequest, codeInvalidParam},
		{"缺字段", `{}`, http.StatusBadRequest, codeInvalidParam},
		{"非法 JSON", `{"chapterIds":[`, http.StatusBadRequest, codeInvalidParam},
		{"章节不存在", `{"chapterIds":[999999,999998]}`, http.StatusNotFound, codeNotFound},
		{"任一章不属于本书", `{"chapterIds":[` + itoa64(ids[0]) + `,` + itoa64(otherIDs[1]) + `]}`, http.StatusNotFound, codeNotFound},
	}
	for _, tc := range cases {
		rr := env.postJSON(target, tc.body)
		if rr.Code != tc.status {
			t.Errorf("%s：状态码 = %d，期望 %d（body=%s）", tc.name, rr.Code, tc.status, rr.Body.String())
			continue
		}
		if code := errorCodeOf(t, rr); code != tc.code {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, code, tc.code)
		}
	}
	if got := env.bookChapters(bookID); got.Total != 4 {
		t.Errorf("被拒的合并改了目录：总章数 = %d，期望 4", got.Total)
	}
}

// TestRenumberChaptersBookScope 全书重编号：200 + 章数，标题按 idx+1 重写。
func TestRenumberChaptersBookScope(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("a")

	rr := env.postJSON(chaptersPath(bookID)+"renumber", `{"scope":"book"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if body := decodeBody[renumberResponse](t, rr); body.Renamed != 4 {
		t.Fatalf("重编号章数 = %d，期望 4", body.Renamed)
	}
	want := []string{"第 1 章 起", "第 2 章 承", "第 3 章 转", "第 4 章 结"}
	got := env.bookChapters(bookID)
	for i, expected := range want {
		if got.Items[i].Title != expected {
			t.Errorf("第 %d 章标题 = %q，期望 %q", i, got.Items[i].Title, expected)
		}
	}
}

// TestRenumberChaptersVolumeScope 单卷重编号：只改该卷标题，章号仍是全书序号。
func TestRenumberChaptersVolumeScope(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("a")
	volumes := env.volumeIDs(bookID)
	if len(volumes) != 2 {
		t.Fatalf("卷数 = %d，期望 2", len(volumes))
	}

	body := `{"scope":"volume","volumeId":` + itoa64(volumes[1]) + `}`
	rr := env.postJSON(chaptersPath(bookID)+"renumber", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeBody[renumberResponse](t, rr); got.Renamed != 1 {
		t.Fatalf("重编号章数 = %d，期望 1", got.Renamed)
	}
	chapters := env.bookChapters(bookID)
	if chapters.Items[0].Title != "第一章 起" {
		t.Errorf("卷外章节被改了：%q", chapters.Items[0].Title)
	}
	// 全书序号：卷二唯一那章是全书第 4 章，不从 1 重来。
	if chapters.Items[3].Title != "第 4 章 结" {
		t.Errorf("卷内章节标题 = %q，期望「第 4 章 结」", chapters.Items[3].Title)
	}
}

// TestRenumberChaptersRejects scope 非法 / 缺 volumeId 400；卷不属本书或不存在 404。
func TestRenumberChaptersRejects(t *testing.T) {
	env := newEnv(t)
	bookID, _ := env.seedVolumeBook("a")
	otherBook, _ := env.seedVolumeBook("b")
	otherVolumes := env.volumeIDs(otherBook)
	target := chaptersPath(bookID) + "renumber"

	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"scope 非法", `{"scope":"all"}`, http.StatusBadRequest, codeInvalidParam},
		{"scope 缺失", `{}`, http.StatusBadRequest, codeInvalidParam},
		{"缺 volumeId", `{"scope":"volume"}`, http.StatusBadRequest, codeInvalidParam},
		{"volumeId 非正", `{"scope":"volume","volumeId":0}`, http.StatusBadRequest, codeInvalidParam},
		{"非法 JSON", `{"scope":`, http.StatusBadRequest, codeInvalidParam},
		{"卷不属本书", `{"scope":"volume","volumeId":` + itoa64(otherVolumes[0]) + `}`, http.StatusNotFound, codeNotFound},
		{"卷不存在", `{"scope":"volume","volumeId":999999}`, http.StatusNotFound, codeNotFound},
	}
	for _, tc := range cases {
		rr := env.postJSON(target, tc.body)
		if rr.Code != tc.status {
			t.Errorf("%s：状态码 = %d，期望 %d（body=%s）", tc.name, rr.Code, tc.status, rr.Body.String())
			continue
		}
		if code := errorCodeOf(t, rr); code != tc.code {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, code, tc.code)
		}
	}
	if got := env.bookChapters(bookID); got.Items[0].Title != "第一章 起" {
		t.Errorf("被拒的重编号改了标题：%q", got.Items[0].Title)
	}
}
