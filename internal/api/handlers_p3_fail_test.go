package api

// P3 新端点的失败路径与纯校验助手：覆盖 500 / 400 分支（正常路径由各端点的 _test 覆盖）。

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"norvels/internal/store"
)

// p3JSONHeader 是 JSON 请求的 Content-Type。
var p3JSONHeader = map[string]string{"Content-Type": "application/json"}

// expectInternal 断言「store 不可用」时端点返回 500 internal（而不是 panic 或 404）。
func expectInternal(t *testing.T, e *testEnv, method, target, body string) {
	t.Helper()
	rr := e.do(method, target, strings.NewReader(body), p3JSONHeader)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("%s %s 状态码 = %d，期望 500，body=%s", method, target, rr.Code, rr.Body.String())
	}
}

func TestP3ReadsStoreUnavailable(t *testing.T) {
	e := newEnv(t)
	if err := e.st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, target := range []string{
		"/api/series", "/api/series/x", "/api/collections", "/api/tags",
		"/api/books", "/api/books/random", "/api/reading/history", "/api/stats/reading",
	} {
		rr := e.get(target)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s 状态码 = %d，期望 500，body=%s", target, rr.Code, rr.Body.String())
		}
	}
}

func TestP3WritesStoreUnavailable(t *testing.T) {
	e := newEnv(t)
	if err := e.st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cases := []struct{ method, target, body string }{
		{http.MethodPost, "/api/tags", `{"name":"x"}`},
		{http.MethodPatch, "/api/tags/1", `{"name":"y"}`},
		{http.MethodDelete, "/api/tags/1", ""},
		{http.MethodPost, "/api/collections", `{"name":"c"}`},
		{http.MethodPatch, "/api/collections/1", `{"name":"d"}`},
		{http.MethodDelete, "/api/collections/1", ""},
		{http.MethodPut, "/api/collections/1/books", `{"bookIds":[]}`},
		{http.MethodPost, "/api/books/batch", `{"ids":[1],"patch":{"rating":1}}`},
		{http.MethodPost, "/api/books/1/ack-new-chapters", ""},
		{http.MethodPost, "/api/reading/sessions", `{"bookId":1,"chapterId":1,"seconds":60}`},
		{http.MethodPost, "/api/books/1/chapters", `{"title":"t","content":"c"}`},
		{http.MethodPut, "/api/reading/progress", `{"bookId":1,"chapterId":1,"percent":0.1}`},
	}
	for _, c := range cases {
		expectInternal(t, e, c.method, c.target, c.body)
	}
}

func TestSeriesPathName(t *testing.T) {
	got, err := seriesPathName(url.PathEscape("测试系列"))
	if err != nil || got != "测试系列" {
		t.Fatalf("正常名字 = %q, %v", got, err)
	}
	if _, err := seriesPathName("%zz"); err == nil {
		t.Fatal("非法百分号转义应当报错")
	}
	if _, err := seriesPathName("a/b"); err == nil {
		t.Fatal("含路径分隔符的名字应当报错")
	}
	if _, err := seriesPathName("   "); err == nil {
		t.Fatal("空白名字应当报错")
	}
}

func TestRenameTagBadBody(t *testing.T) {
	e := newEnv(t)
	requireError(t, sendTag(t, e, http.MethodPatch, "/api/tags/1", `not json`),
		http.StatusBadRequest, codeInvalidParam)
}

func TestFailReadingWriteInvalidArgument(t *testing.T) {
	e := newEnv(t)
	rr := httptest.NewRecorder()
	e.srv.failReadingWrite(rr, httptest.NewRequest(http.MethodPost, "/api/reading/sessions", nil),
		fmt.Errorf("%w: seconds 0 越界", store.ErrInvalidArgument))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("参数错误状态码 = %d，期望 400，body=%s", rr.Code, rr.Body.String())
	}
}

func TestNewBatchPatchResponseShape(t *testing.T) {
	failed := map[int64]error{
		2: fmt.Errorf("%w: book 2", store.ErrNotFound),
		1: fmt.Errorf("%w: rating 9", store.ErrInvalidArgument),
	}
	got := newBatchPatchResponse(nil, failed)
	if got.Updated == nil || len(got.Updated) != 0 {
		t.Fatalf("updated 应为 []，实际 %+v", got.Updated)
	}
	if len(got.Failed) != 2 || got.Failed[0].ID != 1 || got.Failed[1].ID != 2 {
		t.Fatalf("failed 应按 id 升序，实际 %+v", got.Failed)
	}
}

func TestBatchMessageMapping(t *testing.T) {
	invalid := fmt.Errorf("%w: rating 9 必须在 0~5 之间", store.ErrInvalidArgument)
	if got := batchMessage(invalid); !strings.Contains(got, "rating 9") {
		t.Fatalf("参数类错误消息 = %q", got)
	}
	if got := batchMessage(fmt.Errorf("%w: book 9", store.ErrNotFound)); got != "书籍不存在" {
		t.Fatalf("未找到消息 = %q", got)
	}
	if got := batchMessage(errors.New("db down")); got != "更新失败" {
		t.Fatalf("兜底消息 = %q", got)
	}
}
