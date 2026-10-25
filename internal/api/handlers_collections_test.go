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

// sendCollection 发起一次带 JSON 请求体的书单接口调用。
func sendCollection(t *testing.T, e *testEnv, method, target, payload string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(method, target, strings.NewReader(payload),
		map[string]string{"Content-Type": "application/json"})
}

// createCollectionViaAPI 建一个书单并返回（测试前置用）。
func createCollectionViaAPI(t *testing.T, e *testEnv, name string) store.Collection {
	t.Helper()
	rr := sendCollection(t, e, http.MethodPost, "/api/collections", fmt.Sprintf(`{"name":%q}`, name))
	requireStatus(t, rr, http.StatusCreated)
	return decodeBody[store.Collection](t, rr)
}

// collectionBookCount 直接用 store 查书单里的书目数（书目筛选端点尚未实现）。
func collectionBookCount(t *testing.T, e *testEnv, collectionID int64) int {
	t.Helper()
	page, err := e.st.ListBooks(context.Background(),
		store.BookQuery{Collection: collectionID, Page: 1, Size: 50})
	if err != nil {
		t.Fatalf("ListBooks(collection): %v", err)
	}
	return page.Total
}

func TestCreateCollectionAndList(t *testing.T) {
	e := newEnv(t)
	created := createCollectionViaAPI(t, e, " 通宵书单 ")
	if created.ID <= 0 || created.Name != "通宵书单" || created.BookCount != 0 {
		t.Fatalf("新书单 = %+v", created)
	}
	rr := e.get("/api/collections")
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[collectionsResponse](t, rr)
	if len(body.Items) != 1 || body.Items[0].ID != created.ID {
		t.Fatalf("书单列表 = %+v", body.Items)
	}
}

func TestCreateCollectionDuplicate(t *testing.T) {
	e := newEnv(t)
	createCollectionViaAPI(t, e, "通宵书单")
	requireError(t, sendCollection(t, e, http.MethodPost, "/api/collections", `{"name":"通宵书单"}`),
		http.StatusConflict, codeConflict)
}

func TestCreateCollectionValidation(t *testing.T) {
	e := newEnv(t)
	requireError(t, sendCollection(t, e, http.MethodPost, "/api/collections", `{"name":"   "}`),
		http.StatusBadRequest, codeInvalidParam)
	longName := strings.Repeat("书", 61)
	requireError(t, sendCollection(t, e, http.MethodPost, "/api/collections",
		fmt.Sprintf(`{"name":%q}`, longName)), http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendCollection(t, e, http.MethodPost, "/api/collections", `not json`),
		http.StatusBadRequest, codeInvalidParam)
}

func TestRenameCollection(t *testing.T) {
	e := newEnv(t)
	created := createCollectionViaAPI(t, e, "旧书单")
	rr := sendCollection(t, e, http.MethodPatch,
		fmt.Sprintf("/api/collections/%d", created.ID), `{"name":"新书单"}`)
	requireStatus(t, rr, http.StatusOK)
	got := decodeBody[store.Collection](t, rr)
	if got.ID != created.ID || got.Name != "新书单" {
		t.Fatalf("改名结果 = %+v", got)
	}
}

func TestRenameCollectionConflicts(t *testing.T) {
	e := newEnv(t)
	created := createCollectionViaAPI(t, e, "书单一")
	createCollectionViaAPI(t, e, "书单二")
	requireError(t, sendCollection(t, e, http.MethodPatch,
		fmt.Sprintf("/api/collections/%d", created.ID), `{"name":"书单二"}`),
		http.StatusConflict, codeConflict)
	requireError(t, sendCollection(t, e, http.MethodPatch, "/api/collections/999999", `{"name":"新名"}`),
		http.StatusNotFound, codeNotFound)
	requireError(t, sendCollection(t, e, http.MethodPatch, "/api/collections/abc", `{"name":"新名"}`),
		http.StatusBadRequest, codeInvalidID)
}

func TestDeleteCollection(t *testing.T) {
	e := newEnv(t)
	created := createCollectionViaAPI(t, e, "待删书单")
	bookID := e.seedBook("测试之书", "测试作者")
	rr := sendCollection(t, e, http.MethodPut,
		fmt.Sprintf("/api/collections/%d/books", created.ID), fmt.Sprintf(`{"bookIds":[%d]}`, bookID))
	requireStatus(t, rr, http.StatusNoContent)

	requireStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/api/collections/%d", created.ID), nil, nil),
		http.StatusNoContent)
	body := decodeBody[collectionsResponse](t, e.get("/api/collections"))
	if len(body.Items) != 0 {
		t.Fatalf("删后书单列表 = %+v，期望空", body.Items)
	}
	requireStatus(t, e.get(fmt.Sprintf("/api/books/%d", bookID)), http.StatusOK) // 书本身不动
}

func TestReplaceCollectionBooks(t *testing.T) {
	e := newEnv(t)
	created := createCollectionViaAPI(t, e, "收藏夹")
	first := e.seedBook("书甲", "作者甲")
	second := e.seedBook("书乙", "作者乙")
	target := fmt.Sprintf("/api/collections/%d/books", created.ID)

	requireStatus(t, sendCollection(t, e, http.MethodPut, target,
		fmt.Sprintf(`{"bookIds":[%d,%d,%d]}`, first, second, first)), http.StatusNoContent)
	if got := collectionBookCount(t, e, created.ID); got != 2 {
		t.Fatalf("全量替换后书数 = %d，期望 2（去重）", got)
	}
	requireStatus(t, sendCollection(t, e, http.MethodPut, target,
		fmt.Sprintf(`{"bookIds":[%d]}`, second)), http.StatusNoContent)
	if got := collectionBookCount(t, e, created.ID); got != 1 {
		t.Fatalf("替换为单本后书数 = %d，期望 1", got)
	}
	requireStatus(t, sendCollection(t, e, http.MethodPut, target, `{"bookIds":[]}`),
		http.StatusNoContent)
	if got := collectionBookCount(t, e, created.ID); got != 0 {
		t.Fatalf("清空后书数 = %d，期望 0", got)
	}
}

func TestReplaceCollectionBooksValidation(t *testing.T) {
	e := newEnv(t)
	created := createCollectionViaAPI(t, e, "收藏夹")
	target := fmt.Sprintf("/api/collections/%d/books", created.ID)

	requireError(t, sendCollection(t, e, http.MethodPut, target, `{}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendCollection(t, e, http.MethodPut, target, `{"bookIds":[-1]}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendCollection(t, e, http.MethodPut, target, `{"bookIds":[999999]}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendCollection(t, e, http.MethodPut, "/api/collections/999999/books", `{"bookIds":[]}`),
		http.StatusNotFound, codeNotFound)
}
