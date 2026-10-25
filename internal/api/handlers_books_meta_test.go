package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// bookDetailResp 是 GET/PATCH /api/books/{id} 的响应结构（只取测试要断言的字段）。
type bookDetailResp struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	AuthorName  string   `json:"authorName"`
	Authors     []string `json:"authors"`
	Intro       string   `json:"intro"`
	Series      string   `json:"series"`
	SeriesIndex int      `json:"seriesIndex"`
	HasCover    bool     `json:"hasCover"`
	Format      string   `json:"format"`
	Tags        []string `json:"tags"`
	// 0010 起的详情增量字段：连载状态与最新章节。
	Status           string `json:"status"`
	SerialStatus     string `json:"serialStatus"`
	LastChapterIdx   int    `json:"lastChapterIdx"`
	LastChapterTitle string `json:"lastChapterTitle"`
	// 0011 起的阅读体验增量：阅读状态与书签数。
	ReadStatus    string `json:"readStatus"`
	BookmarkCount int    `json:"bookmarkCount"`
}

// authorsResp 是 GET /api/authors 的响应（items + total）。
type authorsResp struct {
	Items []store.AuthorSummary `json:"items"`
	Total int                   `json:"total"`
}

// patch 发起一次 PATCH /api/books/{id}。
func (e *testEnv) patch(target, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPatch, target, strings.NewReader(body), map[string]string{"Content-Type": "application/json"})
}

// bookPath 拼接单本书的路径。
func bookPath(id int64) string { return "/api/books/" + strconv.FormatInt(id, 10) }

func TestPatchBook(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("待改名的示例书", "示例作者")
	target := bookPath(id)

	rr := env.patch(target, `{"title":"改名后的示例书","intro":"新简介","tags":["示例分类"],"series":"示例系列","seriesIndex":1}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[bookDetailResp](t, rr)
	if body.Title != "改名后的示例书" || body.Intro != "新简介" || body.Series != "示例系列" || body.SeriesIndex != 1 {
		t.Fatalf("PATCH 响应 = %+v", body)
	}
	if len(body.Tags) != 1 || body.Tags[0] != "示例分类" {
		t.Fatalf("PATCH 响应 tags = %v", body.Tags)
	}

	// 保存后详情立即反映（spec §6 的验收线）。
	got := decodeBody[bookDetailResp](t, env.get(target))
	if got.Title != "改名后的示例书" || len(got.Tags) != 1 {
		t.Fatalf("GET 详情 = %+v", got)
	}
}

func TestPatchBookAuthorChangeRecomputesAuthors(t *testing.T) {
	env := newEnv(t)
	movedID := env.seedBook("换作者的书", "作者甲")
	env.seedBook("原属作者乙的书", "作者乙")

	rr := env.patch(bookPath(movedID), `{"author":"作者乙"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	authors := decodeBody[authorsResp](t, env.get("/api/authors?size=50"))
	counts := make(map[string]int, len(authors.Items))
	for _, a := range authors.Items {
		counts[a.Name] = a.BookCount
	}
	if counts["作者乙"] != 2 || counts["作者甲"] != 0 {
		t.Fatalf("作者聚合 = %v, want 作者乙 2 / 作者甲 0", counts)
	}
}

func TestPatchBookValidation(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("校验用示例书", "示例作者")
	target := bookPath(id)

	cases := []struct {
		name   string
		target string
		body   string
		status int
		code   string
	}{
		{"空 body", target, "", http.StatusBadRequest, codeInvalidParam},
		{"空对象", target, "{}", http.StatusBadRequest, codeInvalidParam},
		{"只有未知字段", target, `{"nope":1}`, http.StatusBadRequest, codeInvalidParam},
		{"非法 JSON", target, `{"title":`, http.StatusBadRequest, codeInvalidParam},
		{"空书名", target, `{"title":"   "}`, http.StatusBadRequest, codeInvalidParam},
		{"非法 id", "/api/books/abc", `{"title":"x"}`, http.StatusBadRequest, codeInvalidID},
		{"书不存在", "/api/books/999999", `{"title":"x"}`, http.StatusNotFound, codeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireError(t, env.patch(tc.target, tc.body), tc.status, tc.code)
		})
	}
}

func TestGetBookCoverNotFound(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("没有封面的示例书", "示例作者")
	requireError(t, env.get(bookPath(id)+"/cover"), http.StatusNotFound, codeNotFound)
}

func TestGetBookCover(t *testing.T) {
	env := newEnv(t)
	coverBytes := []byte("\x89PNG\r\n\x1a\napi-cover")
	book := &model.ParsedBook{
		Title:        "带封面的示例书",
		Author:       "示例作者",
		SourcePath:   "/path/to/cover.epub",
		SourceName:   "cover.epub",
		Encoding:     "utf-8",
		FileSize:     4096,
		SHA256:       strings.Repeat("q", 64),
		WordCount:    2,
		SourceFormat: "epub",
		Cover:        &model.CoverImage{MIME: "image/png", Bytes: coverBytes},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: "正文。", WordCount: 2},
		},
	}
	res, err := env.st.SaveBook(context.Background(), book, store.SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	rr := env.get(bookPath(res.BookID) + "/cover")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "public, max-age=86400" {
		t.Errorf("Cache-Control = %q, want public, max-age=86400", cc)
	}
	if rr.Body.String() != string(coverBytes) {
		t.Errorf("响应体与封面字节不一致：%q", rr.Body.String())
	}

	// 详情要告诉前端这本书有封面（前端据此渲染 img 而不是占位）。
	detail := decodeBody[bookDetailResp](t, env.get(bookPath(res.BookID)))
	if !detail.HasCover || detail.Format != "epub" {
		t.Fatalf("详情 hasCover/format = %v/%q", detail.HasCover, detail.Format)
	}
}
