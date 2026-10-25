package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bookmarkBody 是书签响应的测试侧结构（字段与 docs/api.md 一致）。
type bookmarkBody struct {
	ID           int64  `json:"id"`
	ChapterID    int64  `json:"chapterId"`
	ChapterIdx   int    `json:"chapterIdx"`
	ChapterTitle string `json:"chapterTitle"`
	ParaIndex    int    `json:"paraIndex"`
	Label        string `json:"label"`
	CreatedAt    string `json:"createdAt"`
}

// bookmarkListBody 是 GET /api/books/{id}/bookmarks 的响应。
type bookmarkListBody struct {
	Bookmarks []bookmarkBody `json:"bookmarks"`
}

// postBookmark 发一次 POST /api/books/{id}/bookmarks。
func (e *testEnv) postBookmark(bookID int64, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPost, fmt.Sprintf("/api/books/%d/bookmarks", bookID), strings.NewReader(body), nil)
}

// TestBookmarkLifecycle 覆盖 201 → 幂等 200 → 列表 → DELETE 204 → 404 全链路。
func TestBookmarkLifecycle(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("书签测试书", "书签作者")
	chapterID := env.firstChapterID(bookID)
	body := fmt.Sprintf(`{"chapterId":%d,"paraIndex":2,"label":"第一处"}`, chapterID)

	first := requireBookmarkCreated(t, env, bookID, body)
	if first.ParaIndex != 2 || first.Label != "第一处" {
		t.Fatalf("书签字段不符: %+v", first)
	}
	second := requireBookmarkReplay(t, env, bookID, strings.Replace(body, "第一处", "改过的", 1))
	if second.ID != first.ID {
		t.Errorf("幂等应复用同一行：first=%d second=%d", first.ID, second.ID)
	}
	requireBookmarkListLen(t, env, bookID, 1, "改过的")
	requireBookmarkDelete(t, env, first.ID, http.StatusNoContent)
	requireBookmarkDelete(t, env, first.ID, http.StatusNotFound)
	requireBookmarkListLen(t, env, bookID, 0, "")
}

// requireBookmarkCreated 断言首次添加返回 201 并解出书签。
func requireBookmarkCreated(t *testing.T, env *testEnv, bookID int64, body string) bookmarkBody {
	t.Helper()
	rr := env.postBookmark(bookID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("首次 POST 状态码 = %d，期望 201，body=%s", rr.Code, rr.Body.String())
	}
	bm := decodeBody[bookmarkBody](t, rr)
	if bm.ID <= 0 || bm.ChapterID <= 0 || bm.ChapterTitle == "" {
		t.Fatalf("书签字段不完整: %+v", bm)
	}
	return bm
}

// requireBookmarkReplay 断言同位置重复添加返回 200（幂等）。
func requireBookmarkReplay(t *testing.T, env *testEnv, bookID int64, body string) bookmarkBody {
	t.Helper()
	rr := env.postBookmark(bookID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("重复 POST 状态码 = %d，期望 200（幂等），body=%s", rr.Code, rr.Body.String())
	}
	return decodeBody[bookmarkBody](t, rr)
}

// requireBookmarkListLen 断言书签条数（wantLabel 非空时再断言首条 label）。
func requireBookmarkListLen(t *testing.T, env *testEnv, bookID int64, want int, wantLabel string) {
	t.Helper()
	list := decodeBody[bookmarkListBody](t, env.get(fmt.Sprintf("/api/books/%d/bookmarks", bookID)))
	if len(list.Bookmarks) != want {
		t.Fatalf("书签条数 = %d，期望 %d", len(list.Bookmarks), want)
	}
	if wantLabel != "" && list.Bookmarks[0].Label != wantLabel {
		t.Errorf("首条 label = %q，期望 %q", list.Bookmarks[0].Label, wantLabel)
	}
}

// requireBookmarkDelete 删除书签并断言状态码（204 或 404）。
func requireBookmarkDelete(t *testing.T, env *testEnv, id int64, want int) {
	t.Helper()
	rr := env.do(http.MethodDelete, fmt.Sprintf("/api/bookmarks/%d", id), nil, nil)
	if want == http.StatusNoContent {
		if rr.Code != want {
			t.Fatalf("DELETE 状态码 = %d，期望 %d", rr.Code, want)
		}
		return
	}
	requireError(t, rr, want, codeNotFound)
}

// TestBookmarkValidation 覆盖跨边界入参：缺字段、负数、不属于本书的章节、书不存在。
func TestBookmarkValidation(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("书签校验书", "书签作者")
	chapterID := env.firstChapterID(bookID)
	otherID := env.seedBook("另一本书", "另一位作者")
	otherChapterID := env.firstChapterID(otherID)

	badBodies := []string{
		`{}`,
		`{"paraIndex":0}`,
		`{"chapterId":0,"paraIndex":0}`,
		fmt.Sprintf(`{"chapterId":%d}`, chapterID),
		fmt.Sprintf(`{"chapterId":%d,"paraIndex":-1}`, chapterID),
		fmt.Sprintf(`{"chapterId":%d,"paraIndex":0}`, otherChapterID),
	}
	for _, body := range badBodies {
		t.Run("body="+body, func(t *testing.T) {
			requireError(t, env.postBookmark(bookID, body), http.StatusBadRequest, codeInvalidParam)
		})
	}

	requireError(t, env.postBookmark(99999, fmt.Sprintf(`{"chapterId":%d,"paraIndex":0}`, chapterID)),
		http.StatusNotFound, codeNotFound)
	requireError(t, env.get("/api/books/99999/bookmarks"), http.StatusNotFound, codeNotFound)
	requireError(t, env.get("/api/books/abc/bookmarks"), http.StatusBadRequest, codeInvalidID)
}

// TestBookReadStatusPatchAndFilter 覆盖 PATCH readStatus、详情回显与书架筛选。
func TestBookReadStatusPatchAndFilter(t *testing.T) {
	env := newEnv(t)
	readingID := env.seedBook("在读测试书", "作者甲")
	doneID := env.seedBook("已读测试书", "作者乙")

	patch := func(id int64, body string) *httptest.ResponseRecorder {
		return env.do(http.MethodPatch, fmt.Sprintf("/api/books/%d", id), strings.NewReader(body), nil)
	}
	for _, tc := range []struct {
		id   int64
		body string
		want string
	}{
		{readingID, `{"readStatus":"reading"}`, "reading"},
		{doneID, `{"readStatus":"done"}`, "done"},
	} {
		rr := patch(tc.id, tc.body)
		if rr.Code != http.StatusOK {
			t.Fatalf("PATCH %s 状态码 = %d，body=%s", tc.body, rr.Code, rr.Body.String())
		}
		if got := decodeBody[bookDetailResp](t, rr).ReadStatus; got != tc.want {
			t.Errorf("PATCH 回显 readStatus = %q，期望 %q", got, tc.want)
		}
	}
	requireError(t, patch(readingID, `{"readStatus":"abc"}`), http.StatusBadRequest, codeInvalidParam)
	if rr := env.get("/api/books?readStatus=done"); rr.Code != http.StatusOK {
		t.Fatalf("筛选状态码 = %d", rr.Code)
	} else {
		list := decodeBody[bookListProbe](t, rr)
		if list.Total != 1 || list.Items[0].ID != doneID {
			t.Errorf("按 done 筛选 = %+v，期望只有 %d", list, doneID)
		}
	}
	requireError(t, env.get("/api/books?readStatus=abc"), http.StatusBadRequest, codeInvalidParam)
	if list := decodeBody[bookListProbe](t, env.get("/api/books?readStatus=want")); list.Total != 0 {
		t.Errorf("按 want 筛选 = %+v，期望空", list)
	}
}

// bookListProbe 只取书架列表的 id/readStatus/total。
type bookListProbe struct {
	Total int `json:"total"`
	Items []struct {
		ID         int64  `json:"id"`
		ReadStatus string `json:"readStatus"`
	} `json:"items"`
}

// TestProgressAutoMarksReadingAPI 覆盖「写进度自动转在读」与「不覆盖用户标记」。
func TestProgressAutoMarksReadingAPI(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("进度自动书", "作者丙")
	chapterID := env.firstChapterID(bookID)
	putProgress := func() *httptest.ResponseRecorder {
		return env.do(http.MethodPut, "/api/reading/progress",
			strings.NewReader(fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"percent":0.3}`, bookID, chapterID)), nil)
	}
	if rr := putProgress(); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 进度状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	detail := decodeBody[bookDetailResp](t, env.get(fmt.Sprintf("/api/books/%d", bookID)))
	if detail.ReadStatus != "reading" {
		t.Fatalf("写进度后 readStatus = %q，期望 reading", detail.ReadStatus)
	}
	rr := env.do(http.MethodPatch, fmt.Sprintf("/api/books/%d", bookID),
		strings.NewReader(`{"readStatus":"want"}`), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH want 状态码 = %d", rr.Code)
	}
	if rr := putProgress(); rr.Code != http.StatusNoContent {
		t.Fatalf("再次 PUT 进度状态码 = %d", rr.Code)
	}
	if got := decodeBody[bookDetailResp](t, env.get(fmt.Sprintf("/api/books/%d", bookID))).ReadStatus; got != "want" {
		t.Errorf("用户标记 want 被进度覆盖成 %q", got)
	}
}

// TestDeleteBookCascadeBookmarks 删书后详情与书签一起消失。
func TestDeleteBookCascadeBookmarks(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("级联书签书", "作者丁")
	chapterID := env.firstChapterID(bookID)
	rr := env.postBookmark(bookID, fmt.Sprintf(`{"chapterId":%d,"paraIndex":1}`, chapterID))
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST 书签状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	detail := decodeBody[bookDetailResp](t, env.get(fmt.Sprintf("/api/books/%d", bookID)))
	if detail.BookmarkCount != 1 {
		t.Fatalf("详情 bookmarkCount = %d，期望 1", detail.BookmarkCount)
	}
	if rr := env.do(http.MethodDelete, fmt.Sprintf("/api/books/%d", bookID), nil, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE 书状态码 = %d", rr.Code)
	}
	requireError(t, env.get(fmt.Sprintf("/api/books/%d", bookID)), http.StatusNotFound, codeNotFound)
	requireError(t, env.get(fmt.Sprintf("/api/books/%d/bookmarks", bookID)), http.StatusNotFound, codeNotFound)
}
