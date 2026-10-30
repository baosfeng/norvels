package api

import (
	"net/http"
	"net/url"
	"testing"

	"norvels/internal/store"
)

func TestSearchValidation(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.get("/api/search"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/search?q=%20"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/search?q=单"), http.StatusBadRequest, codeQueryTooShort)
	requireError(t, env.get("/api/search?q=测试&type=bogus"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.get("/api/search?q=测试&page=0"), http.StatusBadRequest, codeInvalidParam)
}

// TestSearchRejectsChapterType 搜索不再支持按章节标题（目录）检索：
// 传 type=chapter 必须明确报错，不能静默降级成书名搜索。
func TestSearchRejectsChapterType(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/search?q=测试&type=chapter")
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	body := decodeBody[errResp](t, rr)
	if body.Error.Message != noticeScope+"：传 chapter 无效，按章节标题（目录）搜索已下线" {
		t.Fatalf("错误消息 = %q，未说明搜索范围", body.Error.Message)
	}
}

// TestSearch 只返回书籍命中：书名 / 作者两路 LIKE，响应里没有 chapters 字段。
func TestSearch(t *testing.T) {
	env := newEnv(t)
	env.seedBook("测试之书", "测试作者")

	t.Run("按书名", func(t *testing.T) { checkSearchByTitle(t, env) })
	t.Run("按作者", func(t *testing.T) { checkSearchByAuthor(t, env) })
	t.Run("按章节标题不再命中", func(t *testing.T) { checkSearchChapterTitleMiss(t, env) })
	t.Run("type=book 与默认等价", func(t *testing.T) { checkSearchTypeBook(t, env) })
	t.Run("分页回填", func(t *testing.T) { checkSearchPagination(t, env) })
}

// searchBody 是搜索响应的断言用结构：只认 books 与 notice。
type searchBody struct {
	Q     string               `json:"q"`
	Books store.Page[struct{}] `json:"books"`
	// Chapters 用指针区分「字段不存在（nil）」与「空分页」。
	Chapters *struct{} `json:"chapters"`
	Notice   string    `json:"notice"`
}

// getSearch 发起搜索并解析响应，要求 200。
func getSearch(t *testing.T, env *testEnv, target string) searchBody {
	t.Helper()
	rr := env.get(target)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s 状态码 = %d，body=%s", target, rr.Code, rr.Body.String())
	}
	return decodeBody[searchBody](t, rr)
}

// checkSearchByTitle 书名命中该本书。
func checkSearchByTitle(t *testing.T, env *testEnv) {
	t.Helper()
	res := getSearch(t, env, "/api/search?q=测试之书")
	if res.Books.Total != 1 || res.Q != "测试之书" {
		t.Fatalf("书名命中 = %+v，q = %q", res.Books, res.Q)
	}
}

// checkSearchByAuthor 作者命中该本书。
func checkSearchByAuthor(t *testing.T, env *testEnv) {
	t.Helper()
	res := getSearch(t, env, "/api/search?q=测试作者")
	if res.Books.Total != 1 {
		t.Fatalf("作者命中 = %+v", res.Books)
	}
}

// checkSearchChapterTitleMiss 种子书的章节标题（序章 / 第1章 起）不再参与匹配，
// 且响应里不再有 chapters 字段。
func checkSearchChapterTitleMiss(t *testing.T, env *testEnv) {
	t.Helper()
	for _, kw := range []string{"第1章 起", "序章", "第2章"} {
		target := "/api/search?q=" + url.QueryEscape(kw)
		res := getSearch(t, env, target)
		if res.Books.Total != 0 {
			t.Errorf("按章节标题 %q 不应命中书籍: %+v", kw, res.Books)
		}
		if res.Chapters != nil {
			t.Errorf("%q 的响应仍带 chapters 字段", kw)
		}
	}
}

// checkSearchTypeBook type=book / type=all 都是书名 + 作者，200 且结果一致。
func checkSearchTypeBook(t *testing.T, env *testEnv) {
	t.Helper()
	for _, typ := range []string{"book", "all"} {
		res := getSearch(t, env, "/api/search?q=测试之书&type="+typ)
		if res.Books.Total != 1 {
			t.Errorf("type=%s 命中 = %+v", typ, res.Books)
		}
	}
}

// checkSearchPagination page/size 原样回填到 books 分页。
func checkSearchPagination(t *testing.T, env *testEnv) {
	t.Helper()
	res := getSearch(t, env, "/api/search?q=测试&page=2&size=5")
	if res.Books.Page != 2 || res.Books.Size != 5 {
		t.Fatalf("分页回填 = %d/%d，期望 2/5", res.Books.Page, res.Books.Size)
	}
	if res.Books.Items == nil {
		t.Fatal("items 不应为 null")
	}
}
