package api

import (
	"errors"
	"net/http"

	"norvels/internal/store"
)

// 我的书架（迁移 0016 user_shelf）：用户主动加入书架的书。
//
// 全部书仍然可浏览，书架只是「我挑出来的那几本」。三个端点都走当前账号，
// 账号由鉴权中间件注入 context（开放模式下是默认账号）。

// shelfListResponse 是 GET /api/shelf 的响应。
type shelfListResponse struct {
	Items []store.ShelfItem `json:"items"`
	Total int               `json:"total"`
}

// shelfIDsResponse 是 GET /api/shelf/ids 的响应。
//
// 一次返回全量 id：前端拿它给书库列表的每一行打「已加入」标记，
// 翻页时不必再逐页问后端。量级是「一个人收藏的书」，不做分页。
type shelfIDsResponse struct {
	BookIDs []int64 `json:"bookIds"`
}

// handleListShelf 处理 GET /api/shelf?page=&size=：按加入时间倒序分页。
func (s *Server) handleListShelf(w http.ResponseWriter, r *http.Request) {
	page, size, err := parsePagination(r)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	items, total, err := s.st.ListShelf(r.Context(), page, size)
	if err != nil {
		s.fail(w, r, err, "书架不可用")
		return
	}
	if items == nil {
		items = []store.ShelfItem{}
	}
	writeJSON(w, http.StatusOK, shelfListResponse{Items: items, Total: total})
}

// handleShelfBookIDs 处理 GET /api/shelf/ids：当前账号书架的全部 book_id。
func (s *Server) handleShelfBookIDs(w http.ResponseWriter, r *http.Request) {
	ids, err := s.st.ShelfBookIDs(r.Context())
	if err != nil {
		s.fail(w, r, err, "书架不可用")
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	writeJSON(w, http.StatusOK, shelfIDsResponse{BookIDs: ids})
}

// handleAddToShelf 处理 PUT /api/shelf/{bookId}：加入书架，幂等，204。
//
// 书不存在 404；重复加入同样 204（前端不必先查是否已在架上）。
func (s *Server) handleAddToShelf(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.shelfBookID(w, r)
	if !ok {
		return
	}
	if err := s.st.AddToShelf(r.Context(), bookID); err != nil {
		s.failShelfWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveFromShelf 处理 DELETE /api/shelf/{bookId}：移出书架，幂等，204。
func (s *Server) handleRemoveFromShelf(w http.ResponseWriter, r *http.Request) {
	bookID, ok := s.shelfBookID(w, r)
	if !ok {
		return
	}
	if err := s.st.RemoveFromShelf(r.Context(), bookID); err != nil {
		s.failShelfWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// shelfBookID 解析路径里的 bookId；非法时已写好 400。
func (s *Server) shelfBookID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := pathID(r, "bookId")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return 0, false
	}
	return id, true
}

// failShelfWrite 把书架写失败翻译成 404（书不存在）/400（入参）/500。
func (s *Server) failShelfWrite(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, r, err, "书籍不存在")
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	default:
		s.fail(w, r, err, "书架写入失败")
	}
}
