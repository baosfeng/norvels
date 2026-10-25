package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/store"
)

// sendTag 发起一次带 JSON 请求体的标签接口调用。
func sendTag(t *testing.T, e *testEnv, method, target, payload string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(method, target, strings.NewReader(payload),
		map[string]string{"Content-Type": "application/json"})
}

// createTagViaAPI 建一个标签并返回（同名已存在时返回已有的那行）。
func createTagViaAPI(t *testing.T, e *testEnv, name string) store.TagSummary {
	t.Helper()
	rr := sendTag(t, e, http.MethodPost, "/api/tags", fmt.Sprintf(`{"name":%q}`, name))
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("建标签状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	return decodeBody[store.TagSummary](t, rr)
}

// bindTags 把标签挂到书上（走既有 PATCH 元数据路径，与前端同一条链路）。
func bindTags(t *testing.T, e *testEnv, bookID int64, names []string) {
	t.Helper()
	if err := e.st.UpdateBookMeta(context.Background(), bookID,
		store.BookMetaPatch{Tags: &names}); err != nil {
		t.Fatalf("UpdateBookMeta tags: %v", err)
	}
}

func TestListTagsWithBookCount(t *testing.T) {
	e := newEnv(t)
	id := e.seedBook("测试之书", "测试作者")
	bindTags(t, e, id, []string{"玄幻", "长篇"})

	rr := e.get("/api/tags")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[tagsResponse](t, rr)
	if len(body.Items) != 2 {
		t.Fatalf("标签数 = %d，期望 2：%+v", len(body.Items), body.Items)
	}
	for _, it := range body.Items {
		if it.ID <= 0 || it.BookCount != 1 {
			t.Fatalf("标签行 = %+v，期望 bookCount=1", it)
		}
	}
}

func TestCreateTagIdempotent(t *testing.T) {
	e := newEnv(t)
	first := decodeBody[store.TagSummary](t, sendTag(t, e, http.MethodPost, "/api/tags", `{"name":"玄幻"}`))
	if first.ID <= 0 || first.Name != "玄幻" {
		t.Fatalf("首次新建 = %+v", first)
	}
	if rr := sendTag(t, e, http.MethodPost, "/api/tags", `{"name":" 玄幻 "}`); rr.Code != http.StatusOK {
		t.Fatalf("同名再建状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	second := decodeBody[store.TagSummary](t, sendTag(t, e, http.MethodPost, "/api/tags", `{"name":"玄幻"}`))
	if second.ID != first.ID {
		t.Fatalf("同名再建返回了新行：%d != %d", second.ID, first.ID)
	}
}

func TestCreateTagValidation(t *testing.T) {
	e := newEnv(t)
	requireError(t, sendTag(t, e, http.MethodPost, "/api/tags", `{"name":"  "}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendTag(t, e, http.MethodPost, "/api/tags", `{"name":"`+strings.Repeat("长", 65)+`"}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendTag(t, e, http.MethodPost, "/api/tags", `not json`),
		http.StatusBadRequest, codeInvalidParam)
}

func TestRenameTag(t *testing.T) {
	e := newEnv(t)
	first := createTagViaAPI(t, e, "玄幻")
	createTagViaAPI(t, e, "科幻")

	rr := sendTag(t, e, http.MethodPatch, fmt.Sprintf("/api/tags/%d", first.ID), `{"name":"奇幻"}`)
	requireStatus(t, rr, http.StatusOK)
	got := decodeBody[store.TagSummary](t, rr)
	if got.ID != first.ID || got.Name != "奇幻" {
		t.Fatalf("改名结果 = %+v", got)
	}
}

func TestRenameTagConflicts(t *testing.T) {
	e := newEnv(t)
	first := createTagViaAPI(t, e, "玄幻")
	createTagViaAPI(t, e, "科幻")
	requireError(t, sendTag(t, e, http.MethodPatch, fmt.Sprintf("/api/tags/%d", first.ID), `{"name":"科幻"}`),
		http.StatusConflict, codeConflict)
	requireError(t, sendTag(t, e, http.MethodPatch, "/api/tags/999999", `{"name":"新名"}`),
		http.StatusNotFound, codeNotFound)
	requireError(t, sendTag(t, e, http.MethodPatch, "/api/tags/abc", `{"name":"新名"}`),
		http.StatusBadRequest, codeInvalidID)
}

func TestDeleteTag(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("测试之书", "测试作者")
	bindTags(t, e, bookID, []string{"玄幻"})
	tag := createTagViaAPI(t, e, "玄幻")

	rr := e.do(http.MethodDelete, fmt.Sprintf("/api/tags/%d", tag.ID), nil, nil)
	requireStatus(t, rr, http.StatusNoContent)

	body := decodeBody[tagsResponse](t, e.get("/api/tags"))
	if len(body.Items) != 0 {
		t.Fatalf("删后标签列表 = %+v，期望空", body.Items)
	}
	detail := decodeBody[store.BookDetail](t, e.get(fmt.Sprintf("/api/books/%d", bookID)))
	if len(detail.Tags) != 0 {
		t.Fatalf("删后书的标签 = %+v，期望全库解绑", detail.Tags)
	}
	requireError(t, e.do(http.MethodDelete, fmt.Sprintf("/api/tags/%d", tag.ID), nil, nil),
		http.StatusNotFound, codeNotFound)
}
