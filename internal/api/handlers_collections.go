package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"norvels/internal/store"
)

// 书单 CRUD 与书目全量替换（P3 §自定义书单）。
//
// 书单是扁平收藏夹：一本书可属多个书单，不做嵌套/排序（见 p3-scope 的取舍）；
// collections.name 全库唯一，重名是 409「换个名字」，不是并发冲突。

// maxCollectionNameRunes 是 api 侧书单名上限；store 侧还有一道更宽的同名守卫。
const maxCollectionNameRunes = 60

// collectionsResponse 对应 docs/api.md 的 GET /api/collections。
type collectionsResponse struct {
	Items []store.Collection `json:"items"`
}

// collectionNameRequest 是 POST/PATCH 书单的请求体。
type collectionNameRequest struct {
	Name string `json:"name"`
}

// collectionBooksRequest 是 PUT /api/collections/{id}/books 的请求体。
//
// 用指针区分「没传 bookIds」与「传了空数组」：前者是坏请求（可能误清空），
// 后者是明确的清空意图。
type collectionBooksRequest struct {
	BookIDs *[]int64 `json:"bookIds"`
}

// handleListCollections 处理 GET /api/collections：全部书单 + 实时书目数。
func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.ListCollections(r.Context())
	if err != nil {
		s.fail(w, r, err, "书单列表不可用")
		return
	}
	if items == nil {
		items = []store.Collection{}
	}
	writeJSON(w, http.StatusOK, collectionsResponse{Items: items})
}

// handleCreateCollection 处理 POST /api/collections：201 + 新书单，重名 409。
func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	name, ok := s.collectionNameFromBody(w, r)
	if !ok {
		return
	}
	item, err := s.st.CreateCollection(r.Context(), name)
	if err != nil {
		s.failCollectionWrite(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// handleRenameCollection 处理 PATCH /api/collections/{id}：200 + 改名后的书单。
func (s *Server) handleRenameCollection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	name, ok := s.collectionNameFromBody(w, r)
	if !ok {
		return
	}
	if err := s.st.RenameCollection(r.Context(), id, name); err != nil {
		s.failCollectionWrite(w, r, err)
		return
	}
	item, ok, err := s.findCollection(r.Context(), id)
	if err != nil || !ok {
		s.fail(w, r, err, "书单不存在")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleDeleteCollection 处理 DELETE /api/collections/{id}：204，书目关联级联清空。
func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	if err := s.st.DeleteCollection(r.Context(), id); err != nil {
		s.fail(w, r, err, "书单不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReplaceCollectionBooks 处理 PUT /api/collections/{id}/books：全量替换，204。
func (s *Server) handleReplaceCollectionBooks(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	var body collectionBooksRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"bookIds":[...]}`) {
		return
	}
	if body.BookIDs == nil {
		s.badRequest(w, badParam("请求体必须包含 bookIds 数组（传 [] 表示清空）"), codeInvalidParam)
		return
	}
	if err := s.st.ReplaceCollectionBooks(r.Context(), id, *body.BookIDs); err != nil {
		s.failCollectionWrite(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// collectionNameFromBody 解析并校验书单名；出错时已写好 400。
func (s *Server) collectionNameFromBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body collectionNameRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"name":"..."}`) {
		return "", false
	}
	name, err := cleanCollectionName(body.Name)
	if err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return "", false
	}
	return name, true
}

// cleanCollectionName 归一化并校验书单名：去首尾空白后非空、不超上限。
func cleanCollectionName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", badParam("书单名不能为空")
	}
	if utf8.RuneCountInString(name) > maxCollectionNameRunes {
		return "", badParam("书单名不能超过 %d 字", maxCollectionNameRunes)
	}
	return name, nil
}

// failCollectionWrite 把书单写失败翻译成 409（重名）/400（入参）/404（不存在）。
func (s *Server) failCollectionWrite(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, "书单名已存在")
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	default:
		s.fail(w, r, err, "书单不存在")
	}
}

// findCollection 回读书单的最新状态（改名/建单后用，书单量小不必单开查询）。
func (s *Server) findCollection(ctx context.Context, id int64) (store.Collection, bool, error) {
	items, err := s.st.ListCollections(ctx)
	if err != nil {
		return store.Collection{}, false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it, true, nil
		}
	}
	return store.Collection{}, false, nil
}
