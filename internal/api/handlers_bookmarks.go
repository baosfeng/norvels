package api

import (
	"net/http"

	"norvels/internal/store"
)

// bookmarkListResponse 对应 GET /api/books/{id}/bookmarks（docs/api.md §2）。
type bookmarkListResponse struct {
	Bookmarks []store.Bookmark `json:"bookmarks"`
}

// bookmarkRequest 是 POST /api/books/{id}/bookmarks 的请求体。
//
// chapterId 与 paraIndex 都是必填：缺省值会让「第 0 段」这类真实位置被误当成默认值，
// 书签落错位置比报错更糟，因此用指针区分「没传」与「传了 0」。
type bookmarkRequest struct {
	ChapterID *int64  `json:"chapterId"`
	ParaIndex *int    `json:"paraIndex"`
	Label     *string `json:"label"`
}

// handleListBookmarks 列出全书书签（按 created_at 倒序）。
func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	items, err := s.st.ListBookmarks(r.Context(), bookID)
	if err != nil {
		s.fail(w, r, err, "书籍不存在")
		return
	}
	if items == nil {
		items = []store.Bookmark{}
	}
	writeJSON(w, http.StatusOK, bookmarkListResponse{Bookmarks: items})
}

// handleCreateBookmark 幂等新增书签：新建 201，命中同位置则更新 label 并返回 200。
func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	bookID, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	var body bookmarkRequest
	if !decodeJSONBody(w, r, &body, "请求体必须是 {chapterId, paraIndex, label?} 组成的 JSON 对象") {
		return
	}
	chapterID, paraIndex, label, err := body.fields()
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	bm, created, err := s.st.AddBookmark(r.Context(), bookID, chapterID, paraIndex, label)
	if err != nil {
		s.fail(w, r, err, "书籍不存在")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, bm)
}

// handleDeleteBookmark 删除一条书签；不存在 404。
func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	if err := s.st.DeleteBookmark(r.Context(), id); err != nil {
		s.fail(w, r, err, "书签不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fields 做字段级校验并解引用：两个必填项缺失即 400（不猜默认位置）。
func (b bookmarkRequest) fields() (chapterID int64, paraIndex int, label string, err error) {
	if b.ChapterID == nil || *b.ChapterID <= 0 {
		return 0, 0, "", badParam("chapterId 必须是正整数")
	}
	if b.ParaIndex == nil || *b.ParaIndex < 0 {
		return 0, 0, "", badParam("paraIndex 必须是非负整数")
	}
	if b.Label != nil {
		label = *b.Label
	}
	return *b.ChapterID, *b.ParaIndex, label, nil
}
