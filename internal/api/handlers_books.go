package api

import (
	"net/http"
	"strings"

	"norvels/internal/store"
)

// bookSorts 是 GET /api/books 支持的排序字段。
var bookSorts = map[string]bool{
	"updated":  true,
	"title":    true,
	"chapters": true,
	"words":    true,
	"imported": true,
}

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "updated"
	}
	if !bookSorts[sort] {
		s.badRequest(w, badParam("参数 sort 只支持 updated|title|chapters|words|imported"), codeInvalidParam)
		return
	}
	authorID, _, err := int64Query(r, "authorId")
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	q := store.BookQuery{
		Q:        strings.TrimSpace(r.URL.Query().Get("q")),
		AuthorID: authorID,
		Tag:      strings.TrimSpace(r.URL.Query().Get("tag")),
		Sort:     sort,
		Page:     page,
		Size:     size,
	}
	res, err := s.st.ListBooks(r.Context(), q)
	if err != nil {
		s.fail(w, r, err, "书籍列表不可用")
		return
	}
	normalizePage(&res, page, size)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	detail, err := s.st.GetBook(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "book not found")
		return
	}
	if detail.Volumes == nil {
		detail.Volumes = []store.VolumeNode{}
	}
	writeJSON(w, http.StatusOK, detail)
}

// chapterListResponse 对应 docs/api.md 的 GET /api/books/{id}/chapters（items + total）。
type chapterListResponse struct {
	Items []store.ChapterBrief `json:"items"`
	Total int                  `json:"total"`
}

func (s *Server) handleListChapters(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	from, err := intQuery(r, "from", 1, 1, maxPage)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	limit, err := intQuery(r, "limit", defaultChapterLimit, 1, maxChapterLimit)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	res, err := s.st.ListChapters(r.Context(), store.ChapterQuery{BookID: bookID, From: from, Limit: limit})
	if err != nil {
		s.fail(w, r, err, "book not found")
		return
	}
	items := res.Items
	if items == nil {
		items = []store.ChapterBrief{}
	}
	writeJSON(w, http.StatusOK, chapterListResponse{Items: items, Total: res.Total})
}

func (s *Server) handleGetChapter(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	chapterID, err := pathID(r, "cid")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	chapter, err := s.st.GetChapter(r.Context(), bookID, chapterID)
	if err != nil {
		s.fail(w, r, err, "chapter not found")
		return
	}
	if chapter == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "chapter not found")
		return
	}
	writeJSON(w, http.StatusOK, chapter)
}
