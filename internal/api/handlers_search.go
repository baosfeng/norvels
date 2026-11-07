package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// minQueryRunes 是搜索关键词的最短长度：一个字在书名上的命中范围过宽，也省掉一次全表 LIKE。
const minQueryRunes = 2

// 搜索范围只到书名：章节标题（目录）与作者名都不参与匹配。
// 拒绝时直接说清能搜什么，避免调用方以为还能按章节搜。
const (
	searchTypeBook = "book"
	searchTypeAll  = "all"
	noticeScope    = "搜索范围仅支持书名"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "参数 q 必填")
		return
	}
	if len([]rune(q)) < minQueryRunes {
		writeError(w, http.StatusBadRequest, codeQueryTooShort, "关键词至少 2 个字")
		return
	}
	query, err := searchQuery(r, q)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	if err := checkSearchType(r); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	res, err := s.st.Search(r.Context(), query)
	if err != nil {
		s.fail(w, r, err, "检索不可用")
		return
	}
	if res == nil {
		res = &store.SearchResult{}
	}
	res.Q = q
	normalizePage(&res.Books, query.Page, query.Size)
	writeJSON(w, http.StatusOK, res)
}

// searchQuery 解析检索请求：q 之外还接受与 GET /api/books 同名同口径的筛选参数。
//
// 复用 handlers_books.go 的解析函数而不是再写一套：非法值、边界与错误文案
// 在列表页和搜索页必须一致，两份实现迟早会漂移（rating 允许 1–5、serialStatus
// 的 none 哨兵、authorId 必须是正整数……都在那些函数里）。
// 只给 categoryL2 不给 categoryL1 时按「不过滤」处理——store 侧 appendCategoryWhere
// 就是这么做的，这里不必额外拒绝（与 /api/books 一致）。
func searchQuery(r *http.Request, q string) (store.SearchQuery, error) {
	page, size, err := parsePagination(r)
	if err != nil {
		return store.SearchQuery{}, err
	}
	rating, err := intQuery(r, "rating", 0, 1, 5)
	if err != nil {
		return store.SearchQuery{}, err
	}
	status, err := parseBookStatusFilters(r)
	if err != nil {
		return store.SearchQuery{}, err
	}
	authorID, _, err := int64Query(r, "authorId")
	if err != nil {
		return store.SearchQuery{}, err
	}
	return store.SearchQuery{
		Q:            q,
		Page:         page,
		Size:         size,
		CategoryL1:   strings.TrimSpace(r.URL.Query().Get("categoryL1")),
		CategoryL2:   strings.TrimSpace(r.URL.Query().Get("categoryL2")),
		Rating:       rating,
		ReadStatus:   status.read,
		SerialStatus: status.serial,
		AuthorID:     authorID,
		Tag:          strings.TrimSpace(r.URL.Query().Get("tag")),
	}, nil
}

// checkSearchType 校验 type：只接受 book / all（两者等价，都是书名检索）。
// chapter 明确报错而不是静默降级——降级会让调用方以为还能按目录搜。
func checkSearchType(r *http.Request) error {
	typ := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	switch typ {
	case "", searchTypeBook, searchTypeAll:
		return nil
	case "chapter":
		return badParam("%s：传 chapter 无效，按章节标题（目录）搜索已下线", noticeScope)
	default:
		return badParam("参数 type 只支持 book|all（%s）", noticeScope)
	}
}
