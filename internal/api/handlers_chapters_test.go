package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 章节结构编辑（管理页 M1）的接口口径：改标题 / 拆分 / 删除。

// chapterURL 拼接「某本书的某一章」路径（corrupt_block_test.go 里已有一个取第一章的 chapterPath）。
func chapterURL(bookID, chapterID int64) string {
	return "/api/books/" + strconv.FormatInt(bookID, 10) + "/chapters/" + strconv.FormatInt(chapterID, 10)
}

// seedParagraphBook 写一本「一章四段」的书，专供拆分用（seedBook 的章都是单段）。
// 第二个章是两段，用来验证「拆中间那章」时后面章节跟着移位。
func (e *testEnv) seedParagraphBook() (bookID int64, ids []int64) {
	e.t.Helper()
	sum := sha256.Sum256([]byte("norvels-test:paragraph-book"))
	book := &model.ParsedBook{
		Title:      "分段样本",
		Author:     "样本作者",
		SourcePath: filepath.Join(e.t.TempDir(), "paragraph.txt"),
		SourceName: "paragraph.txt",
		Encoding:   "utf-8",
		FileSize:   4096,
		SHA256:     hex.EncodeToString(sum[:]),
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "第一卷"}},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "长章", Content: "第一段落。\n第二段落。\n第三段落。\n第四段落。", WordCount: 16},
			{Idx: 1, VolumeIdx: 0, Title: "收尾章", Content: "收尾第一段。\n收尾第二段。", WordCount: 12},
		},
	}
	res, err := e.st.SaveBook(context.Background(), book, store.SaveOptions{})
	if err != nil {
		e.t.Fatalf("SaveBook: %v", err)
	}
	list := e.bookChapters(res.BookID)
	for _, item := range list.Items {
		ids = append(ids, item.ID)
	}
	return res.BookID, ids
}

// TestPatchChapterTitle 改标题：204 无响应体，详情立即反映；非法输入按码返回。
func TestPatchChapterTitle(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedParagraphBook()
	target := chapterURL(bookID, ids[0])

	rr := env.patch(target, `{"title":"  改过的标题  "}`)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if got := env.bookChapters(bookID); got.Items[0].Title != "改过的标题" {
		t.Fatalf("目录标题 = %q，期望「改过的标题」", got.Items[0].Title)
	}

	cases := []struct {
		name   string
		target string
		body   string
		status int
		code   string
	}{
		{"空对象", target, "{}", http.StatusBadRequest, codeInvalidParam},
		{"空白标题", target, `{"title":"   "}`, http.StatusBadRequest, codeInvalidParam},
		{"非法 JSON", target, `{"title":`, http.StatusBadRequest, codeInvalidParam},
		{"章节不存在", chapterURL(bookID, 99999), `{"title":"标题"}`, http.StatusNotFound, codeNotFound},
		{"书籍不存在", chapterURL(99999, ids[0]), `{"title":"标题"}`, http.StatusNotFound, codeNotFound},
		{"章节 id 非法", "/api/books/" + strconv.FormatInt(bookID, 10) + "/chapters/abc", `{"title":"标题"}`, http.StatusBadRequest, codeInvalidID},
	}
	for _, tc := range cases {
		got := env.patch(tc.target, tc.body)
		if got.Code != tc.status {
			t.Errorf("%s：状态码 = %d，期望 %d（body=%s）", tc.name, got.Code, tc.status, got.Body.String())
			continue
		}
		if code := errorCodeOf(t, got); code != tc.code {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, code, tc.code)
		}
	}
}

// TestSplitChapterEndpoint 拆分：200 + 章数，目录与正文都跟着变。
func TestSplitChapterEndpoint(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedParagraphBook()
	target := chapterURL(bookID, ids[0]) + "/split"

	rr := env.postJSON(target, `{"cutAfter":[1]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if body := decodeBody[splitResponse](t, rr); body.Chapters != 2 {
		t.Fatalf("返回章数 = %d，期望 2", body.Chapters)
	}
	got := env.bookChapters(bookID)
	if got.Total != 3 {
		t.Fatalf("目录总章数 = %d，期望 3", got.Total)
	}
	wantTitles := []string{"长章（1/2）", "长章（2/2）", "收尾章"}
	for i, want := range wantTitles {
		if got.Items[i].Title != want {
			t.Errorf("第 %d 章 = %q，期望 %q", i, got.Items[i].Title, want)
		}
	}
	// 收尾章的 id 不变（书签 / 阅读进度都按章节 id 指过来）
	if got.Items[2].ID != ids[1] {
		t.Errorf("收尾章 id = %d，期望 %d", got.Items[2].ID, ids[1])
	}
}

// TestSplitChapterEndpointValidation 拆分点非法时 400 且带上原因，不改库。
func TestSplitChapterEndpointValidation(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedParagraphBook()
	target := chapterURL(bookID, ids[0]) + "/split"

	cases := []struct {
		name    string
		body    string
		status  int
		code    string
		message string
	}{
		{"没有拆分点", `{"cutAfter":[]}`, http.StatusBadRequest, codeInvalidParam, "拆分点"},
		{"越界", `{"cutAfter":[9]}`, http.StatusBadRequest, codeInvalidParam, "超出"},
		{"乱序", `{"cutAfter":[2,1]}`, http.StatusBadRequest, codeInvalidParam, "递增"},
		{"非法 JSON", `{"cutAfter":`, http.StatusBadRequest, codeInvalidParam, ""},
	}
	for _, tc := range cases {
		rr := env.postJSON(target, tc.body)
		if rr.Code != tc.status {
			t.Errorf("%s：状态码 = %d，期望 %d", tc.name, rr.Code, tc.status)
			continue
		}
		payload := decodeBody[errorBody](t, rr)
		if payload.Error.Code != tc.code {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, payload.Error.Code, tc.code)
		}
		if tc.message != "" && !strings.Contains(payload.Error.Message, tc.message) {
			t.Errorf("%s：错误消息 = %q，应含 %q", tc.name, payload.Error.Message, tc.message)
		}
	}
	if got := env.bookChapters(bookID); got.Total != 2 {
		t.Errorf("被拒的拆分改了目录：总章数 = %d，期望 2", got.Total)
	}
}

// TestDeleteChapterEndpoint 删除：204，序号补齐，重复删 404。
func TestDeleteChapterEndpoint(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedParagraphBook()
	target := chapterURL(bookID, ids[0])

	rr := env.do(http.MethodDelete, target, nil, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	got := env.bookChapters(bookID)
	if got.Total != 1 || got.Items[0].Title != "收尾章" || got.Items[0].Idx != 0 {
		t.Fatalf("删后目录 = %+v", got)
	}
	again := env.do(http.MethodDelete, target, nil, nil)
	if again.Code != http.StatusNotFound {
		t.Errorf("重复删除状态码 = %d，期望 404", again.Code)
	}
	if body := decodeBody[bookDetailResp](t, env.get(bookPath(bookID))); body.Status == "" {
		t.Errorf("详情读取失败：%+v", body)
	}
}

// errorCodeOf 取错误响应里的 code（响应不是标准错误体时 fail）。
func errorCodeOf(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if rr.Code >= 200 && rr.Code < 300 {
		return ""
	}
	return decodeBody[errorBody](t, rr).Error.Code
}

// postJSON 发起一次带 JSON 体的 POST。
func (e *testEnv) postJSON(target, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPost, target, strings.NewReader(body), map[string]string{"Content-Type": "application/json"})
}
