package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// authorSorts 是 GET /api/authors 支持的排序字段。
var authorSorts = map[string]bool{"books": true, "name": true}

func (s *Server) handleListAuthors(w http.ResponseWriter, r *http.Request) {
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "books"
	}
	if !authorSorts[sort] {
		s.badRequest(w, badParam("参数 sort 只支持 books|name"), codeInvalidParam)
		return
	}
	res, err := s.st.ListAuthors(r.Context(), store.AuthorQuery{
		Q:    strings.TrimSpace(r.URL.Query().Get("q")),
		Sort: sort,
		Page: page,
		Size: size,
	})
	if err != nil {
		s.fail(w, r, err, "作者列表不可用")
		return
	}
	normalizePage(&res, page, size)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetAuthor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	detail, err := s.st.GetAuthor(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "author not found")
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "author not found")
		return
	}
	if detail.Books == nil {
		detail.Books = []store.BookSummary{}
	}
	writeJSON(w, http.StatusOK, detail)
}
