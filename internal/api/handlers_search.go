package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// minQueryRunes 是全文检索关键词的最短长度（避免全表扫描）。
const minQueryRunes = 2

// 搜索范围只到书名与作者：章节标题（目录）不再参与匹配。
// 拒绝时直接说清能搜什么，避免调用方以为还能按章节搜。
const (
	searchTypeBook = "book"
	searchTypeAll  = "all"
	noticeScope    = "搜索范围仅支持书名与作者"
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
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	if err := checkSearchType(r); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	res, err := s.st.Search(r.Context(), store.SearchQuery{Q: q, Page: page, Size: size})
	if err != nil {
		s.fail(w, r, err, "检索不可用")
		return
	}
	if res == nil {
		res = &store.SearchResult{}
	}
	res.Q = q
	normalizePage(&res.Books, page, size)
	writeJSON(w, http.StatusOK, res)
}

// checkSearchType 校验 type：只接受 book / all（两者等价，都是书名 + 作者）。
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
