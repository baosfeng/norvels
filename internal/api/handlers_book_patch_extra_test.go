package api

import (
	"net/http"
	"testing"
)

// TestPatchBookPublishingFields 覆盖 PATCH 新增的评分与出版信息五字段。
func TestPatchBookPublishingFields(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("出版信息示例书", "示例作者")

	rr := env.patch(bookPath(id),
		`{"rating":4,"publisher":"示例出版社","pubDate":"2024-05-01","isbn":"978-7-0000-0000-0","language":"zh-CN"}`)
	requireStatus(t, rr, http.StatusOK)
	body := decodeBody[bookDetailResp](t, rr)
	if body.Rating != 4 || body.Publisher != "示例出版社" || body.PubDate != "2024-05-01" {
		t.Fatalf("PATCH 出版信息 = %+v", body)
	}
	if body.ISBN != "978-7-0000-0000-0" || body.Language != "zh-CN" {
		t.Fatalf("PATCH isbn/language = %q / %q", body.ISBN, body.Language)
	}
	got := decodeBody[bookDetailResp](t, env.get(bookPath(id)))
	if got.Rating != 4 || got.PubDate != "2024-05-01" {
		t.Fatalf("详情回读 = %+v", got)
	}
}

// TestPatchBookRatingClear 覆盖 rating=0（未评）与清空出版信息。
func TestPatchBookRatingClear(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("清空评分示例书", "示例作者")
	requireStatus(t, env.patch(bookPath(id), `{"rating":5,"publisher":"示例出版社"}`), http.StatusOK)

	requireStatus(t, env.patch(bookPath(id), `{"rating":0,"publisher":"","pubDate":""}`), http.StatusOK)
	got := decodeBody[bookDetailResp](t, env.get(bookPath(id)))
	if got.Rating != 0 || got.Publisher != "" || got.PubDate != "" {
		t.Fatalf("清空后 = %+v", got)
	}
}

// TestPatchBookPublishingValidation 覆盖 rating 越界与 pubDate 格式非法。
func TestPatchBookPublishingValidation(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("校验示例书", "示例作者")
	for _, body := range []string{`{"rating":6}`, `{"rating":-1}`, `{"pubDate":"2024/05/01"}`, `{"pubDate":"2024-13-40"}`} {
		requireError(t, env.patch(bookPath(id), body), http.StatusBadRequest, codeInvalidParam)
	}
}
