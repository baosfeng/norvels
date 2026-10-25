package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sendBooks 发起一次带 JSON 请求体的书籍接口调用。
func sendBooks(t *testing.T, e *testEnv, method, target, payload string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(method, target, strings.NewReader(payload),
		map[string]string{"Content-Type": "application/json"})
}

// batchPayload 拼批量请求体。
func batchPayload(ids []int64, patch string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return fmt.Sprintf(`{"ids":[%s],"patch":%s}`, strings.Join(parts, ","), patch)
}

func TestBatchPatchBooks(t *testing.T) {
	e := newEnv(t)
	first := e.seedBook("批量甲", "作者甲")
	second := e.seedBook("批量乙", "作者乙")
	payload := fmt.Sprintf(`{"ids":[%d,%d],"patch":{"series":"批量系列","rating":3}}`, first, second)

	rr := sendBooks(t, e, http.MethodPost, "/api/books/batch", payload)
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[batchPatchResponse](t, rr)
	if len(body.Updated) != 2 || len(body.Failed) != 0 {
		t.Fatalf("批量结果 = %+v", body)
	}
	for _, id := range []int64{first, second} {
		got := decodeBody[bookDetailResp](t, e.get(bookPath(id)))
		if got.Rating != 3 || got.Series != "批量系列" {
			t.Fatalf("书 %d 批量后 = %+v", id, got)
		}
	}
}

func TestBatchPatchBooksPartialFailure(t *testing.T) {
	e := newEnv(t)
	first := e.seedBook("批量丙", "作者丙")
	payload := batchPayload([]int64{first, 999999}, `{"rating":2}`)

	rr := sendBooks(t, e, http.MethodPost, "/api/books/batch", payload)
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[batchPatchResponse](t, rr)
	if len(body.Updated) != 1 || body.Updated[0] != first {
		t.Fatalf("updated = %v", body.Updated)
	}
	if len(body.Failed) != 1 || body.Failed[0].ID != 999999 || body.Failed[0].Message == "" {
		t.Fatalf("failed = %+v", body.Failed)
	}
}

func TestBatchPatchBooksValidation(t *testing.T) {
	e := newEnv(t)
	target := "/api/books/batch"
	requireError(t, sendBooks(t, e, http.MethodPost, target, `{"ids":[],"patch":{"rating":1}}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target, `{"ids":[0],"patch":{"rating":1}}`),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target, batchPayload([]int64{1}, `{}`)),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target, batchPayload([]int64{1}, `{"rating":9}`)),
		http.StatusBadRequest, codeInvalidParam)
	requireError(t, sendBooks(t, e, http.MethodPost, target, `not json`),
		http.StatusBadRequest, codeInvalidParam)
}

func TestBatchPatchBooksTooMany(t *testing.T) {
	e := newEnv(t)
	ids := make([]int64, maxBatchBooks+1)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	requireError(t, sendBooks(t, e, http.MethodPost, "/api/books/batch", batchPayload(ids, `{"rating":1}`)),
		http.StatusBadRequest, codeInvalidParam)
}
