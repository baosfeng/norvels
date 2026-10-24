package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/store"
)

func TestHealth(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/health")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q，期望 application/json", ct)
	}
	body := decodeBody[healthResponse](t, rr)
	if body.Status != "ok" || body.DB != "ok" {
		t.Fatalf("健康检查 = %+v，期望 status/db 均为 ok", body)
	}
	if body.Version != "slim" {
		t.Fatalf("version = %q，期望 slim", body.Version)
	}
	if body.UptimeSeconds < 0 {
		t.Fatalf("uptimeSeconds = %d，期望 >= 0", body.UptimeSeconds)
	}
}

func TestVersion(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/version")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", rr.Code)
	}
	body := decodeBody[versionResponse](t, rr)
	if body.Version != "slim" || body.Commit != "cafe" || body.BuiltAt == "" {
		t.Fatalf("/api/version 返回 %+v", body)
	}
}

func TestListBooksParamValidation(t *testing.T) {
	env := newEnv(t)
	cases := []string{
		"/api/books?page=0",
		"/api/books?page=-1",
		"/api/books?page=abc",
		"/api/books?size=0",
		"/api/books?size=201",
		"/api/books?size=x",
		"/api/books?sort=bogus",
		"/api/books?authorId=0",
		"/api/books?authorId=abc",
	}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			requireError(t, env.get(target), http.StatusBadRequest, codeInvalidParam)
		})
	}
}

func TestListBooks(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("测试之书", "测试作者")
	env.seedBook("第二本书", "另一位作者")

	t.Run("首页分页与目标书", func(t *testing.T) { checkBookListPage(t, env, id) })
	t.Run("书名过滤", func(t *testing.T) { checkBookListFilter(t, env, id) })
	t.Run("分页边界", func(t *testing.T) { checkBookListSecondPage(t, env) })
}

// checkBookListPage 校验首页条数、分页回填字段与目标书是否在列表中。
func checkBookListPage(t *testing.T, env *testEnv, id int64) {
	t.Helper()
	rr := env.get("/api/books?page=1&size=10")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	page := decodeBody[store.Page[store.BookSummary]](t, rr)
	if page.Total < 2 || len(page.Items) < 2 {
		t.Fatalf("分页结果 = %+v，期望至少 2 本", page)
	}
	if page.Page != 1 || page.Size != 10 {
		t.Fatalf("分页回填 = page %d size %d，期望 1/10", page.Page, page.Size)
	}
	for _, item := range page.Items {
		if item.ID == id && item.Title == "测试之书" && item.AuthorName == "测试作者" {
			return
		}
	}
	t.Fatalf("列表中未找到 book %d，items=%+v", id, page.Items)
}

// checkBookListFilter 书名过滤只返回目标书。
func checkBookListFilter(t *testing.T, env *testEnv, id int64) {
	t.Helper()
	filtered := decodeBody[store.Page[store.BookSummary]](t, env.get("/api/books?q=测试之书"))
	if filtered.Total != 1 || filtered.Items[0].ID != id {
		t.Fatalf("q=测试之书 结果 = %+v", filtered)
	}
}

// checkBookListSecondPage size=1 的第二页只有 1 条且回填字段正确。
func checkBookListSecondPage(t *testing.T, env *testEnv) {
	t.Helper()
	second := decodeBody[store.Page[store.BookSummary]](t, env.get("/api/books?page=2&size=1"))
	if second.Page != 2 || second.Size != 1 || len(second.Items) != 1 {
		t.Fatalf("第二页 = %+v", second)
	}
}

func TestGetBook(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("测试之书", "测试作者")

	rr := env.get(fmt.Sprintf("/api/books/%d", id))
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	detail := decodeBody[store.BookDetail](t, rr)
	if detail.ID != id || detail.Title != "测试之书" {
		t.Fatalf("详情 = %+v", detail)
	}
	if len(detail.Volumes) == 0 {
		t.Fatalf("详情缺少卷/章节树: %+v", detail)
	}
}

func TestGetBookInvalidID(t *testing.T) {
	env := newEnv(t)
	for _, target := range []string{"/api/books/abc", "/api/books/0", "/api/books/-1", "/api/books/1.5"} {
		t.Run(target, func(t *testing.T) {
			requireError(t, env.get(target), http.StatusBadRequest, codeInvalidID)
		})
	}
}

func TestGetBookNotFound(t *testing.T) {
	env := newEnv(t)
	env.seedBook("测试之书", "测试作者")
	requireError(t, env.get("/api/books/999999"), http.StatusNotFound, codeNotFound)
}

func TestListChapters(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("测试之书", "测试作者")
	base := fmt.Sprintf("/api/books/%d/chapters", id)

	rr := env.get(base + "?limit=2")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	list := decodeBody[chapterListResponse](t, rr)
	if list.Total != 3 {
		t.Fatalf("total = %d，期望 3", list.Total)
	}
	if len(list.Items) != 2 {
		t.Fatalf("items = %d，期望 2", len(list.Items))
	}
	if list.Items[0].ID == 0 || list.Items[0].Title == "" {
		t.Fatalf("章节条目为空: %+v", list.Items[0])
	}

	second := decodeBody[chapterListResponse](t, env.get(base+"?from=2&limit=2"))
	if len(second.Items) != 1 {
		t.Fatalf("第二页条目 = %d，期望 1", len(second.Items))
	}

	beyond := decodeBody[chapterListResponse](t, env.get(base+"?from=99&limit=2"))
	if len(beyond.Items) != 0 || beyond.Total != 3 {
		t.Fatalf("越界页 = %+v", beyond)
	}

	for _, target := range []string{base + "?from=0", base + "?limit=0", base + "?limit=501", base + "?limit=x"} {
		t.Run(target, func(t *testing.T) {
			requireError(t, env.get(target), http.StatusBadRequest, codeInvalidParam)
		})
	}
}

func TestGetChapter(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("测试之书", "测试作者")
	list := decodeBody[chapterListResponse](t, env.get(fmt.Sprintf("/api/books/%d/chapters", id)))
	if len(list.Items) == 0 {
		t.Fatal("目录为空")
	}
	first := list.Items[0]

	rr := env.get(fmt.Sprintf("/api/books/%d/chapters/%d", id, first.ID))
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	chapter := decodeBody[store.ChapterContent](t, rr)
	if chapter.BookID != id || chapter.ID != first.ID {
		t.Fatalf("章节 = %+v", chapter)
	}
	if chapter.Title == "" || chapter.Content == "" {
		t.Fatalf("章节标题/正文为空: %+v", chapter)
	}
	if chapter.BookTitle != "测试之书" {
		t.Fatalf("bookTitle = %q", chapter.BookTitle)
	}

	requireError(t, env.get(fmt.Sprintf("/api/books/%d/chapters/999999", id)), http.StatusNotFound, codeNotFound)
	requireError(t, env.get(fmt.Sprintf("/api/books/%d/chapters/abc", id)), http.StatusBadRequest, codeInvalidID)
}

func TestListAuthors(t *testing.T) {
	env := newEnv(t)
	env.seedBook("测试之书", "测试作者")
	env.seedBook("第二本书", "另一位作者")

	rr := env.get("/api/authors?q=测试")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	page := decodeBody[store.Page[store.AuthorSummary]](t, rr)
	if page.Total < 1 || len(page.Items) == 0 {
		t.Fatalf("作者列表 = %+v", page)
	}
	author := page.Items[0]

	detail := decodeBody[store.AuthorDetail](t, env.get(fmt.Sprintf("/api/authors/%d", author.ID)))
	if detail.ID != author.ID || len(detail.Books) == 0 {
		t.Fatalf("作者详情 = %+v", detail)
	}

	for _, target := range []string{"/api/authors?sort=bogus", "/api/authors?page=0", "/api/authors?size=999"} {
		t.Run(target, func(t *testing.T) {
			requireError(t, env.get(target), http.StatusBadRequest, codeInvalidParam)
		})
	}
	requireError(t, env.get("/api/authors/abc"), http.StatusBadRequest, codeInvalidID)
	requireError(t, env.get("/api/authors/999999"), http.StatusNotFound, codeNotFound)
}

func TestSearchValidation(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.get("/api/search"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/search?q=%20"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/search?q=单"), http.StatusBadRequest, codeQueryTooShort)
	requireError(t, env.get("/api/search?q=测试&type=bogus"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/search?q=测试&page=0"), http.StatusBadRequest, codeInvalidParam)
}

func TestSearch(t *testing.T) {
	env := newEnv(t)
	env.seedBook("测试之书", "测试作者")

	rr := env.get("/api/search?q=测试")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	res := decodeBody[store.SearchResult](t, rr)
	if res.Q != "测试" {
		t.Fatalf("q = %q，期望回显 测试", res.Q)
	}
	if res.Books.Items == nil || res.Chapters.Items == nil {
		t.Fatalf("books/chapters items 不应为 null: %+v", res)
	}
	if res.Books.Page != 1 || res.Books.Size != 20 {
		t.Fatalf("分页回填 = %+v", res.Books)
	}

	// type=book 时也必须是 200
	if rr := env.get("/api/search?q=测试&type=book"); rr.Code != http.StatusOK {
		t.Fatalf("type=book 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
}

func TestTags(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/tags")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	res := decodeBody[tagsResponse](t, rr)
	if res.Items == nil {
		t.Fatal("items 不应为 null")
	}
}

func TestStats(t *testing.T) {
	env := newEnv(t)
	env.seedBook("测试之书", "测试作者")
	rr := env.get("/api/stats")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	stats := decodeBody[store.Stats](t, rr)
	if stats.Books < 1 || stats.Chapters < 3 || stats.Authors < 1 {
		t.Fatalf("统计 = %+v", stats)
	}
	if stats.DBBytes <= 0 {
		t.Fatalf("dbBytes = %d，期望 > 0", stats.DBBytes)
	}
}

func TestAPINotFoundAndMethodNotAllowed(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.get("/api/nope"), http.StatusNotFound, codeNotFound)

	rr := env.do(http.MethodPost, "/api/books", nil, nil)
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	if allow := rr.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("Allow = %q，期望包含 GET", allow)
	}
}
