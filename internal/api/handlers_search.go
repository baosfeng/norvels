package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// searchTypes 是 GET /api/search 支持的检索范围。
var searchTypes = map[string]bool{"chapter": true, "book": true, "all": true}

// minQueryRunes 是全文检索关键词的最短长度（避免全表扫描）。
const minQueryRunes = 2

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
	typ := strings.TrimSpace(r.URL.Query().Get("type"))
	if typ == "" {
		typ = "all"
	}
	if !searchTypes[typ] {
		s.badRequest(w, badParam("参数 type 只支持 chapter|book|all"), codeInvalidParam)
		return
	}
	bookID, _, err := int64Query(r, "bookId")
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	res, err := s.st.Search(r.Context(), store.SearchQuery{
		Q:      q,
		Type:   typ,
		Page:   page,
		Size:   size,
		BookID: bookID,
	})
	if err != nil {
		s.fail(w, r, err, "检索不可用")
		return
	}
	if res == nil {
		res = &store.SearchResult{}
	}
	res.Q = q
	normalizePage(&res.Books, page, size)
	normalizePage(&res.Chapters, page, size)
	writeJSON(w, http.StatusOK, res)
}
