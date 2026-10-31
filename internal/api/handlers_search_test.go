package api

import (
	"net/http"
	"net/url"
	"strconv"
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

// searchFixture 是三本筛选测试书：分类 / 评分 / 阅读状态 / 连载状态 / 标签/作者各不相同，
// 每条筛选都能落到唯一一本（或唯一一组），断言总数即可。
type searchFixture struct {
	dongfang int64 // 玄幻 › 东方玄幻，5 星，在读，连载中，标签「武侠」
	qihuan   int64 // 玄幻 › 奇幻，3 星，已读，已完结
	dushi    int64 // 都市（无小分类），未评，未标记，未知
}

// seedSearchFilters 造三本书名都含「示例」的书，再逐本 PATCH 出筛选维度。
func seedSearchFilters(t *testing.T, env *testEnv) searchFixture {
	t.Helper()
	base := []struct {
		title string
		patch string
	}{
		{"示例玄幻甲", `{"categoryL1":"玄幻","categoryL2":"东方玄幻","rating":5,"readStatus":"reading","serialStatus":"ongoing","tags":["武侠"]}`},
		{"示例玄幻乙", `{"categoryL1":"玄幻","categoryL2":"奇幻","rating":3,"readStatus":"done","serialStatus":"completed"}`},
		{"示例都市丙", `{"categoryL1":"都市"}`},
	}
	ids := make([]int64, 0, len(base))
	for i, item := range base {
		id := env.seedBook(item.title, "作者"+string(rune('甲'+i)))
		if rr := env.patch(bookPath(id), item.patch); rr.Code != http.StatusOK {
			t.Fatalf("PATCH %s 失败: %d %s", item.title, rr.Code, rr.Body.String())
		}
		ids = append(ids, id)
	}
	return searchFixture{dongfang: ids[0], qihuan: ids[1], dushi: ids[2]}
}

// TestSearchFilters：q 之外的筛选参数与 GET /api/books 同名同口径，且每条都真的收窄结果。
func TestSearchFilters(t *testing.T) {
	env := newEnv(t)
	fx := seedSearchFilters(t, env)
	authorID := bookAuthorID(t, env, fx.dongfang)

	cases := []struct {
		name  string
		query string
		total int
	}{
		{"不加筛选", "", 3},
		{"大分类", "&categoryL1=" + url.QueryEscape("玄幻"), 2},
		{"大+小分类", "&categoryL1=" + url.QueryEscape("玄幻") + "&categoryL2=" + url.QueryEscape("东方玄幻"), 1},
		{"只给小分类按不过滤", "&categoryL2=" + url.QueryEscape("奇幻"), 3},
		{"评分", "&rating=5", 1},
		{"阅读状态", "&readStatus=reading", 1},
		{"连载中", "&serialStatus=ongoing", 1},
		{"连载未知", "&serialStatus=none", 1},
		{"标签", "&tag=" + url.QueryEscape("武侠"), 1},
		{"作者", "&authorId=" + strconv.FormatInt(authorID, 10), 1},
		{"多条件叠加", "&categoryL1=" + url.QueryEscape("玄幻") + "&readStatus=done&serialStatus=completed", 1},
	}
	for _, tc := range cases {
		res := getSearch(t, env, "/api/search?q="+url.QueryEscape("示例")+tc.query)
		if res.Books.Total != tc.total {
			t.Errorf("%s：命中 %d，期望 %d", tc.name, res.Books.Total, tc.total)
		}
	}
}

// TestSearchFilterValidation：非法值沿用 /api/books 的 400 + codeInvalidParam，不静默忽略。
func TestSearchFilterValidation(t *testing.T) {
	env := newEnv(t)
	for _, query := range []string{
		"&rating=0",
		"&rating=6",
		"&rating=abc",
		"&readStatus=bogus",
		"&serialStatus=bogus",
		"&authorId=abc",
		"&authorId=0",
		"&page=0",
		"&size=0",
	} {
		requireError(t, env.get("/api/search?q="+url.QueryEscape("示例")+query), http.StatusBadRequest, codeInvalidParam)
	}
}

// TestSearchFilterPagination：筛选与分页组合时，总数是「筛选后的总数」，页码原样回填。
func TestSearchFilterPagination(t *testing.T) {
	env := newEnv(t)
	seedSearchFilters(t, env)

	target := "/api/search?q=" + url.QueryEscape("示例") + "&categoryL1=" + url.QueryEscape("玄幻") + "&size=1&page=2"
	res := getSearch(t, env, target)
	if res.Books.Total != 2 || res.Books.Page != 2 || res.Books.Size != 1 {
		t.Fatalf("分页/筛选组合 = total %d page %d size %d，期望 2/2/1", res.Books.Total, res.Books.Page, res.Books.Size)
	}
}
